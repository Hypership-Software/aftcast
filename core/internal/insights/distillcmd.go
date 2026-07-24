package insights

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/Hypership-Software/aftcast/internal/analytics"
	"github.com/Hypership-Software/aftcast/internal/audit"
	"github.com/Hypership-Software/aftcast/internal/telemetry"
)

// CoachDistill writes the bundle an operator hands to their own Claude to
// draft a skill from a recurring failure: transcript coordinates, not
// transcript content. A tainted session's coordinates are never offered —
// skill persistence is injection persistence, so a skill drafted from a
// session that touched untrusted input would persist whatever that source
// injected. The same reasoning extends to the record itself: a broken HMAC
// chain means the taint gate's own inputs can't be trusted, so a broken
// chain refuses coordinates and the drafting scaffold exactly like an
// all-tainted cluster does.
func CoachDistill(store *telemetry.Store, slug string, rep audit.Report, w io.Writer, now time.Time) error {
	clusters, err := windowedClusters(store, now)
	if err != nil {
		return err
	}
	var target analytics.FrictionCluster
	found := false
	for _, c := range clusters {
		if c.Slug() == slug {
			target, found = c, true
			break
		}
	}
	if !found {
		return fmt.Errorf("nothing matches %q this week — run aftcast coach to see what's worth fixing", slug)
	}

	fmt.Fprintln(w, "# Distill a skill from a recurring failure")
	fmt.Fprintln(w)
	writeDistillAttestation(w, rep)
	if !rep.OK {
		fmt.Fprintln(w, "This cluster cannot be distilled: the record that would supply its coordinates just failed verification.")
		return nil
	}

	sessions, err := store.Sessions()
	if err != nil {
		return fmt.Errorf("coach distill: load sessions: %w", err)
	}
	// A resumed id owns several folded sessions and failures are attributed by raw
	// id, so taint accumulates across an id's runs: a later clean run must not clear
	// an earlier tainted one and readmit it to the scaffold.
	taintByID := make(map[string]bool, len(sessions))
	for _, s := range sessions {
		taintByID[s.SessionID] = taintByID[s.SessionID] || s.Taint
	}

	var clean, tainted []analytics.SessionFailures
	for _, s := range target.Sessions {
		if taintByID[s.SessionID] {
			tainted = append(tainted, s)
		} else {
			clean = append(clean, s)
		}
	}

	fmt.Fprintf(w, "Your agents %s %d times across %s %s.\n", describeFriction(target), target.Failures,
		countNoun(len(target.Sessions), "session", "sessions"), bundleDates(target))
	fmt.Fprintln(w, "This bundle contains counts, dates, and session and prompt references only —")
	fmt.Fprintln(w, "no commands or content were captured.")
	fmt.Fprintln(w)

	if len(clean) > 0 {
		fmt.Fprintln(w, "## Coordinates")
		fmt.Fprintln(w)
		for _, s := range clean {
			// The full session id is what a reader substitutes into <session-id> in
			// the transcript template below — shortID's 8-char prefix can't resolve
			// to a real transcript filename, so this is the one place shortID must
			// not be used.
			fmt.Fprintf(w, "- session %s — %s — transcript at ~/.claude/projects/<project-dir>/<session-id>.jsonl\n",
				s.SessionID, promptCoordinates(s.Failures, s.PromptIDs))
		}
		fmt.Fprintln(w)
	}

	if len(tainted) > 0 {
		fmt.Fprintln(w, "## Excluded — untrusted input")
		fmt.Fprintln(w)
		for _, s := range tainted {
			fmt.Fprintf(w, "- session %s touched untrusted input and is excluded: a skill drafted from it\n", shortID(s.SessionID))
			fmt.Fprintln(w, "  would persist whatever the untrusted source injected.")
		}
		fmt.Fprintln(w)
	}

	if len(clean) == 0 {
		fmt.Fprintln(w, "Every session in this cluster carries taint, so this cluster cannot be distilled.")
		return nil
	}

	fmt.Fprintln(w, "## Drafting scaffold")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Instructions for the user's own Claude:")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "1. Read the transcripts at the coordinates above.")
	fmt.Fprintln(w, "2. Identify what the human's corrections had in common.")
	fmt.Fprintln(w, "3. Draft a SKILL.md that prevents this failure class.")
	fmt.Fprintf(w, "4. Include a provenance line naming the %q cluster and its date range (%s).\n", slug, bundleDates(target))
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Transcript content stays on this machine — only the drafted skill leaves this")
	fmt.Fprintln(w, "step, and the author reviews the draft before adopting it.")
	return nil
}

func writeDistillAttestation(w io.Writer, rep audit.Report) {
	fmt.Fprintln(w, "## Attestation")
	fmt.Fprintln(w)
	if !rep.OK {
		fmt.Fprintf(w, "**ATTESTATION FAILED:** the record's HMAC chain broke at record %d (%s). Nothing in\n", rep.BadSeq, rep.Detail)
		fmt.Fprintln(w, "this bundle can be trusted.")
		fmt.Fprintln(w)
		return
	}
	fmt.Fprintf(w, "The record's HMAC chain is intact: chain verified across %d records. Within the\n", rep.Count)
	fmt.Fprintln(w, "observed sessions, these coordinates are attested.")
	fmt.Fprintln(w)
}

// promptCoordinates describes a session's failures alongside the prompts
// that caused them. A session can fail before Aftcast starts recording
// prompt ids (or on a harness that doesn't emit them), so an empty
// PromptIDs is a real, expected case — not an error — and gets its own
// plain sentence rather than an empty "at prompts " fragment.
func promptCoordinates(failures int, promptIDs []string) string {
	if len(promptIDs) == 0 {
		return countNoun(failures, "failure", "failures") + "; its failures carry no prompt references"
	}
	word := "prompt"
	if len(promptIDs) > 1 {
		word = "prompts"
	}
	return fmt.Sprintf("%s at %s %s", countNoun(failures, "failure", "failures"), word, strings.Join(promptIDs, ", "))
}
