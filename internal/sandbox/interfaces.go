package sandbox

import "context"

type Runner interface {
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
