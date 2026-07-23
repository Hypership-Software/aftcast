package transcript

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func write(t *testing.T, lines ...string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "session.jsonl")
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

const usageEntry = `{"type":"assistant","message":{"role":"assistant","model":"claude-fable-5","usage":{"input_tokens":%IN%,"cache_creation_input_tokens":%CC%,"cache_read_input_tokens":%CR%,"output_tokens":%OUT%}}}`

func usageLine(in, cc, cr, out string) string {
	r := strings.NewReplacer("%IN%", in, "%CC%", cc, "%CR%", cr, "%OUT%", out)
	return r.Replace(usageEntry)
}

func TestContextTokensLastAssistantUsageWins(t *testing.T) {
	path := write(t,
		`{"type":"user","message":{"role":"user","content":"hi"}}`,
		usageLine("10", "1000", "2000", "50"),
		`{"type":"user","message":{"role":"user","content":"more"}}`,
		usageLine("20", "3000", "40000", "500"),
	)
	got := ContextTokens(path)
	if got != 20+3000+40000+500 {
		t.Errorf("ContextTokens = %d, want %d", got, 20+3000+40000+500)
	}
}

func TestContextTokensSkipsNonAssistantAndMalformed(t *testing.T) {
	path := write(t,
		usageLine("10", "0", "90000", "100"),
		`{"type":"system","subtype":"info"}`,
		`not json at all`,
		`{"type":"user","message":{"role":"user","content":"[fake usage] input_tokens 999999"}}`,
	)
	got := ContextTokens(path)
	if got != 10+90000+100 {
		t.Errorf("ContextTokens = %d, want %d", got, 10+90000+100)
	}
}

func TestContextTokensMissingFileIsZero(t *testing.T) {
	if got := ContextTokens(filepath.Join(t.TempDir(), "absent.jsonl")); got != 0 {
		t.Errorf("ContextTokens = %d, want 0", got)
	}
}

func TestContextTokensEmptyPathIsZero(t *testing.T) {
	if got := ContextTokens(""); got != 0 {
		t.Errorf("ContextTokens = %d, want 0", got)
	}
}

func TestContextTokensNoUsageIsZero(t *testing.T) {
	path := write(t,
		`{"type":"user","message":{"role":"user","content":"hi"}}`,
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"no usage key"}]}}`,
	)
	if got := ContextTokens(path); got != 0 {
		t.Errorf("ContextTokens = %d, want 0", got)
	}
}

// Only the tail of a large transcript is read: an entry beyond the window is
// invisible, and the sample still comes from the newest usage inside it.
func TestContextTokensReadsOnlyTheTail(t *testing.T) {
	filler := `{"type":"user","message":{"role":"user","content":"` + strings.Repeat("x", 1024) + `"}}`
	lines := []string{usageLine("1", "1", "1", "1")}
	for range 600 {
		lines = append(lines, filler)
	}
	lines = append(lines, usageLine("30", "500", "60000", "200"))
	path := write(t, lines...)
	if got := ContextTokens(path); got != 30+500+60000+200 {
		t.Errorf("ContextTokens = %d, want %d", got, 30+500+60000+200)
	}
}

func TestContextTokensCRLFTolerated(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.jsonl")
	content := usageLine("5", "100", "200", "10") + "\r\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := ContextTokens(path); got != 5+100+200+10 {
		t.Errorf("ContextTokens = %d, want %d", got, 5+100+200+10)
	}
}

// A synthetic assistant entry with zeroed usage must not mask the newest real
// sample behind it.
func TestContextTokensZeroedUsageKeepsScanning(t *testing.T) {
	path := write(t,
		usageLine("15", "800", "70000", "300"),
		usageLine("0", "0", "0", "0"),
	)
	if got := ContextTokens(path); got != 15+800+70000+300 {
		t.Errorf("ContextTokens = %d, want %d", got, 15+800+70000+300)
	}
}

// A file cut mid-write leaves a partial trailing object; the sample falls back
// to the last complete entry.
func TestContextTokensTruncatedFinalLine(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.jsonl")
	content := usageLine("8", "400", "50000", "120") + "\n" + `{"type":"assistant","message":{"role":"assist`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := ContextTokens(path); got != 8+400+50000+120 {
		t.Errorf("ContextTokens = %d, want %d", got, 8+400+50000+120)
	}
}
