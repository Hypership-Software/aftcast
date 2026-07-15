package insights

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/Hypership-Software/aftcast/internal/analytics"
	"github.com/Hypership-Software/aftcast/internal/telemetry"
	"github.com/Hypership-Software/aftcast/internal/ui"
)

// CoachReport prints what keeps failing and is worth a permanent fix — the
// non-TUI twin of the overview card, for `gated coach`.
func CoachReport(store *telemetry.Store, w io.Writer, now time.Time) error {
	clusters, err := windowedClusters(store, now)
	if err != nil {
		return err
	}
	worth := analytics.WorthFixing(clusters)
	if len(worth) == 0 {
		fmt.Fprintln(w, "Nothing has crossed the worth-a-permanent-fix line this week.")
		fmt.Fprintln(w, ui.Hint("Aftcast raises a hand when the same kind of failure shows up in 3+ sessions on 2+ days."))
		return nil
	}
	fmt.Fprintln(w, "Worth a permanent fix · across your projects · last 7 days")
	fmt.Fprintln(w)
	for _, c := range worth {
		fmt.Fprintf(w, "  %s\n", frictionLine(c))
		fmt.Fprintf(w, "    %s\n", ui.Hint("→ gated coach export "+c.Slug()))
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, ui.Hint("Export a bundle and hand it to your agent to encode the fix."))
	return nil
}

// CoachExport writes one fingerprint's evidence bundle: plain-English prose an
// operator can read and an agent can turn into a fix. Receipts only — counts,
// dates, session references. Never command content.
func CoachExport(store *telemetry.Store, slug string, w io.Writer, now time.Time) error {
	clusters, err := windowedClusters(store, now)
	if err != nil {
		return err
	}
	for _, c := range clusters {
		if c.Slug() == slug {
			writeBundle(w, c)
			return nil
		}
	}
	return fmt.Errorf("nothing matches %q this week — run gated coach to see what's worth fixing", slug)
}

func windowedClusters(store *telemetry.Store, now time.Time) ([]analytics.FrictionCluster, error) {
	failures, err := store.FailedCalls()
	if err != nil {
		return nil, fmt.Errorf("coach: load failed calls: %w", err)
	}
	return analytics.FrictionClusters(frictionWindow(failures, now)), nil
}

func writeBundle(w io.Writer, c analytics.FrictionCluster) {
	fmt.Fprintln(w, "# Worth a permanent fix")
	fmt.Fprintln(w)
	fmt.Fprintf(w, "Your agents %s %d times across %s %s.\n", describeFriction(c), c.Failures,
		countNoun(len(c.Sessions), "session", "sessions"), bundleDates(c))
	fmt.Fprintln(w, "Aftcast records what happened, not what was typed: this bundle contains")
	fmt.Fprintln(w, "counts, dates, and session references only. No commands were captured.")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "## The sessions")
	fmt.Fprintln(w)
	for _, s := range c.Sessions {
		fmt.Fprintf(w, "- %s — session %s, %s\n", bundleDay(s.First), shortID(s.SessionID),
			countNoun(s.Failures, "failure", "failures"))
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "## What to do with this")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Encode the fix at the strongest rung that applies:")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "1. Fix the root cause — a product or environment change")
	fmt.Fprintln(w, "2. A test that fails if the problem comes back")
	fmt.Fprintln(w, "3. A CI gate")
	fmt.Fprintln(w, "4. A skill your agents load")
	fmt.Fprintln(w, "5. A rule in CLAUDE.md")
}

func bundleDates(c analytics.FrictionCluster) string {
	first, last := bundleDay(c.First), bundleDay(c.Last)
	if first == last {
		return "on " + first
	}
	return "between " + first + " and " + last
}

func bundleDay(t time.Time) string {
	if t.IsZero() {
		return "an unrecorded date"
	}
	return strings.TrimSpace(t.Format("January 2"))
}
