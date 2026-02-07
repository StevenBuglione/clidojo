package state

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

type SQLiteStore struct {
	db *sql.DB
}

func NewSQLite(path string) (*SQLiteStore, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	return &SQLiteStore{db: db}, nil
}

func (s *SQLiteStore) EnsureSchema(ctx context.Context) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS level_runs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			session_id TEXT NOT NULL,
			pack_id TEXT NOT NULL,
			level_id TEXT NOT NULL,
			start_ts TEXT NOT NULL,
			resets INTEGER NOT NULL DEFAULT 0,
			attempts INTEGER NOT NULL DEFAULT 0,
			last_passed INTEGER NOT NULL DEFAULT 0
		);`,
		`CREATE TABLE IF NOT EXISTS check_attempts (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			run_id INTEGER NOT NULL,
			attempt_ts TEXT NOT NULL DEFAULT (datetime('now')),
			passed INTEGER NOT NULL,
			FOREIGN KEY(run_id) REFERENCES level_runs(id)
		);`,
	}
	for _, stmt := range stmts {
		if _, err := s.db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("ensure schema: %w", err)
		}
	}
	return nil
}

func (s *SQLiteStore) StartLevelRun(ctx context.Context, run LevelRun) (int64, error) {
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO level_runs(session_id, pack_id, level_id, start_ts) VALUES(?,?,?,?)`,
		run.SessionID, run.PackID, run.LevelID, run.StartTS.UTC().Format(timeLayout),
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *SQLiteStore) IncrementReset(ctx context.Context, runID int64) error {
	_, err := s.db.ExecContext(ctx, `UPDATE level_runs SET resets = resets + 1 WHERE id = ?`, runID)
	return err
}

func (s *SQLiteStore) RecordCheckAttempt(ctx context.Context, runID int64, passed bool) error {
	passedInt := 0
	if passed {
		passedInt = 1
	}
	if _, err := s.db.ExecContext(ctx, `INSERT INTO check_attempts(run_id, passed) VALUES(?, ?)`, runID, passedInt); err != nil {
		return err
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE level_runs SET attempts = attempts + 1, last_passed = ? WHERE id = ?`, passedInt, runID); err != nil {
		return err
	}
	return nil
}

func (s *SQLiteStore) GetSummary(ctx context.Context) (Summary, error) {
	var out Summary
	row := s.db.QueryRowContext(ctx, `
		SELECT
			COUNT(*) as level_runs,
			COALESCE(SUM(attempts),0) as attempts,
			COALESCE(SUM(last_passed),0) as passes,
			COALESCE(SUM(resets),0) as resets
		FROM level_runs
	`)
	if err := row.Scan(&out.LevelRuns, &out.Attempts, &out.Passes, &out.Resets); err != nil {
		return Summary{}, err
	}
	return out, nil
}

func (s *SQLiteStore) Close() error {
	if s.db == nil {
		return nil
	}
	return s.db.Close()
}

const timeLayout = "2006-01-02T15:04:05Z07:00"
