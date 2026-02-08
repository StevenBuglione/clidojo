package state

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

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
		`CREATE TABLE IF NOT EXISTS review_queue (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			concept TEXT NOT NULL,
			source_level_id TEXT NOT NULL,
			due_date TEXT NOT NULL,
			completed INTEGER NOT NULL DEFAULT 0,
			created_ts TEXT NOT NULL DEFAULT (datetime('now')),
			UNIQUE(concept, source_level_id, due_date)
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

func (s *SQLiteStore) EnqueueReviewConcepts(ctx context.Context, sourceLevelID string, concepts []string, reviewDays []int, now time.Time) error {
	if strings.TrimSpace(sourceLevelID) == "" || len(concepts) == 0 {
		return nil
	}
	if len(reviewDays) == 0 {
		reviewDays = []int{1, 3, 7}
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()
	for _, rawConcept := range concepts {
		concept := strings.TrimSpace(rawConcept)
		if concept == "" {
			continue
		}
		for _, day := range reviewDays {
			if day <= 0 {
				continue
			}
			due := now.UTC().AddDate(0, 0, day).Format("2006-01-02")
			if _, err = tx.ExecContext(
				ctx,
				`INSERT OR IGNORE INTO review_queue(concept, source_level_id, due_date, created_ts) VALUES(?,?,?,?)`,
				concept,
				sourceLevelID,
				due,
				now.UTC().Format(timeLayout),
			); err != nil {
				return err
			}
		}
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	return nil
}

func (s *SQLiteStore) CountDueReviews(ctx context.Context, at time.Time) (int, error) {
	var due int
	row := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM review_queue
		WHERE completed = 0 AND due_date <= ?
	`, at.UTC().Format("2006-01-02"))
	if err := row.Scan(&due); err != nil {
		return 0, err
	}
	return due, nil
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

func (s *SQLiteStore) GetLastRun(ctx context.Context) (*LastRun, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT pack_id, level_id, start_ts, last_passed, attempts, resets
		FROM level_runs
		ORDER BY id DESC
		LIMIT 1
	`)
	var (
		packID     string
		levelID    string
		startTSRaw string
		lastPassed int
		attempts   int
		resets     int
	)
	if err := row.Scan(&packID, &levelID, &startTSRaw, &lastPassed, &attempts, &resets); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	startTS, err := time.Parse(timeLayout, startTSRaw)
	if err != nil {
		startTS = time.Time{}
	}
	return &LastRun{
		PackID:     packID,
		LevelID:    levelID,
		StartTS:    startTS,
		LastPassed: lastPassed == 1,
		Attempts:   attempts,
		Resets:     resets,
	}, nil
}

func (s *SQLiteStore) Close() error {
	if s.db == nil {
		return nil
	}
	return s.db.Close()
}

const timeLayout = "2006-01-02T15:04:05Z07:00"
