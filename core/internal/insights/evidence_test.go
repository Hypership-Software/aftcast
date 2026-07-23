package insights

import (
	"reflect"
	"testing"
	"time"

	"github.com/Hypership-Software/aftcast/internal/telemetry"
)

func TestEvidenceRowsWindowsGroupsAndSums(t *testing.T) {
	now := time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC)
	since := now.AddDate(0, 0, -14)
	sessions := []telemetry.Session{
		{SessionID: "old", Started: "2026-07-01T10:00:00Z", ProjectName: "aftcast", Shipped: true},
		{SessionID: "s1", Started: "2026-07-20T09:00:00Z", ProjectName: "aftcast", Shipped: true, CorrectionTurns: 2, DangerDetected: 1, FilesChanged: 5},
		{SessionID: "s2", Started: "2026-07-21T09:00:00Z", ProjectName: "aftcast", Taint: true, FilesChanged: 3},
		{SessionID: "s3", Started: "2026-07-22T09:00:00Z", ProjectName: "kuper", Shipped: true},
		{SessionID: "bad", Started: "not-a-time", ProjectName: "kuper"},
	}
	rows := EvidenceRows(sessions, since, now)
	want := []RepoEvidence{
		{Repo: "aftcast", SessionIDs: []string{"s1", "s2"}, Sessions: 2, Shipped: 1, Corrections: 2, Danger: 1, Tainted: 1, FilesChanged: 8},
		{Repo: "kuper", SessionIDs: []string{"s3"}, Sessions: 1, Shipped: 1},
	}
	if !reflect.DeepEqual(rows, want) {
		t.Errorf("rows:\n got %+v\nwant %+v", rows, want)
	}
}

func TestEvidenceRowsOrdersByEarliestSessionNotInputOrder(t *testing.T) {
	now := time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC)
	since := now.AddDate(0, 0, -14)
	sessions := []telemetry.Session{
		{SessionID: "z1", Started: "2026-07-22T09:00:00Z", ProjectName: "zeta"},
		{SessionID: "a1", Started: "2026-07-20T09:00:00Z", ProjectName: "alpha"},
		{SessionID: "z2", Started: "2026-07-22T10:00:00Z", ProjectName: "zeta"},
	}
	rows := EvidenceRows(sessions, since, now)
	want := []RepoEvidence{
		{Repo: "alpha", SessionIDs: []string{"a1"}, Sessions: 1},
		{Repo: "zeta", SessionIDs: []string{"z1", "z2"}, Sessions: 2},
	}
	if !reflect.DeepEqual(rows, want) {
		t.Errorf("rows:\n got %+v\nwant %+v", rows, want)
	}
}
