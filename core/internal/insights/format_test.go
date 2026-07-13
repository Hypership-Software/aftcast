package insights

import (
	"testing"
	"time"
)

func TestHumanize(t *testing.T) {
	now := time.Date(2026, 7, 13, 15, 0, 0, 0, time.UTC)
	cases := map[string]string{
		"2026-07-13T14:59:30Z": "just now",
		"2026-07-13T14:42:00Z": "18m ago",
		"2026-07-13T13:00:00Z": "2h ago",
		"2026-07-10T15:00:00Z": "3d ago",
		"":                     "",
		"not-a-time":           "",
	}
	for in, want := range cases {
		if got := humanize(in, now); got != want {
			t.Errorf("humanize(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestHumanizeDuration(t *testing.T) {
	cases := map[int64]string{0: "", 250: "0.2s", 9109: "9.1s", 65000: "1m", 1080000: "18m"}
	for in, want := range cases {
		if got := humanizeDuration(in); got != want {
			t.Errorf("humanizeDuration(%d) = %q, want %q", in, got, want)
		}
	}
}
