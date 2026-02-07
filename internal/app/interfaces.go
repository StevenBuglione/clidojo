package app

import "context"

type Sandbox interface {
	Detect(ctx context.Context, forceEngine string) (EngineInfo, error)
	StartLevel(ctx context.Context, spec StartSpec) (Handle, error)
	CleanupOrphans(ctx context.Context, activeSession string) error
}

type Handle interface {
	ShellCommand() []string
	Stop(ctx context.Context) error
	WorkDir() string
	ContainerName() string
	Cwd() string
	Env() []string
	IsMock() bool
}

type Store interface {
	EnsureSchema(ctx context.Context) error
	StartLevelRun(ctx context.Context, run LevelRun) (int64, error)
	IncrementReset(ctx context.Context, runID int64) error
	RecordCheckAttempt(ctx context.Context, runID int64, passed bool) error
	Close() error
}
