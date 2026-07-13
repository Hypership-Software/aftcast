package telemetry

import (
	"encoding/json"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Hypership-Software/atlas/internal/analytics"
	"github.com/Hypership-Software/atlas/internal/audit"
	"github.com/Hypership-Software/atlas/internal/schema"
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
		raw, err := json.Marshal(e)
		if err != nil {
			return err
		}
		if _, err := insEvent.Exec(e.Seq, e.SessionID, e.TurnIndex, string(e.EventType), e.TS, string(raw)); err != nil {
			return err
		}
	}

	upsert, err := tx.Prepare(`INSERT INTO sessions
		(session_id, user, org, harness, started, ended, exit_reason,
		 turn_count, tool_calls, danger_detected, taint, skills_used, duration_ms, files_touched,
		 outcome, clean_delivery, correction_turns, task_type)
		VALUES (?,?,?,?,?,?,?, ?,?,?,?,?,?,?, ?,?,?,?)
		ON CONFLICT(session_id) DO UPDATE SET
			user=excluded.user, org=excluded.org, harness=excluded.harness,
			started=excluded.started, ended=excluded.ended, exit_reason=excluded.exit_reason,
			turn_count=excluded.turn_count, tool_calls=excluded.tool_calls,
			danger_detected=excluded.danger_detected, taint=excluded.taint,
			skills_used=excluded.skills_used, duration_ms=excluded.duration_ms, files_touched=excluded.files_touched,
			outcome=excluded.outcome, clean_delivery=excluded.clean_delivery,
			correction_turns=excluded.correction_turns, task_type=excluded.task_type`)
	if err != nil {
		return err
	}
	defer upsert.Close()
	for _, sess := range foldSessions(visible) {
		if _, err := upsert.Exec(sess.SessionID, sess.User, sess.Org, sess.Harness,
			sess.Started, sess.Ended, sess.ExitReason,
			sess.TurnCount, sess.ToolCalls, sess.DangerDetected, b2i(sess.Taint), sess.SkillsUsed, sess.DurationMS, sess.FilesTouched,
			sess.Outcome, b2i(sess.CleanDelivery), sess.CorrectionTurns, sess.TaskType); err != nil {
			return err
		}
	}

	if _, err := tx.Exec(`INSERT INTO meta (key, value) VALUES ('last_projected_seq', ?)
		ON CONFLICT(key) DO UPDATE SET value=excluded.value`, strconv.FormatUint(maxSeq, 10)); err != nil {
		return err
	}
	return tx.Commit()
}

// foldSessions groups events by session_id into the structural summary columns.
// Pure, and it requires events in seq order (as Log.Events returns them) so the
// first/last timestamps per session are its start/end.
func foldSessions(evs []schema.TelemetryEvent) []Session {
	type acc struct {
		sess   *Session
		skills map[string]struct{}
		files  map[string]struct{}
		events []schema.TelemetryEvent
	}
	byID := make(map[string]*acc)
	var order []string

	for _, e := range evs {
		a := byID[e.SessionID]
		if a == nil {
			a = &acc{sess: &Session{SessionID: e.SessionID, Started: e.TS}, skills: map[string]struct{}{}, files: map[string]struct{}{}}
			byID[e.SessionID] = a
			order = append(order, e.SessionID)
		}
		a.events = append(a.events, e)
		s := a.sess
		if s.User == "" {
			s.User = e.User
		}
		if s.Org == "" {
			s.Org = e.OrgID
		}
		if s.Harness == "" {
			s.Harness = e.Harness
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
	for _, id := range order {
		a := byID[id]
		a.sess.SkillsUsed = joinSorted(a.skills)
		a.sess.DurationMS = durationMS(a.sess.Started, a.sess.Ended)
		a.sess.FilesTouched = len(a.files)
		clean, corrections := analytics.CleanDelivery(a.events)
		taskType, _ := analytics.Taxonomy(a.events)
		a.sess.Outcome = string(analytics.Outcome(a.events))
		a.sess.CleanDelivery = clean
		a.sess.CorrectionTurns = corrections
		a.sess.TaskType = taskType
		out = append(out, *a.sess)
	}
	return out
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
