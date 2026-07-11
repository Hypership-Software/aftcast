// Package svc is the daemon lifecycle: it wires the observer libraries (risk
// classifier, taint ledger, HMAC audit log, integrity checker) into a resident
// daemon and serves the two local transports (control-plane stream + localhost
// HTTP hook listener). Every hook is recorded and classified; the daemon returns
// no decision, so Atlas observes without blocking.
//
// It runs in the foreground (`gated daemon run`); OS-service auto-start and a
// periodic read-model projection tick are deferred (the HMAC log is the source of
// truth, the projection a downstream rebuild).
package svc

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/Hypership-Software/atlas/internal/adapter"
	"github.com/Hypership-Software/atlas/internal/audit"
	"github.com/Hypership-Software/atlas/internal/daemon"
	"github.com/Hypership-Software/atlas/internal/integrity"
	"github.com/Hypership-Software/atlas/internal/ipc"
	"github.com/Hypership-Software/atlas/internal/policy"
	"github.com/Hypership-Software/atlas/internal/schema"
	"github.com/Hypership-Software/atlas/internal/taint"
)

const (
	defaultIntegrityTick = 5 * time.Minute
	maxSpoolLine         = 4 << 20 // matches audit's scanner cap
)

// Options configures a daemon run. The zero value is a valid production run
// against ~/.gated on the default hook port.
type Options struct {
	// Home is the gate's state directory. "" => $GATED_HOME, else ~/.gated.
	Home string
	// HTTPPort is the preferred hook port. 0 => ipc.DefaultHTTPPort, with a
	// fallback scan if taken (the bound port is reported in Info).
	HTTPPort int
	// TrustedDomains is the taint allowlist: fetches to these do not taint.
	TrustedDomains []string
	// IntegrityTick is the tamper-check interval. 0 => default; negative disables
	// the checker (tests).
	IntegrityTick time.Duration
	// Integrity says what to verify; an empty Config makes each check a no-op.
	Integrity integrity.Config
	// Ready, if non-nil, receives Info once both listeners are bound. Buffer it
	// (cap 1) or receive promptly.
	Ready chan<- Info
	// Logf overrides the logger; nil logs to stderr.
	Logf func(string, ...any)
}

// Info reports what a running daemon bound, for the CLI, doctor, and settings
// writer (which bakes HTTPURL into the hook config).
type Info struct {
	HTTPPort   int    `json:"http_port"`
	HTTPURL    string `json:"http_url"`
	PolicyHash string `json:"policy_hash"`
}

// daemonFile is the on-disk health record other commands read to find the live
// daemon's port and identity.
type daemonFile struct {
	PID        int    `json:"pid"`
	HTTPPort   int    `json:"http_port"`
	HTTPURL    string `json:"http_url"`
	PolicyHash string `json:"policy_hash"`
}

