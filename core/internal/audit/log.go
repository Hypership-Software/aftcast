// Package audit is the tamper-evident event log — the single source of truth for
// the whole system. Every hook produces one TelemetryEvent (risk classification
// and telemetry share this one stream); each is HMAC-chained to its predecessor,
// so any reorder, insertion, deletion, or edit is detectable by Verify. The
// SQLite read-model and the taint ledger are both rebuildable projections of
// this log.
//
// The chain is metadata-only: TelemetryEvent carries no prompt/diff/output
// content (opt-in content goes to a separate deletable sidecar, Task 18), so
// right-to-erasure and tamper-evidence coexist.
package audit

import (
	"bufio"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/Hypership-Software/atlas/internal/schema"
)

const (
	eventsFile      = "events.jsonl"
	checkpointsFile = "checkpoints.jsonl"
	checkpointEvery = 100
	maxLine         = 4 << 20 // 4 MiB scanner line cap
)

// Log is an append-only, HMAC-chained event log on disk.
type Log struct {
	mu        sync.Mutex
	dir       string
	f         *os.File
	key       []byte
	seq       uint64
	prevHash  string
	sinceCkpt int
	user      string
	host      string
}

// SetIdentity records the machine identity the daemon stamps onto events that
// don't already carry it. Called once at daemon startup; safe to leave unset
// (fields simply stay empty).
func (l *Log) SetIdentity(user, host string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.user, l.host = user, host
}

// NewLog opens (creating if needed) the log in dir, keyed by hmacKey, and
// recovers the chain head (last seq + hash) so a restart continues the chain.
func NewLog(dir string, hmacKey []byte) (*Log, error) {
	if len(hmacKey) == 0 {
		return nil, errors.New("audit: empty HMAC key")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	path := filepath.Join(dir, eventsFile)
	seq, prev, err := readTail(path)
	if err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, err
	}
	return &Log{dir: dir, f: f, key: append([]byte(nil), hmacKey...), seq: seq, prevHash: prev}, nil
}

// Record appends an event to the log: it stamps seq and prev_hash, computes the
// HMAC hash over the canonical form, writes the JSONL line, and fsyncs. In-memory
// chain state is advanced only after the durable write, so a failed write leaves
// the head unchanged. Record satisfies daemon.Recorder.
func (l *Log) Record(e schema.TelemetryEvent) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	next := l.seq + 1
	e.Seq = next
	e.PrevHash = l.prevHash
	if e.V == 0 {
		e.V = schema.SchemaVersion
	}
	// Stamp capture-time facts the harness payload doesn't carry, before hashing
	// so they're covered by the chain. Only fill what's absent — a shim that
	// spooled with its own hook-time timestamp keeps it.
	if e.TS == "" {
		e.TS = time.Now().UTC().Format(time.RFC3339Nano)
	}
	if e.User == "" {
		e.User = l.user
	}
	if e.Host == "" {
		e.Host = l.host
	}
	hash, err := hashEvent(e, l.key)
	if err != nil {
		return err
	}
	e.Hash = hash
	line, err := json.Marshal(e)
	if err != nil {
		return err
	}
	if _, err := l.f.Write(append(line, '\n')); err != nil {
		return err
	}
	if err := l.f.Sync(); err != nil {
		return err
	}

	l.seq = next
	l.prevHash = hash
	if l.sinceCkpt++; l.sinceCkpt >= checkpointEvery {
		l.sinceCkpt = 0
		_ = l.writeCheckpoint(next, hash) // best-effort anchor
	}
	return nil
}

// Close releases the underlying file.
func (l *Log) Close() error { return l.f.Close() }

// hashEvent computes HMAC-SHA256(key, canonical(e)). Canonical excludes the hash
// field (it can't sign itself) but includes seq and prev_hash, so tampering with
// any of them breaks verification.
func hashEvent(e schema.TelemetryEvent, key []byte) (string, error) {
	canon, err := e.Canonical()
	if err != nil {
		return "", err
	}
	mac := hmac.New(sha256.New, key)
	mac.Write(canon)
	return hex.EncodeToString(mac.Sum(nil)), nil
}

// readTail returns the last event's seq and hash, or (0,"") for a missing/empty
// log.
func readTail(path string) (uint64, string, error) {
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return 0, "", nil
	}
	if err != nil {
		return 0, "", err
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), maxLine)
	var seq uint64
	var prev string
	for sc.Scan() {
		var e schema.TelemetryEvent
		if err := json.Unmarshal(sc.Bytes(), &e); err != nil {
			return 0, "", fmt.Errorf("audit: corrupt log line during recovery: %w", err)
		}
		seq, prev = e.Seq, e.Hash
	}
	return seq, prev, sc.Err()
}

func (l *Log) writeCheckpoint(seq uint64, hash string) error {
	line, err := json.Marshal(map[string]any{"seq": seq, "hash": hash})
	if err != nil {
		return err
	}
	f, err := os.OpenFile(filepath.Join(l.dir, checkpointsFile), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(append(line, '\n'))
	return err
}
