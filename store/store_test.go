package store_test

import (
	"fmt"
	"testing"

	"dep-health/models"
	"dep-health/store"
)

const testSession = "test-session-abc"

func newTestStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestCreateAndFinishScanRun(t *testing.T) {
	s := newTestStore(t)

	id, err := s.CreateScanRun("/tmp/proj", "https://github.com/org/repo", testSession)
	if err != nil {
		t.Fatalf("CreateScanRun: %v", err)
	}
	if id <= 0 {
		t.Errorf("expected positive id, got %d", id)
	}

	if err := s.FinishScanRun(id, nil); err != nil {
		t.Fatalf("FinishScanRun: %v", err)
	}

	run, deps, err := s.GetScan(id, testSession)
	if err != nil {
		t.Fatalf("GetScan: %v", err)
	}
	if run.Status != "done" {
		t.Errorf("expected status=done, got %q", run.Status)
	}
	if run.FinishedAt == nil {
		t.Error("expected FinishedAt to be set")
	}
	if len(deps) != 0 {
		t.Errorf("expected 0 deps, got %d", len(deps))
	}
}

func TestFinishScanRunWithError(t *testing.T) {
	s := newTestStore(t)

	id, _ := s.CreateScanRun("/tmp/proj", "", testSession)
	scanErr := fmt.Errorf("clone failed") //nolint:goerr113
	if err := s.FinishScanRun(id, scanErr); err != nil {
		t.Fatalf("FinishScanRun: %v", err)
	}

	run, _, err := s.GetScan(id, testSession)
	if err != nil {
		t.Fatalf("GetScan: %v", err)
	}
	if run.Status != "failed" {
		t.Errorf("expected status=failed, got %q", run.Status)
	}
	if run.Error == "" {
		t.Error("expected non-empty error string")
	}
}

func TestSaveDepsAndGetScan(t *testing.T) {
	s := newTestStore(t)

	id, _ := s.CreateScanRun("/tmp/proj", "", testSession)

	reports := []models.AdvisoryReport{
		{
			ScoredDependency: models.ScoredDependency{
				EnrichedDependency: models.EnrichedDependency{
					Dependency: models.Dependency{
						Name:      "lodash",
						Ecosystem: "npm",
					},
					LatestVersion:  "4.17.21",
					SeverityGap:    "major",
					VersionsBehind: 5,
				},
				RiskScore: 72.5,
				Reasons:   []string{"major version gap", "3 CVEs"},
			},
			Summary:        "Upgrade lodash to fix prototype pollution vulnerabilities.",
			MigrationSteps: []string{"run npm install lodash@4"},
		},
	}

	if err := s.SaveDeps(id, reports); err != nil {
		t.Fatalf("SaveDeps: %v", err)
	}
	s.FinishScanRun(id, nil)

	_, got, err := s.GetScan(id, testSession)
	if err != nil {
		t.Fatalf("GetScan: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 dep, got %d", len(got))
	}
	d := got[0]
	if d.Name != "lodash" {
		t.Errorf("name: got %q, want %q", d.Name, "lodash")
	}
	if d.RiskScore != 72.5 {
		t.Errorf("risk_score: got %v, want 72.5", d.RiskScore)
	}
	if len(d.MigrationSteps) != 1 {
		t.Errorf("migration_steps: got %d, want 1", len(d.MigrationSteps))
	}
}

func TestListScans(t *testing.T) {
	s := newTestStore(t)

	id1, _ := s.CreateScanRun("/proj/a", "", testSession)
	id2, _ := s.CreateScanRun("/proj/b", "", testSession)
	s.FinishScanRun(id1, nil)
	s.FinishScanRun(id2, nil)

	runs, err := s.ListScans(testSession)
	if err != nil {
		t.Fatalf("ListScans: %v", err)
	}
	if len(runs) != 2 {
		t.Fatalf("expected 2 runs, got %d", len(runs))
	}
	// ListScans returns newest first.
	if runs[0].ID != id2 {
		t.Errorf("expected first run to be id=%d, got id=%d", id2, runs[0].ID)
	}
}

func TestCreateMultiScanRun(t *testing.T) {
	s := newTestStore(t)

	targets := []string{"https://github.com/org/a", "https://github.com/org/b"}
	id, err := s.CreateMultiScanRun(targets, testSession)
	if err != nil {
		t.Fatalf("CreateMultiScanRun: %v", err)
	}

	run, _, err := s.GetScan(id, testSession)
	if err != nil {
		t.Fatalf("GetScan: %v", err)
	}
	if !run.IsMulti {
		t.Error("expected IsMulti=true")
	}
	if len(run.Targets) != 2 {
		t.Errorf("expected 2 targets, got %d", len(run.Targets))
	}
}

func TestRecoverStuckScans(t *testing.T) {
	s := newTestStore(t)

	id, _ := s.CreateScanRun("/tmp/proj", "", testSession) // status = "running"

	if err := s.RecoverStuckScans(); err != nil {
		t.Fatalf("RecoverStuckScans: %v", err)
	}

	run, _, err := s.GetScan(id, testSession)
	if err != nil {
		t.Fatalf("GetScan: %v", err)
	}
	if run.Status != "failed" {
		t.Errorf("expected status=failed after recovery, got %q", run.Status)
	}
}

func TestGetScanNotFound(t *testing.T) {
	s := newTestStore(t)

	_, _, err := s.GetScan(9999, testSession)
	if err == nil {
		t.Error("expected error for missing scan, got nil")
	}
}

// ── Session isolation ─────────────────────────────────────────────────────────

func TestSessionIsolation_ListScans(t *testing.T) {
	s := newTestStore(t)

	// Session A creates a scan.
	idA, _ := s.CreateScanRun("/proj/a", "", "session-a")
	s.FinishScanRun(idA, nil)

	// Session B creates a scan.
	idB, _ := s.CreateScanRun("/proj/b", "", "session-b")
	s.FinishScanRun(idB, nil)

	// Session A should only see its own scan.
	runsA, err := s.ListScans("session-a")
	if err != nil {
		t.Fatalf("ListScans(a): %v", err)
	}
	if len(runsA) != 1 {
		t.Fatalf("session-a: expected 1 run, got %d", len(runsA))
	}
	if runsA[0].ID != idA {
		t.Errorf("session-a: expected run id=%d, got id=%d", idA, runsA[0].ID)
	}

	// Session B should only see its own scan.
	runsB, err := s.ListScans("session-b")
	if err != nil {
		t.Fatalf("ListScans(b): %v", err)
	}
	if len(runsB) != 1 {
		t.Fatalf("session-b: expected 1 run, got %d", len(runsB))
	}
	if runsB[0].ID != idB {
		t.Errorf("session-b: expected run id=%d, got id=%d", idB, runsB[0].ID)
	}
}

func TestSessionIsolation_GetScan(t *testing.T) {
	s := newTestStore(t)

	// Session A creates a scan.
	idA, _ := s.CreateScanRun("/proj/a", "", "session-a")
	s.FinishScanRun(idA, nil)

	// Session B should NOT be able to view session A's scan.
	_, _, err := s.GetScan(idA, "session-b")
	if err == nil {
		t.Error("expected error when session-b tries to get session-a's scan, got nil")
	}

	// Session A should be able to view its own scan.
	run, _, err := s.GetScan(idA, "session-a")
	if err != nil {
		t.Fatalf("GetScan(a): %v", err)
	}
	if run.SessionID != "session-a" {
		t.Errorf("expected session_id=session-a, got %q", run.SessionID)
	}
}