// Run wires the gate and serves until ctx is cancelled. It returns nil on a
// clean shutdown and the first fatal error otherwise.
func Run(ctx context.Context, opts Options) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	logf := opts.Logf
	if logf == nil {
		l := log.New(os.Stderr, "gated: ", log.LstdFlags)
		logf = func(f string, a ...any) { l.Printf(f, a...) }
	}

	home := resolveHome(opts.Home)
	if err := os.MkdirAll(home, 0o700); err != nil {
		return fmt.Errorf("create gate home %s: %w", home, err)
	}
	var (
		logDir     = filepath.Join(home, "log")
		policyDir  = filepath.Join(home, "policies")
		keyPath    = filepath.Join(home, "audit.key")
		daemonPath = filepath.Join(home, "daemon.json")
		spoolPath  = filepath.Join(home, "spool", "spool.jsonl")
	)

	key, err := loadOrCreateKey(keyPath)
	if err != nil {
		return fmt.Errorf("audit key: %w", err)
	}

	set, err := policy.LoadWithStarter(policyDir)
	if err != nil {
		return fmt.Errorf("load policy: %w", err)
	}
	engine := set.Engine()
	policyHash := set.Hash()

	alog, err := audit.NewLog(logDir, key)
	if err != nil {
		return fmt.Errorf("open audit log: %w", err)
	}
	defer alog.Close()
	// Stamp this machine's identity onto events that don't carry it (the harness
	// payload has neither user nor host).
	host, _ := os.Hostname()
	alog.SetIdentity(currentUser(), host)

	// Rebuild session taint from the log so a restart doesn't lose the tainted
	// flag. Non-fatal on read error — a fresh log is empty and taint starts clean.
	ledger := taint.NewLedger(opts.TrustedDomains)
	if evs, rerr := alog.Events(); rerr != nil {
		logf("taint rebuild: reading log: %v", rerr)
	} else {
		ledger.Rebuild(evs)
	}

	h := daemon.NewHandler(daemon.Deps{Eval: engine, Taint: ledger, Record: alog})

	// Drain telemetry the shim spooled while the daemon was down, before accepting
	// new events (no concurrency during drain).
	drainSpool(spoolPath, alog, logf)

	adp, ok := adapter.Get("claudecode")
	if !ok {
		return errors.New("claudecode adapter not registered")
	}

	cln, err := ipc.Listen()
	if err != nil {
		return fmt.Errorf("control-plane listen: %w", err)
	}
	defer cln.Close()
	httpLn, port, err := ipc.HTTPListen(opts.HTTPPort)
	if err != nil {
		return fmt.Errorf("hook listen: %w", err)
	}

	info := Info{
		HTTPPort:   port,
		HTTPURL:    fmt.Sprintf("http://127.0.0.1:%d/hook", port),
		PolicyHash: policyHash,
	}
	if werr := writeDaemonFile(daemonPath, info); werr != nil {
		logf("write daemon file: %v", werr) // non-fatal: doctor degrades gracefully
	}
	logf("gate ready: %d policies (hash %.12s), hook %s", set.Len(), policyHash, info.HTTPURL)

	var (
		mu     sync.Mutex
		runErr error
	)
	fail := func(e error) {
		if e == nil {
			return
		}
		mu.Lock()
		if runErr == nil {
			runErr = e
		}
		mu.Unlock()
		cancel() // one listener failing tears down the other
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); fail(daemon.Serve(ctx, cln, h)) }()
	go func() { defer wg.Done(); fail(serveHTTP(ctx, httpLn, hookHandler(h, adp, logf))) }()
	if tick := integrityInterval(opts.IntegrityTick); tick > 0 {
		wg.Add(1)
		go func() { defer wg.Done(); runIntegrityTicker(ctx, opts.Integrity, alog, logf, tick) }()
	}

	if opts.Ready != nil {
		opts.Ready <- info
	}

	wg.Wait()
	mu.Lock()
	defer mu.Unlock()
	return runErr
}

// currentUser reports the OS user from the environment so it stays CGO-free
// (USERNAME on Windows, USER on Unix).
func currentUser() string {
	for _, k := range []string{"USERNAME", "USER"} {
		if v := os.Getenv(k); v != "" {
			return v
		}
	}
	return ""
}

func resolveHome(home string) string {
	if home != "" {
		return home
	}
	if env := os.Getenv("GATED_HOME"); env != "" {
		return env
	}
	h, _ := os.UserHomeDir()
	return filepath.Join(h, ".gated")
}

