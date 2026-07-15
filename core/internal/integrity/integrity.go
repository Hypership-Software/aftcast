// Package integrity detects tampering with the gate's own install — missing hook
// entries or a changed binary. Any Drift becomes an integrity telemetry event
// plus a loud warning, surfacing that the observer itself may have been tampered
// with (an agent trying to blind its own monitoring).
package integrity

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"strings"

	"github.com/Hypership-Software/aftcast/internal/schema"
)

type DriftKind string

const (
	DriftHookMissing   DriftKind = "hook_missing"
	DriftBinaryChanged DriftKind = "binary_changed"
)

type Drift struct {
	Kind   DriftKind
	Detail string
}

// Config points the checker at what to verify. An empty field skips that check.
type Config struct {
	SettingsPath string // harness settings file that must contain our hook entries
	HookMarker   string // substring our hook entries carry (e.g. the daemon URL or "gated hook")
	BinaryPath   string // the running gated binary
	BinaryHash   string // expected SHA-256 hex from the install manifest
}

type Checker struct{ cfg Config }

func NewChecker(cfg Config) *Checker { return &Checker{cfg: cfg} }

// Check returns every detected drift (nil means intact). Drift is data, not an
// error, so the daemon can record and warn on each item.
func (c *Checker) Check() []Drift {
	var drift []Drift

	if c.cfg.SettingsPath != "" {
		data, err := os.ReadFile(c.cfg.SettingsPath)
		switch {
		case err != nil:
			drift = append(drift, Drift{DriftHookMissing, "settings file unreadable: " + c.cfg.SettingsPath})
		case c.cfg.HookMarker != "" && !strings.Contains(string(data), c.cfg.HookMarker):
			drift = append(drift, Drift{DriftHookMissing, "gate hook entry absent from " + c.cfg.SettingsPath})
		}
	}

	if c.cfg.BinaryPath != "" && c.cfg.BinaryHash != "" {
		sum, err := hashFile(c.cfg.BinaryPath)
		switch {
		case err != nil:
			drift = append(drift, Drift{DriftBinaryChanged, "cannot hash the running binary: " + err.Error()})
		case sum != c.cfg.BinaryHash:
			drift = append(drift, Drift{DriftBinaryChanged, "running binary hash does not match the install manifest"})
		}
	}

	return drift
}

// DriftEvent renders a drift as an integrity telemetry event. Detail goes to the
// daemon's warning; the append-only log carries the kind in rule_id.
func DriftEvent(d Drift) schema.TelemetryEvent {
	return schema.TelemetryEvent{
		V:         schema.SchemaVersion,
		EventType: schema.EventIntegrity,
		RuleID:    string(d.Kind),
	}
}

func hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
