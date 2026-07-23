// Package transcript derives numeric context-usage samples from a harness's
// local transcript file. Only derived token counts leave this package — the
// transcript's content never does (ADR-011).
package transcript

import (
	"encoding/json"
	"io"
	"os"
	"strings"
)

// tailWindow bounds the read: the newest assistant entry is what the sample
// wants, and it lives at the end of the file.
const tailWindow = 256 * 1024

type entry struct {
	Type    string `json:"type"`
	Message struct {
		Usage *struct {
			InputTokens         int64 `json:"input_tokens"`
			CacheCreationTokens int64 `json:"cache_creation_input_tokens"`
			CacheReadTokens     int64 `json:"cache_read_input_tokens"`
			OutputTokens        int64 `json:"output_tokens"`
		} `json:"usage"`
	} `json:"message"`
}

// ContextTokens returns the context-window occupancy after the transcript's
// newest assistant turn: that request's input + cache tokens plus its output.
// Best-effort by design — a missing, truncated, or unrecognized transcript
// samples 0, and 0 is never recorded (omitempty); a missed sample breaks
// nothing.
func ContextTokens(path string) int64 {
	if path == "" {
		return 0
	}
	f, err := os.Open(path)
	if err != nil {
		return 0
	}
	defer f.Close()

	size, err := f.Seek(0, io.SeekEnd)
	if err != nil {
		return 0
	}
	start := size - tailWindow
	if start < 0 {
		start = 0
	}
	if _, err := f.Seek(start, io.SeekStart); err != nil {
		return 0
	}
	buf, err := io.ReadAll(f)
	if err != nil {
		return 0
	}

	lines := strings.Split(string(buf), "\n")
	if start > 0 && len(lines) > 0 {
		lines = lines[1:]
	}
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimRight(lines[i], "\r")
		if line == "" {
			continue
		}
		var e entry
		if json.Unmarshal([]byte(line), &e) != nil {
			continue
		}
		if e.Type != "assistant" || e.Message.Usage == nil {
			continue
		}
		u := e.Message.Usage
		total := u.InputTokens + u.CacheCreationTokens + u.CacheReadTokens + u.OutputTokens
		if total == 0 {
			continue
		}
		return total
	}
	return 0
}
