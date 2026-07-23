package handoff

import (
	"fmt"
	"strings"

	"github.com/Hypership-Software/aftcast/internal/audit"
)

// Render emits the digest skeleton. Everything below the narrative sections is
// code-generated; the narrative sections carry instructions + coordinates only,
// so evidence never launders through a model and content never enters Aftcast.
func Render(ref string, facts []SessionFacts, rep audit.Report) []byte {
	var b strings.Builder
	fmt.Fprintf(&b, "# Handoff digest — %s\n\n", ref)

	ids := make([]string, 0, len(facts))
	for _, f := range facts {
		ids = append(ids, f.ID)
	}

	narration := func(section string) {
		fmt.Fprintf(&b, "## %s\n\n", section)
		if len(facts) == 0 {
			b.WriteString("No captured session joins to this ref, so there is nothing to narrate.\n\n")
			return
		}
		b.WriteString("> **Narration instructions (for the author's own Claude):** read your local\n")
		b.WriteString("> transcripts for the sessions listed below (they live under\n")
		b.WriteString("> `~/.claude/projects/<project-dir>/<session-id>.jsonl` on the author's\n")
		fmt.Fprintf(&b, "> machine) and write the %s section in plain English.\n", section)
		b.WriteString("> Transcript content stays on this machine — only your written summary\n")
		b.WriteString("> enters the digest, and the author reviews it before sharing.\n>\n")
		fmt.Fprintf(&b, "> Sessions: %s\n\n", strings.Join(ids, ", "))
	}
	narration("Intent")
	narration("Journey")

	b.WriteString("## Sessions\n\n")
	if len(facts) == 0 {
		b.WriteString("No captured session recorded a commit on this ref. Sessions recorded before\n")
		b.WriteString("commit capture existed cannot join and are not listed.\n\n")
	}
	for _, f := range facts {
		fmt.Fprintf(&b, "Session `%s` ran from %s to %s and recorded %d events across %d prompts. ",
			f.ID, f.Started, f.Ended, f.Events, f.Prompts)
		if len(f.CommitSHAs) > 0 {
			fmt.Fprintf(&b, "It committed %s. ", strings.Join(f.CommitSHAs, ", "))
		}
		fmt.Fprintf(&b, "It sent %d delivery signals and had %d failed tool calls. ",
			f.Deliveries, f.Failures)
		if len(f.Skills) > 0 {
			fmt.Fprintf(&b, "Skills invoked: %s. ", strings.Join(f.Skills, ", "))
		}
		if f.MaxContext > 0 {
			fmt.Fprintf(&b, "Peak observed context: %d tokens. ", f.MaxContext)
		}
		if len(f.DangerRules) > 0 {
			fmt.Fprintf(&b, "Flagged operations: %s. ", strings.Join(f.DangerRules, ", "))
		}
		b.WriteString("\n\n")
	}

	b.WriteString("## Attestation\n\n")
	if rep.OK {
		fmt.Fprintf(&b, "The record's HMAC chain is intact: chain verified across %s records.\n\n", thousands(rep.Count))
	} else {
		fmt.Fprintf(&b, "**ATTESTATION FAILED:** the record's HMAC chain broke at record %d (%s). Nothing below can be trusted.\n\n", rep.BadSeq, rep.Detail)
	}
	b.WriteString("Within the observed sessions: ")
	review := reviewShaped(facts)
	if len(review) == 0 {
		b.WriteString("no review agent or review skill ran. ")
	} else {
		fmt.Fprintf(&b, "review-shaped activity ran (%s). ", strings.Join(review, ", "))
	}
	modes := uniqueModes(facts)
	if len(modes) > 0 {
		fmt.Fprintf(&b, "Permission modes seen: %s. ", strings.Join(modes, ", "))
	}
	if tainted(facts) {
		b.WriteString("At least one session carried taint from an untrusted source. ")
	} else if len(facts) > 0 {
		b.WriteString("No session taint was recorded. ")
	}
	b.WriteString("\n\n")
	b.WriteString("Not captured: anything outside the observed sessions — a review on another\n")
	b.WriteString("machine or in a web UI is invisible to this record and is neither confirmed\n")
	b.WriteString("nor denied here.\n")

	return []byte(b.String())
}

func reviewShaped(facts []SessionFacts) []string {
	var out []string
	for _, f := range facts {
		for _, name := range append(append([]string{}, f.Subagents...), f.Skills...) {
			if strings.Contains(strings.ToLower(name), "review") {
				out = append(out, name)
			}
		}
	}
	return out
}

func uniqueModes(facts []SessionFacts) []string {
	seen := map[string]bool{}
	var out []string
	for _, f := range facts {
		for _, m := range f.PermissionModes {
			if !seen[m] {
				seen[m] = true
				out = append(out, m)
			}
		}
	}
	return out
}

func tainted(facts []SessionFacts) bool {
	for _, f := range facts {
		if f.Tainted {
			return true
		}
	}
	return false
}

func thousands(n int) string {
	s := fmt.Sprintf("%d", n)
	if len(s) <= 3 {
		return s
	}
	var parts []string
	for len(s) > 3 {
		parts = append([]string{s[len(s)-3:]}, parts...)
		s = s[:len(s)-3]
	}
	return s + "," + strings.Join(parts, ",")
}
