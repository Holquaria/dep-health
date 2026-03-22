// Package store provides SQLite-backed persistence for dep-health scan results.
package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"dep-health/models"
)

// ScanRun represents a single pipeline execution stored in the database.
type ScanRun struct {
	ID         int64      `json:"id"`
	Dir        string     `json:"dir"`
	RepoURL    string     `json:"repo_url"`
	IsMulti    bool       `json:"is_multi"`
	Targets    []string   `json:"targets,omitempty"`
	Status     string     `json:"status"` // pending | running | done | failed
	StartedAt  time.Time  `json:"started_at"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
	Error      string     `json:"error,omitempty"`
}

// Store wraps a SQLite database connection.
type Store struct {
	db *sql.DB
}

// New opens (or creates) a SQLite database at the given path and applies the schema.
func New(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	if _, err := db.Exec(`PRAGMA journal_mode=WAL; PRAGMA foreign_keys=ON;`); err != nil {
		db.Close()
		return nil, fmt.Errorf("pragma: %w", err)
	}
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) migrate() error {
	// Base schema — idempotent via IF NOT EXISTS.
	_, err := s.db.Exec(`
CREATE TABLE IF NOT EXISTS scan_runs (
	id          INTEGER PRIMARY KEY AUTOINCREMENT,
	dir         TEXT    NOT NULL,
	repo_url    TEXT    NOT NULL DEFAULT '',
	status      TEXT    NOT NULL DEFAULT 'pending',
	started_at  TEXT    NOT NULL,
	finished_at TEXT,
	error       TEXT    NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS scan_deps (
	id               INTEGER PRIMARY KEY AUTOINCREMENT,
	run_id           INTEGER NOT NULL REFERENCES scan_runs(id) ON DELETE CASCADE,
	name             TEXT    NOT NULL,
	ecosystem        TEXT    NOT NULL,
	manifest_path    TEXT    NOT NULL DEFAULT '',
	current_ver      TEXT    NOT NULL DEFAULT '',
	latest_ver       TEXT    NOT NULL DEFAULT '',
	severity_gap     TEXT    NOT NULL DEFAULT '',
	versions_behind  INTEGER NOT NULL DEFAULT 0,
	risk_score       REAL    NOT NULL DEFAULT 0,
	cascade_group    TEXT    NOT NULL DEFAULT '',
	blocked_by       TEXT    NOT NULL DEFAULT '[]',
	peer_constraints TEXT    NOT NULL DEFAULT '{}',
	vulnerabilities  TEXT    NOT NULL DEFAULT '[]',
	reasons          TEXT    NOT NULL DEFAULT '[]',
	summary          TEXT    NOT NULL DEFAULT '',
	breaking_changes TEXT    NOT NULL DEFAULT '[]',
	migration_steps  TEXT    NOT NULL DEFAULT '[]'
);
`)
	if err != nil {
		return fmt.Errorf("migrate: %w", err)
	}

	// Additive column migrations — safe to run against existing databases.
	// SQLite reports "duplicate column name" when the column already exists;
	// that is treated as a no-op.
	additive := []string{
		`ALTER TABLE scan_runs ADD COLUMN is_multi INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE scan_runs ADD COLUMN targets  TEXT    NOT NULL DEFAULT '[]'`,
		`ALTER TABLE scan_deps ADD COLUMN repo_source      TEXT    NOT NULL DEFAULT ''`,
		`ALTER TABLE scan_deps ADD COLUMN cross_repo_count INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE scan_deps ADD COLUMN latest_in_major  TEXT    NOT NULL DEFAULT ''`,
		`ALTER TABLE scan_deps ADD COLUMN license          TEXT    NOT NULL DEFAULT ''`,
		`ALTER TABLE scan_deps ADD COLUMN license_risk     TEXT    NOT NULL DEFAULT ''`,
	}
	for _, stmt := range additive {
		if _, err := s.db.Exec(stmt); err != nil {
			if !strings.Contains(err.Error(), "duplicate column name") {
				return fmt.Errorf("migrate column: %w", err)
			}
		}
	}
	return nil
}

// CreateScanRun inserts a new single-repo scan_run row with status "running".
func (s *Store) CreateScanRun(dir, repoURL string) (int64, error) {
	res, err := s.db.Exec(
		`INSERT INTO scan_runs (dir, repo_url, status, started_at) VALUES (?, ?, 'running', ?)`,
		dir, repoURL, time.Now().UTC().Format(time.RFC3339),
	)
	if err != nil {
		return 0, fmt.Errorf("create scan run: %w", err)
	}
	return res.LastInsertId()
}

// CreateMultiScanRun inserts a multi-repo scan_run row with status "running".
func (s *Store) CreateMultiScanRun(targets []string) (int64, error) {
	targetsJSON, _ := json.Marshal(targets)
	dir := fmt.Sprintf("%d repos", len(targets))
	res, err := s.db.Exec(
		`INSERT INTO scan_runs (dir, repo_url, is_multi, targets, status, started_at)
		 VALUES (?, '', 1, ?, 'running', ?)`,
		dir, string(targetsJSON), time.Now().UTC().Format(time.RFC3339),
	)
	if err != nil {
		return 0, fmt.Errorf("create multi scan run: %w", err)
	}
	return res.LastInsertId()
}

// FinishScanRun updates a scan run to status "done" or "failed".
func (s *Store) FinishScanRun(id int64, scanErr error) error {
	status := "done"
	errMsg := ""
	if scanErr != nil {
		status = "failed"
		errMsg = scanErr.Error()
	}
	_, err := s.db.Exec(
		`UPDATE scan_runs SET status=?, finished_at=?, error=? WHERE id=?`,
		status, time.Now().UTC().Format(time.RFC3339), errMsg, id,
	)
	return err
}

// SaveDeps bulk-inserts advisory reports for a completed scan run.
func (s *Store) SaveDeps(runID int64, reports []models.AdvisoryReport) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck

	stmt, err := tx.Prepare(`
INSERT INTO scan_deps (
	run_id, name, ecosystem, manifest_path, current_ver, latest_ver,
	severity_gap, versions_behind, risk_score, cascade_group, blocked_by,
	peer_constraints, vulnerabilities, reasons, summary, breaking_changes,
	migration_steps, repo_source, cross_repo_count, latest_in_major,
	license, license_risk
) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, r := range reports {
		blockedBy, _ := json.Marshal(r.BlockedBy)
		peers, _ := json.Marshal(r.PeerConstraints)
		vulns, _ := json.Marshal(r.Vulnerabilities)
		reasons, _ := json.Marshal(r.Reasons)
		breaking, _ := json.Marshal(r.BreakingChanges)
		steps, _ := json.Marshal(r.MigrationSteps)

		if _, err := stmt.Exec(
			runID,
			r.Name, r.Ecosystem, r.ManifestPath,
			r.CurrentVersion, r.LatestVersion,
			r.SeverityGap, r.VersionsBehind, r.RiskScore,
			r.CascadeGroup,
			string(blockedBy), string(peers), string(vulns),
			string(reasons), r.Summary,
			string(breaking), string(steps),
			r.RepoSource, r.CrossRepoCount, r.LatestInMajor,
			r.License, r.LicenseRisk,
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ListScans returns all scan runs in descending order of start time.
func (s *Store) ListScans() ([]ScanRun, error) {
	rows, err := s.db.Query(
		`SELECT id, dir, repo_url, is_multi, targets, status, started_at, finished_at, error
		 FROM scan_runs ORDER BY id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanRunsFromRows(rows)
}

// GetScan returns the scan run and its dependency reports for the given ID.
func (s *Store) GetScan(id int64) (ScanRun, []models.AdvisoryReport, error) {
	var run ScanRun
	var startedStr string
	var finishedStr sql.NullString
	var targetsJSON string
	err := s.db.QueryRow(
		`SELECT id, dir, repo_url, is_multi, targets, status, started_at, finished_at, error
		 FROM scan_runs WHERE id=?`, id,
	).Scan(&run.ID, &run.Dir, &run.RepoURL, &run.IsMulti, &targetsJSON,
		&run.Status, &startedStr, &finishedStr, &run.Error)
	if err == sql.ErrNoRows {
		return ScanRun{}, nil, fmt.Errorf("scan %d not found", id)
	}
	if err != nil {
		return ScanRun{}, nil, err
	}
	run.StartedAt, _ = time.Parse(time.RFC3339, startedStr)
	if finishedStr.Valid {
		t, _ := time.Parse(time.RFC3339, finishedStr.String)
		run.FinishedAt = &t
	}
	json.Unmarshal([]byte(targetsJSON), &run.Targets) //nolint:errcheck

	rows, err := s.db.Query(
		`SELECT name, ecosystem, manifest_path, current_ver, latest_ver,
		        severity_gap, versions_behind, risk_score, cascade_group,
		        blocked_by, peer_constraints, vulnerabilities, reasons,
		        summary, breaking_changes, migration_steps,
		        repo_source, cross_repo_count, latest_in_major,
		        license, license_risk
		 FROM scan_deps WHERE run_id=? ORDER BY risk_score DESC`, id)
	if err != nil {
		return run, nil, err
	}
	defer rows.Close()

	var reports []models.AdvisoryReport
	for rows.Next() {
		var r models.AdvisoryReport
		var blockedBy, peers, vulns, reasons, breaking, steps string
		if err := rows.Scan(
			&r.Name, &r.Ecosystem, &r.ManifestPath,
			&r.CurrentVersion, &r.LatestVersion,
			&r.SeverityGap, &r.VersionsBehind, &r.RiskScore,
			&r.CascadeGroup,
			&blockedBy, &peers, &vulns,
			&reasons, &r.Summary,
			&breaking, &steps,
			&r.RepoSource, &r.CrossRepoCount, &r.LatestInMajor,
			&r.License, &r.LicenseRisk,
		); err != nil {
			return run, nil, err
		}
		json.Unmarshal([]byte(blockedBy), &r.BlockedBy)      //nolint:errcheck
		json.Unmarshal([]byte(peers), &r.PeerConstraints)    //nolint:errcheck
		json.Unmarshal([]byte(vulns), &r.Vulnerabilities)    //nolint:errcheck
		json.Unmarshal([]byte(reasons), &r.Reasons)          //nolint:errcheck
		json.Unmarshal([]byte(breaking), &r.BreakingChanges) //nolint:errcheck
		json.Unmarshal([]byte(steps), &r.MigrationSteps)     //nolint:errcheck
		reports = append(reports, r)
	}
	return run, reports, rows.Err()
}

// RecoverStuckScans marks any runs still "running" as "failed" (e.g. after a crash).
func (s *Store) RecoverStuckScans() error {
	_, err := s.db.Exec(
		`UPDATE scan_runs SET status='failed', error='interrupted' WHERE status IN ('pending','running')`,
	)
	return err
}

// Close closes the underlying database connection.
func (s *Store) Close() error { return s.db.Close() }

func scanRunsFromRows(rows *sql.Rows) ([]ScanRun, error) {
	var runs []ScanRun
	for rows.Next() {
		var r ScanRun
		var startedStr string
		var finishedStr sql.NullString
		var targetsJSON string
		if err := rows.Scan(&r.ID, &r.Dir, &r.RepoURL, &r.IsMulti, &targetsJSON,
			&r.Status, &startedStr, &finishedStr, &r.Error); err != nil {
			return nil, err
		}
		r.StartedAt, _ = time.Parse(time.RFC3339, startedStr)
		if finishedStr.Valid {
			t, _ := time.Parse(time.RFC3339, finishedStr.String)
			r.FinishedAt = &t
		}
		json.Unmarshal([]byte(targetsJSON), &r.Targets) //nolint:errcheck
		runs = append(runs, r)
	}
	return runs, rows.Err()
}
