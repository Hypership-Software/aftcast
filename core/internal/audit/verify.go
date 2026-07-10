package audit

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/Hypership-Software/atlas/internal/schema"
)

// Report is the outcome of verifying the chain.
type Report struct {
	OK     bool
	Count  int
	BadSeq uint64 // the seq at which verification first failed (0 if OK)
	Detail string
}

// Verify replays the on-disk log and checks every record: monotonic seq,
// prev_hash linkage, and HMAC hash. It stops at and reports the first break.
func (l *Log) Verify() (Report, error) {
	f, err := os.Open(filepath.Join(l.dir, eventsFile))
	if errors.Is(err, os.ErrNotExist) {
		return Report{OK: true}, nil
	}
	if err != nil {
		return Report{}, err
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), maxLine)
	var prev string
	var expected uint64
	count := 0
	for sc.Scan() {
		var e schema.TelemetryEvent
		if err := json.Unmarshal(sc.Bytes(), &e); err != nil {
			return Report{OK: false, Count: count, BadSeq: expected + 1, Detail: "unparseable log line"}, nil
		}
		expected++
		if e.Seq != expected {
			return Report{OK: false, Count: count, BadSeq: e.Seq, Detail: fmt.Sprintf("seq out of order: want %d, got %d", expected, e.Seq)}, nil
		}
		if e.PrevHash != prev {
			return Report{OK: false, Count: count, BadSeq: e.Seq, Detail: "prev_hash does not chain to the previous record"}, nil
		}
		want, err := hashEvent(e, l.key)
		if err != nil {
			return Report{}, err
		}
		if e.Hash != want {
			return Report{OK: false, Count: count, BadSeq: e.Seq, Detail: "hash mismatch (record was altered)"}, nil
		}
		prev = e.Hash
		count++
	}
	if err := sc.Err(); err != nil {
		return Report{}, err
	}
	return Report{OK: true, Count: count}, nil
}

// Export streams the log as NDJSON, emitting only records at or after since.
// Records with an unparseable timestamp are included (fail open on export — a
// missing filter must not silently drop audit data).
func (l *Log) Export(w io.Writer, since time.Time) error {
	f, err := os.Open(filepath.Join(l.dir, eventsFile))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), maxLine)
	for sc.Scan() {
		var e schema.TelemetryEvent
		if err := json.Unmarshal(sc.Bytes(), &e); err != nil {
			return err
		}
		if e.TS != "" {
			if ts, perr := time.Parse(time.RFC3339Nano, e.TS); perr == nil && ts.Before(since) {
				continue
			}
		}
		if _, err := w.Write(sc.Bytes()); err != nil {
			return err
		}
		if _, err := w.Write([]byte{'\n'}); err != nil {
			return err
		}
	}
	return sc.Err()
}
