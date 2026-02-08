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
	GetSummary(ctx context.Context) (Summary, error)
	GetLastRun(ctx context.Context) (*LastRun, error)
	Close() error
}

type LevelRun struct {
	SessionID string
	PackID    string
	LevelID   string
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
	StartTS    time.Time
	LastPassed bool
	Attempts   int
	Resets     int
}
