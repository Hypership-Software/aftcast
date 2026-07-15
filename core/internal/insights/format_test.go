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
	cases := map[int64]string{
		0:       "",
		5:       "<1s",
		999:     "<1s",
		1000:    "1s",
		9109:    "9s",
		65000:   "1m 5s",
		103190:  "1m 43s",
		6240000: "1h 44m",
	}
	for in, want := range cases {
		if got := humanizeDuration(in); got != want {
			t.Errorf("humanizeDuration(%d) = %q, want %q", in, got, want)
		}
	}
}
