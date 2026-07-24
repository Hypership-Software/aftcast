package telemetry

import (
	"encoding/json"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Hypership-Software/aftcast/internal/analytics"
	"github.com/Hypership-Software/aftcast/internal/audit"
	"github.com/Hypership-Software/aftcast/internal/project"
	"github.com/Hypership-Software/aftcast/internal/schema"
)

// Project folds the audit log into the read-model, upserting session summaries
// and mirroring events. Idempotent (sessions keyed by session_id, events by seq)
// and rebuildable. A last-projected-seq watermark short-circuits when nothing is
// new. Both the structural columns and the analytical columns (outcome, clean_delivery,
// correction_turns, task_type — computed by the analytics package) are written.
func (s *Store) Project(log *audit.Log) error {
	evs, err := log.Events()
	if err != nil {
		return err
	}
	if len(evs) == 0 {
		return nil
	}
	maxSeq := evs[len(evs)-1].Seq
	if maxSeq <= s.lastProjectedSeq() {
		return nil // already projected through here
	}

	// Watermark tracks the full log (so a filtered marker as the last event still
	// advances it); the read-model itself holds only real agent sessions.
	visible := make([]schema.TelemetryEvent, 0, len(evs))
	for _, e := range evs {
		if !schema.IsInternalSession(e.SessionID) {
			visible = append(visible, e)
		}
	}

	// Fold first so the event mirror can be filtered to the same set of real
	// sessions the summary table holds — an empty shell contributes no session
	// row, so mirroring its events would leave orphaned rows no consumer reads.
	folded := foldSessions(visible)
	real := make(map[string]struct{}, len(folded))
	for i := range folded {
		real[folded[i].SessionID] = struct{}{}
	}

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	insEvent, err := tx.Prepare(`INSERT OR IGNORE INTO events (seq, session_id, turn_index, event_type, ts, raw) VALUES (?,?,?,?,?,?)`)
	if err != nil {
		return err
	}
	defer insEvent.Close()
	for _, e := range visible {
		if _, ok := real[e.SessionID]; !ok {
			continue
		}
		raw, err := json.Marshal(e)
		if err != nil {
			return err
		}
		if _, err := insEvent.Exec(e.Seq, e.SessionID, e.TurnIndex, string(e.EventType), e.TS, string(raw)); err != nil {
			return err
		}
	}

	upsert, err := tx.Prepare(`INSERT INTO sessions
		(key, first_seq, last_seq, session_id, user, org, harness, started, ended, exit_reason,
		 turn_count, tool_calls, danger_detected, taint, skills_used, duration_ms,
		 files_touched, files_changed, shipped, capture_version, plan_style,
		 outcome, clean_delivery, correction_turns, task_type, project_id, project_name,
		 changed_files, lines_added, lines_removed, change_stats_covered, observed_tool_ms,
		 plan_ms, build_ms, review_ms, work_mix_covered)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(key) DO UPDATE SET
			first_seq=excluded.first_seq, last_seq=excluded.last_seq,
			session_id=excluded.session_id,
			user=excluded.user, org=excluded.org, harness=excluded.harness,
			started=excluded.started, ended=excluded.ended, exit_reason=excluded.exit_reason,
			turn_count=excluded.turn_count, tool_calls=excluded.tool_calls,
			danger_detected=excluded.danger_detected, taint=excluded.taint,
			skills_used=excluded.skills_used, duration_ms=excluded.duration_ms,
			files_touched=excluded.files_touched, files_changed=excluded.files_changed,
			shipped=excluded.shipped, capture_version=excluded.capture_version,
			plan_style=excluded.plan_style,
			outcome=excluded.outcome, clean_delivery=excluded.clean_delivery,
			correction_turns=excluded.correction_turns, task_type=excluded.task_type,
			project_id=excluded.project_id, project_name=excluded.project_name,
			changed_files=excluded.changed_files, lines_added=excluded.lines_added,
			lines_removed=excluded.lines_removed, change_stats_covered=excluded.change_stats_covered,
			observed_tool_ms=excluded.observed_tool_ms, plan_ms=excluded.plan_ms,
			build_ms=excluded.build_ms, review_ms=excluded.review_ms,
			work_mix_covered=excluded.work_mix_covered`)
	if err != nil {
		return err
	}
	defer upsert.Close()
	for _, sess := range folded {
		changedFiles, err := json.Marshal(sess.ChangedFiles)
		if err != nil {
			return err
		}
		if _, err := upsert.Exec(sess.Key, sess.FirstSeq, sess.LastSeq,
			sess.SessionID, sess.User, sess.Org, sess.Harness,
			sess.Started, sess.Ended, sess.ExitReason,
			sess.TurnCount, sess.ToolCalls, sess.DangerDetected, b2i(sess.Taint),
			sess.SkillsUsed, sess.DurationMS, sess.FilesTouched, sess.FilesChanged,
			b2i(sess.Shipped), sess.CaptureVersion, sess.PlanStyle,
			sess.Outcome, b2i(sess.CleanDelivery), sess.CorrectionTurns, sess.TaskType, sess.ProjectID, sess.ProjectName,
			string(changedFiles), sess.LinesAdded, sess.LinesRemoved, b2i(sess.ChangeStatsCovered), sess.ObservedToolMS,
			sess.PlanMS, sess.BuildMS, sess.ReviewMS, b2i(sess.WorkMixCovered)); err != nil {
			return err
		}
	}

	if _, err := tx.Exec(`INSERT INTO meta (key, value) VALUES ('last_projected_seq', ?)
		ON CONFLICT(key) DO UPDATE SET value=excluded.value`, strconv.FormatUint(maxSeq, 10)); err != nil {
		return err
	}
	return tx.Commit()
}

