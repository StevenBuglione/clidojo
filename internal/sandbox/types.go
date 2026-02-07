package sandbox

type EngineInfo struct {
	Name    string
	Version string
}

type StartSpec struct {
	SessionID     string
	PackID        string
	LevelID       string
	ContainerName string
	Image         string

	DatasetDir   string
	DatasetMount string
	WorkDir      string
	WorkMount    string

	UseSELinuxZ bool

	ShellProgram string
	ShellArgs    []string
	ShellCWD     string
	ShellEnv     map[string]string

	Network      string
	ReadOnlyRoot bool
	CPU          float64
	MemoryMB     int
	PidsLimit    int
	Tmpfs        []TmpfsMount
}

type TmpfsMount struct {
	Mount   string
	Options string
}
