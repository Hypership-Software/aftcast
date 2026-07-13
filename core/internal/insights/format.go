package insights

import (
	"fmt"
	"time"
)

func humanize(tsStr string, now time.Time) string {
	if tsStr == "" {
		return ""
	}
	t, err := time.Parse(time.RFC3339Nano, tsStr)
	if err != nil {
		return ""
	}
	d := now.Sub(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}

func humanizeDuration(ms int64) string {
	switch {
	case ms <= 0:
		return ""
	case ms < 60000:
		return fmt.Sprintf("%.1fs", float64(ms)/1000)
	default:
		return fmt.Sprintf("%dm", ms/60000)
	}
}