// foldSessions groups events into the structural summary columns. Pure, and it
// requires events in seq order (as Log.Events returns them) so the first/last
// timestamps per session are its start/end.
//
// A harness reuses one session id when a session is resumed, so a session_start
// for an id already seen opens a new session rather than extending the previous
// one. Without that, a session resumed across days folds into a single record
// whose span covers all of them and whose latest work dates to the first.
func foldSessions(evs []schema.TelemetryEvent) []Session {
	type acc struct {
		sess    *Session
		skills  map[string]struct{}
		files   map[string]struct{}
		changed map[string]struct{}
		events  []schema.TelemetryEvent
	}
	byKey := make(map[string]*acc)
	currentKey := make(map[string]string)
	runs := make(map[string]int)
	var order []string

	for _, e := range evs {
		key, resumed := currentKey[e.SessionID], false
		if e.EventType == schema.EventSessionStart && key != "" {
			resumed = true
		}
		if key == "" || resumed {
			runs[e.SessionID]++
			key = resumeKey(e.SessionID, runs[e.SessionID])
			currentKey[e.SessionID] = key
		}

		a := byKey[key]
		if a == nil {
			a = &acc{
				sess:    &Session{Key: key, SessionID: e.SessionID, Started: e.TS, FirstSeq: e.Seq},
				skills:  map[string]struct{}{},
				files:   map[string]struct{}{},
				changed: map[string]struct{}{},
			}
			byKey[key] = a
			order = append(order, key)
		}
		a.sess.LastSeq = e.Seq
		a.events = append(a.events, e)
		s := a.sess
		if e.V > s.CaptureVersion {
			s.CaptureVersion = e.V
		}
		if e.ToolClass == schema.ClassFileWrite {
			for _, f := range e.Files {
				if f != "" {
					a.changed[f] = struct{}{}
				}
			}
		}
		if e.DeliverySignal == schema.DeliveryGitPush {
			s.Shipped = true
		}
		if s.User == "" {
			s.User = e.User
		}
		if s.Org == "" {
			s.Org = e.OrgID
		}
		if s.Harness == "" {
			s.Harness = e.Harness
		}
		if s.ProjectID == "" && e.Project != "" {
			s.ProjectID = e.Project
		}
		if e.TS != "" {
			s.Ended = e.TS
		}
		switch e.EventType {
		case schema.EventUserPrompt:
			s.TurnCount++
		case schema.EventPreTool:
			s.ToolCalls++
			if e.Risk == schema.RiskDanger {
				s.DangerDetected++
			}
		case schema.EventStop:
			s.ExitReason = "stopped"
		}
		if e.Taint {
			s.Taint = true
		}
		if e.Skill != "" {
			a.skills[e.Skill] = struct{}{}
		}
		if e.ToolClass == schema.ClassFileRead || e.ToolClass == schema.ClassFileWrite {
			for _, f := range e.Files {
				a.files[f] = struct{}{}
			}
		}
	}

	out := make([]Session, 0, len(order))
	for _, key := range order {
		a := byKey[key]
		if !hasAgentActivity(a.sess) {
			continue
		}
		a.sess.SkillsUsed = joinSorted(a.skills)
		a.sess.DurationMS = durationMS(a.sess.Started, a.sess.Ended)
		a.sess.FilesTouched = len(a.files)
		a.sess.FilesChanged = len(a.changed)
		a.sess.ProjectName = inferredProjectName(a.sess.ProjectID, a.files)
		clean, corrections := analytics.CleanDelivery(a.events)
		taskType, _ := analytics.Taxonomy(a.events)
		a.sess.Outcome = string(analytics.Outcome(a.events))
		a.sess.CleanDelivery = clean
		a.sess.CorrectionTurns = corrections
		a.sess.TaskType = taskType
		a.sess.PlanStyle = string(analytics.ObservedPlanStyle(a.events))
		changes := analytics.ObservedChanges(a.events)
		a.sess.ChangedFiles = changes.Paths()
		a.sess.LinesAdded = changes.LinesAdded
		a.sess.LinesRemoved = changes.LinesRemoved
		a.sess.ChangeStatsCovered = changes.Covered
		mix := analytics.ObservedWorkMix(a.events)
		a.sess.ObservedToolMS = observedToolMS(a.events)
		a.sess.PlanMS = mix.Plan.DurationMS
		a.sess.BuildMS = mix.Build.DurationMS
		a.sess.ReviewMS = mix.Review.DurationMS
		a.sess.WorkMixCovered = mix.Covered
		out = append(out, *a.sess)
	}
	return out
}