// loadOrCreateKey returns the HMAC key at path, generating a fresh 32-byte key
// (0600) on first run. Hardening the key at rest (DPAPI/Keychain) is a later
// managed-tier concern.
func loadOrCreateKey(path string) ([]byte, error) {
	b, err := os.ReadFile(path)
	if err == nil {
		if len(b) == 0 {
			return nil, fmt.Errorf("%s is empty", path)
		}
		return b, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	if err := os.WriteFile(path, key, 0o600); err != nil {
		return nil, err
	}
	return key, nil
}

// drainSpool folds shim-spooled telemetry into the log and clears the spool. On
// a record error it leaves the spool in place so nothing is lost on the next
// start.
func drainSpool(spoolPath string, alog *audit.Log, logf func(string, ...any)) {
	f, err := os.Open(spoolPath)
	if errors.Is(err, os.ErrNotExist) {
		return
	}
	if err != nil {
		logf("spool: open: %v", err)
		return
	}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), maxSpoolLine)
	n := 0
	for sc.Scan() {
		var e schema.TelemetryEvent
		if err := json.Unmarshal(sc.Bytes(), &e); err != nil {
			logf("spool: skipping unparseable line: %v", err)
			continue
		}
		if err := alog.Record(e); err != nil {
			logf("spool: record: %v (leaving spool for retry)", err)
			f.Close()
			return
		}
		n++
	}
	f.Close()
	if err := sc.Err(); err != nil {
		logf("spool: scan: %v (leaving spool for retry)", err)
		return
	}
	if n > 0 {
		logf("drained %d spooled event(s)", n)
	}
	if err := os.Remove(spoolPath); err != nil {
		logf("spool: clear: %v", err)
	}
}

// hookHandler bridges an HTTP hook to the daemon: normalize, classify, record.
// HTTP hooks fail OPEN by design (Rev 4) — a payload we can't process returns 200
// with no decision so the session continues rather than wedging.
func hookHandler(h *daemon.Handler, adp adapter.Adapter, logf func(string, ...any)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(io.LimitReader(r.Body, ipc.MaxFrame))
		if err != nil {
			logf("hook: read body: %v", err)
			w.WriteHeader(http.StatusOK)
			return
		}
		desc, ev, err := adp.Normalize("", body)
		if err != nil {
			logf("hook: normalize: %v", err)
			w.WriteHeader(http.StatusOK)
			return
		}
		if _, err := h.Handle(daemon.Request{Event: ev, Descriptor: desc}); err != nil {
			logf("hook: handle: %v", err)
		}
		// Atlas observes; it never returns a decision. 200 with no body lets Claude
		// Code proceed under its own permission flow.
		w.WriteHeader(http.StatusOK)
	}
}

// serveHTTP serves the hook listener until ctx is cancelled, then drains with a
// short grace period. Clean shutdown returns nil.
func serveHTTP(ctx context.Context, ln net.Listener, handler http.Handler) error {
	srv := &http.Server{Handler: handler}
	go func() {
		<-ctx.Done()
		shutCtx, done := context.WithTimeout(context.Background(), 2*time.Second)
		defer done()
		_ = srv.Shutdown(shutCtx)
	}()
	err := srv.Serve(ln)
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

// runIntegrityTicker runs the tamper check once at startup and then on each tick,
// recording each drift as an integrity event plus a loud warning.
func runIntegrityTicker(ctx context.Context, cfg integrity.Config, rec *audit.Log, logf func(string, ...any), tick time.Duration) {
	checker := integrity.NewChecker(cfg)
	check := func() {
		for _, d := range checker.Check() {
			logf("INTEGRITY: %s — %s", d.Kind, d.Detail)
			if err := rec.Record(integrity.DriftEvent(d)); err != nil {
				logf("integrity: record drift: %v", err)
			}
		}
	}
	check()
	t := time.NewTicker(tick)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			check()
		}
	}
}

func integrityInterval(d time.Duration) time.Duration {
	switch {
	case d < 0:
		return 0
	case d == 0:
		return defaultIntegrityTick
	default:
		return d
	}
}

func writeDaemonFile(path string, info Info) error {
	b, err := json.MarshalIndent(daemonFile{
		PID:        os.Getpid(),
		HTTPPort:   info.HTTPPort,
		HTTPURL:    info.HTTPURL,
		PolicyHash: info.PolicyHash,
	}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o600)
}
