package state

import (
	"context"
	"time"
)

type Store interface {
	EnsureSchema(ctx context.Context) error
	StartLevelRun(ctx context.Context, run LevelRun) (int64, error)
	IncrementReset(ctx context.Context, runID int64) error
	RecordCheckAttempt(ctx context.Context, runID int64, passed bool) error
	UpsertLevelProgress(ctx context.Context, update LevelProgressUpdate) error
	GetLevelProgressMap(ctx context.Context) (map[string]LevelProgress, error)
	UpsertDailyDrill(ctx context.Context, drill DailyDrill) error
	GetDailyDrill(ctx context.Context, day string) (*DailyDrill, error)
	SaveSettings(ctx context.Context, values map[string]string) error
	LoadSettings(ctx context.Context) (map[string]string, error)
	EnqueueReviewConcepts(ctx context.Context, sourceLevelID string, concepts []string, reviewDays []int, now time.Time) error
	CountDueReviews(ctx context.Context, at time.Time) (int, error)
	GetSummary(ctx context.Context) (Summary, error)
	GetLastRun(ctx context.Context) (*LastRun, error)
	Close() error
}

type LevelRun struct {
	SessionID string
	PackID    string
	LevelID   string
	Mode      string
	StartTS   time.Time
}

type Summary struct {
	LevelRuns int
	Attempts  int
	Passes    int
	Resets    int
}

type LastRun struct {
	PackID     string
	LevelID    string
	Mode       string
	StartTS    time.Time
	LastPassed bool
	Attempts   int
	Resets     int
}

type LevelProgress struct {
	LevelID      string
	PassedCount  int
	BestScore    int
	BestTimeMS   int64
	LastPlayedTS time.Time
	LastPassedTS time.Time
}

type LevelProgressUpdate struct {
	LevelID      string
	Passed       bool
	Score        int
	DurationMS   int64
	LastPlayedTS time.Time
}

type DailyDrill struct {
	Day            string
	PlaylistJSON   string
	CompletedCount int
	UpdatedTS      time.Time
}
