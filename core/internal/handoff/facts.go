package handoff

import "github.com/Hypership-Software/aftcast/internal/schema"

type SessionFacts struct {
	ID              string
	Started, Ended  string
	Events          int
	Prompts         int
	Failures        int
	Deliveries      int
	CommitSHAs      []string
	DangerRules     []string
	Skills          []string
	Subagents       []string
	PermissionModes []string
	MaxContext      int64
	Tainted         bool
}

func GatherFacts(sel []Selected) []SessionFacts {
	out := make([]SessionFacts, 0, len(sel))
	for _, s := range sel {
		f := SessionFacts{
			ID:         s.Session.SessionID,
			Started:    s.Session.Started,
			Ended:      s.Session.Ended,
			Events:     len(s.Events),
			CommitSHAs: s.SHAs,
		}
		danger, skills, subs, modes := map[string]bool{}, map[string]bool{}, map[string]bool{}, map[string]bool{}
		for _, e := range s.Events {
			switch e.EventType {
			case schema.EventUserPrompt:
				f.Prompts++
			case schema.EventPostTool:
				if e.ToolOK == schema.OutcomeFailed {
					f.Failures++
				}
			}
			if e.DeliverySignal != "" {
				f.Deliveries++
			}
			if e.Risk == schema.RiskDanger && e.RuleID != "" && !danger[e.RuleID] {
				danger[e.RuleID] = true
				f.DangerRules = append(f.DangerRules, e.RuleID)
			}
			if e.Skill != "" && !skills[e.Skill] {
				skills[e.Skill] = true
				f.Skills = append(f.Skills, e.Skill)
			}
			if e.Subagent != "" && !subs[e.Subagent] {
				subs[e.Subagent] = true
				f.Subagents = append(f.Subagents, e.Subagent)
			}
			if e.PermissionMode != "" && !modes[e.PermissionMode] {
				modes[e.PermissionMode] = true
				f.PermissionModes = append(f.PermissionModes, e.PermissionMode)
			}
			if e.ContextTokens > f.MaxContext {
				f.MaxContext = e.ContextTokens
			}
			if e.Taint {
				f.Tainted = true
			}
		}
		out = append(out, f)
	}
	return out
}
