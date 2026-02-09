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
			mode TEXT NOT NULL DEFAULT 'free',
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
		`CREATE TABLE IF NOT EXISTS level_progress (
			level_id TEXT PRIMARY KEY,
			passed_count INTEGER NOT NULL DEFAULT 0,
			best_score INTEGER NOT NULL DEFAULT 0,
			best_time_ms INTEGER NOT NULL DEFAULT 0,
			last_played_ts TEXT NOT NULL DEFAULT '',
			last_passed_ts TEXT NOT NULL DEFAULT ''
		);`,
		`CREATE TABLE IF NOT EXISTS daily_drill (
			day TEXT PRIMARY KEY,
			playlist_json TEXT NOT NULL,
			completed_count INTEGER NOT NULL DEFAULT 0,
			updated_ts TEXT NOT NULL DEFAULT (datetime('now'))
		);`,
		`CREATE TABLE IF NOT EXISTS app_settings (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL
		);`,
	}
	for _, stmt := range stmts {
		if _, err := s.db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("ensure schema: %w", err)
		}
	}
	// Backfill older schemas that predate level_runs.mode.
	if _, err := s.db.ExecContext(ctx, `ALTER TABLE level_runs ADD COLUMN mode TEXT NOT NULL DEFAULT 'free'`); err != nil {
		msg := strings.ToLower(err.Error())
		if !strings.Contains(msg, "duplicate column name") {
			return fmt.Errorf("ensure schema alter level_runs.mode: %w", err)
		}
	}
	return nil
}

func (s *SQLiteStore) StartLevelRun(ctx context.Context, run LevelRun) (int64, error) {
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO level_runs(session_id, pack_id, level_id, mode, start_ts) VALUES(?,?,?,?,?)`,
		run.SessionID,
		run.PackID,
		run.LevelID,
		strings.TrimSpace(run.Mode),
		run.StartTS.UTC().Format(timeLayout),
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

func (s *SQLiteStore) UpsertLevelProgress(ctx context.Context, update LevelProgressUpdate) error {
	levelID := strings.TrimSpace(update.LevelID)
	if levelID == "" {
		return nil
	}
	playTS := update.LastPlayedTS
	if playTS.IsZero() {
		playTS = time.Now().UTC()
	}
	passTS := ""
	if update.Passed {
		passTS = playTS.UTC().Format(timeLayout)
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO level_progress(level_id, passed_count, best_score, best_time_ms, last_played_ts, last_passed_ts)
		VALUES(?, ?, ?, ?, ?, ?)
		ON CONFLICT(level_id) DO UPDATE SET
			passed_count = level_progress.passed_count + excluded.passed_count,
			best_score = CASE
				WHEN excluded.best_score > 0 AND excluded.best_score > level_progress.best_score THEN excluded.best_score
				ELSE level_progress.best_score
			END,
			best_time_ms = CASE
				WHEN excluded.best_time_ms > 0 AND (level_progress.best_time_ms = 0 OR excluded.best_time_ms < level_progress.best_time_ms) THEN excluded.best_time_ms
				ELSE level_progress.best_time_ms
			END,
			last_played_ts = excluded.last_played_ts,
			last_passed_ts = CASE
				WHEN excluded.last_passed_ts <> '' THEN excluded.last_passed_ts
				ELSE level_progress.last_passed_ts
			END
	`,
		levelID,
		ifThen(update.Passed, 1, 0),
		max(0, update.Score),
		max64(0, update.DurationMS),
		playTS.UTC().Format(timeLayout),
		passTS,
	)
	return err
}

func (s *SQLiteStore) GetLevelProgressMap(ctx context.Context) (map[string]LevelProgress, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT level_id, passed_count, best_score, best_time_ms, last_played_ts, last_passed_ts
		FROM level_progress
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]LevelProgress{}
	for rows.Next() {
		var (
			levelID      string
			passedCount  int
			bestScore    int
			bestTimeMS   int64
			lastPlayed   string
			lastPassed   string
			lastPlayedTS time.Time
			lastPassedTS time.Time
		)
		if err := rows.Scan(&levelID, &passedCount, &bestScore, &bestTimeMS, &lastPlayed, &lastPassed); err != nil {
			return nil, err
		}
		if t, err := time.Parse(timeLayout, lastPlayed); err == nil {
			lastPlayedTS = t
		}
		if t, err := time.Parse(timeLayout, lastPassed); err == nil {
			lastPassedTS = t
		}
		out[levelID] = LevelProgress{
			LevelID:      levelID,
			PassedCount:  passedCount,
			BestScore:    bestScore,
			BestTimeMS:   bestTimeMS,
			LastPlayedTS: lastPlayedTS,
			LastPassedTS: lastPassedTS,
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *SQLiteStore) UpsertDailyDrill(ctx context.Context, drill DailyDrill) error {
	day := strings.TrimSpace(drill.Day)
	if day == "" {
		return nil
	}
	updated := drill.UpdatedTS
	if updated.IsZero() {
		updated = time.Now().UTC()
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO daily_drill(day, playlist_json, completed_count, updated_ts)
		VALUES(?, ?, ?, ?)
		ON CONFLICT(day) DO UPDATE SET
			playlist_json = excluded.playlist_json,
			completed_count = CASE
				WHEN excluded.completed_count > daily_drill.completed_count THEN excluded.completed_count
				ELSE daily_drill.completed_count
			END,
			updated_ts = excluded.updated_ts
	`, day, drill.PlaylistJSON, max(0, drill.CompletedCount), updated.UTC().Format(timeLayout))
	return err
}

func (s *SQLiteStore) GetDailyDrill(ctx context.Context, day string) (*DailyDrill, error) {
	day = strings.TrimSpace(day)
	if day == "" {
		return nil, nil
	}
	row := s.db.QueryRowContext(ctx, `
		SELECT day, playlist_json, completed_count, updated_ts
		FROM daily_drill
		WHERE day = ?
	`, day)
	var (
		out          DailyDrill
		updatedTSRaw string
	)
	if err := row.Scan(&out.Day, &out.PlaylistJSON, &out.CompletedCount, &updatedTSRaw); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	if t, err := time.Parse(timeLayout, updatedTSRaw); err == nil {
		out.UpdatedTS = t
	}
	return &out, nil
}

func (s *SQLiteStore) SaveSettings(ctx context.Context, values map[string]string) error {
	if len(values) == 0 {
		return nil
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
	for key, value := range values {
		k := strings.TrimSpace(key)
		if k == "" {
			continue
		}
		if _, err = tx.ExecContext(ctx, `
			INSERT INTO app_settings(key, value) VALUES(?, ?)
			ON CONFLICT(key) DO UPDATE SET value = excluded.value
		`, k, value); err != nil {
			return err
		}
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	return nil
}

func (s *SQLiteStore) LoadSettings(ctx context.Context) (map[string]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT key, value FROM app_settings`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		out[k] = v
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
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
		SELECT pack_id, level_id, mode, start_ts, last_passed, attempts, resets
		FROM level_runs
		ORDER BY id DESC
		LIMIT 1
	`)
	var (
		packID     string
		levelID    string
		mode       string
		startTSRaw string
		lastPassed int
		attempts   int
		resets     int
	)
	if err := row.Scan(&packID, &levelID, &mode, &startTSRaw, &lastPassed, &attempts, &resets); err != nil {
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
		Mode:       mode,
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

func ifThen(cond bool, yes, no int) int {
	if cond {
		return yes
	}
	return no
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func max64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}