// resumeKey leaves a session's first run keyed by its bare id, so the common case
// reads as the id the harness reports and only resumes carry a suffix.
func resumeKey(sessionID string, run int) string {
	if run <= 1 {
		return sessionID
	}
	return sessionID + "#" + strconv.Itoa(run)
}

func observedToolMS(events []schema.TelemetryEvent) int64 {
	var total int64
	for _, event := range events {
		if event.EventType == schema.EventPostTool && event.LatencyMS > 0 {
			total += event.LatencyMS
		}
	}
	return total
}

// inferredProjectName resolves a human label from files already observed in the
// local audit log. An exact id match against the repository proof wins; when no
// observed repository proves the session's id — a renamed remote kills the old
// id forever — the majority of the session's own file evidence names it instead.
// Legacy id-less rows use the same majority rule, with a stable tie-break.
func inferredProjectName(projectID string, files map[string]struct{}) string {
	if name := majorityRepositoryName(projectID, files); name != "" {
		return name
	}
	if projectID == "" {
		return ""
	}
	return majorityRepositoryName("", files)
}

func majorityRepositoryName(projectID string, files map[string]struct{}) string {
	counts := make(map[string]int)
	for file := range files {
		root, id, ok := project.Repository(filepath.Dir(file))
		if !ok || (projectID != "" && id != projectID) {
			continue
		}
		counts[filepath.Base(root)]++
	}
	best, bestCount := "", 0
	for name, count := range counts {
		if count > bestCount || (count == bestCount && (best == "" || name < best)) {
			best, bestCount = name, count
		}
	}
	return best
}

// hasAgentActivity reports whether a folded session recorded real agent work —
// at least one user prompt or one tool call. A session_start/stop-only shell (a
// Claude Code session opened and closed with no interaction, e.g. a /clear, or a
// lone probe marker) has neither. Such a shell belongs in the audit log but is
// not an agent session, so — like an IsInternalSession marker — the read-model
// drops it: counting it would inflate the session count and pollute the
// clean-delivery denominator with a session that never did anything.
func hasAgentActivity(s *Session) bool {
	return s.TurnCount > 0 || s.ToolCalls > 0
}

func joinSorted(set map[string]struct{}) string {
	if len(set) == 0 {
		return ""
	}
	keys := make([]string, 0, len(set))
	for k := range set {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return strings.Join(keys, ",")
}

// durationMS returns elapsed ms between two RFC3339 timestamps, or 0 if either is
// empty/unparseable (timing is best-effort telemetry).
func durationMS(start, end string) int64 {
	if start == "" || end == "" {
		return 0
	}
	st, err1 := time.Parse(time.RFC3339Nano, start)
	et, err2 := time.Parse(time.RFC3339Nano, end)
	if err1 != nil || err2 != nil {
		return 0
	}
	d := et.Sub(st).Milliseconds()
	if d < 0 {
		return 0
	}
	return d
}

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}

// lastProjectedSeq reads the projection watermark; 0 if unset or unreadable.
func (s *Store) lastProjectedSeq() uint64 {
	var v string
	if err := s.db.QueryRow(`SELECT value FROM meta WHERE key='last_projected_seq'`).Scan(&v); err != nil {
		return 0
	}
	n, err := strconv.ParseUint(v, 10, 64)
	if err != nil {
		return 0
	}
	return n
}
