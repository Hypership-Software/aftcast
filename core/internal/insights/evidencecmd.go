package insights

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/Hypership-Software/aftcast/internal/audit"
	"github.com/Hypership-Software/aftcast/internal/telemetry"
)

// EvidenceReport writes the period evidence document: per-repository facts
// computed from the record, attested via rep, and a narrative scaffold for
// the author's own Claude to turn into prose. Content is metadata-only per
// ADR-011 — counts, dates, rates, repository names, and session references,
// never prompts, code, or command content. A broken HMAC chain means every
// fact downstream of the record can't be trusted, so — same rule as
// CoachDistill — a broken chain refuses everything after the attestation
// banner: no repository rows, no scaffold.
func EvidenceReport(store *telemetry.Store, since time.Time, rep audit.Report, w io.Writer, now time.Time) error {
	fmt.Fprintln(w, "# Evidence of agent work")
	fmt.Fprintln(w)
	writeEvidenceAttestation(w, rep)
	if !rep.OK {
		fmt.Fprintln(w, "This document cannot be produced: the record that would supply it just failed verification.")
		return nil
	}

	sessions, err := store.Sessions()
	if err != nil {
		return fmt.Errorf("evidence: load sessions: %w", err)
	}
	rows := EvidenceRows(sessions, since, now)

	fmt.Fprintf(w, "This document covers %s to %s.\n", bundleDay(since), bundleDay(now))
	fmt.Fprintln(w, "It contains counts, dates, rates, repository names, and session references")
	fmt.Fprintln(w, "only — no prompts, code, or command content is captured.")
	fmt.Fprintln(w)

	if len(rows) == 0 {
		fmt.Fprintln(w, "No captured sessions started in this period.")
		return nil
	}

	for _, r := range rows {
		writeRepoParagraph(w, r)
	}
	writeEvidenceScaffold(w, rows)
	return nil
}

func writeEvidenceAttestation(w io.Writer, rep audit.Report) {
	fmt.Fprintln(w, "## Attestation")
	fmt.Fprintln(w)
	if !rep.OK {
		fmt.Fprintf(w, "**ATTESTATION FAILED:** the record's HMAC chain broke at record %d (%s). Nothing in\n", rep.BadSeq, rep.Detail)
		fmt.Fprintln(w, "this document can be trusted.")
		fmt.Fprintln(w)
		return
	}
	fmt.Fprintf(w, "The record's HMAC chain is intact: chain verified across %d records. Within the\n", rep.Count)
	fmt.Fprintln(w, "observed sessions, this document is attested.")
	fmt.Fprintln(w)
}

func writeRepoParagraph(w io.Writer, r RepoEvidence) {
	fmt.Fprintf(w, "## %s\n\n", r.Repo)
	fmt.Fprintf(w, "%s in this period, shipped in %d of %d sessions. %s and %s.\n",
		countNoun(r.Sessions, "session", "sessions"), r.Shipped, r.Sessions,
		countNoun(r.Corrections, "correction", "corrections"),
		countNoun(r.FilesChanged, "file changed", "files changed"))
	if r.Danger > 0 {
		fmt.Fprintln(w, dangerSentence(r.Danger))
	}
	if r.Tainted > 0 {
		fmt.Fprintln(w, taintSentence(r.Tainted))
	}
	fmt.Fprintln(w)
}

func dangerSentence(n int) string {
	verb := "were recorded"
	if n == 1 {
		verb = "was recorded"
	}
	return fmt.Sprintf("%s %s.", countNoun(n, "flagged operation", "flagged operations"), verb)
}

func taintSentence(n int) string {
	return fmt.Sprintf("%s carried taint from an untrusted source.", countNoun(n, "session", "sessions"))
}

func writeEvidenceScaffold(w io.Writer, rows []RepoEvidence) {
	fmt.Fprintln(w, "## Narrative scaffold")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Instructions for the author's own Claude:")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "1. For each repository above, read the listed sessions' local transcripts.")
	fmt.Fprintln(w, "2. Write one paragraph per repository: what was worked on, what shipped, and")
	fmt.Fprintln(w, "   what needed a human.")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Coordinates:")
	fmt.Fprintln(w)
	for _, r := range rows {
		word := "sessions"
		if len(r.SessionIDs) == 1 {
			word = "session"
		}
		fmt.Fprintf(w, "- %s — %s %s\n", r.Repo, word, strings.Join(r.SessionIDs, ", "))
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Transcript content stays on this machine — only the narrative you write leaves")
	fmt.Fprintln(w, "this step, and the author reviews it before sharing this document.")
}
