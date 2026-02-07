package sandbox

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

type Manager struct {
	mode   string
	engine string
}

func NewManager(mode string) *Manager {
	if mode == "" {
		mode = "auto"
	}
	return &Manager{mode: mode}
}

func (m *Manager) Detect(ctx context.Context, forceEngine string) (EngineInfo, error) {
	if m.mode == "mock" {
		m.engine = "mock"
		return EngineInfo{Name: "mock", Version: "builtin"}, nil
	}

	if forceEngine != "" {
		if err := validateEngine(ctx, forceEngine); err != nil {
			return EngineInfo{}, err
		}
		m.engine = forceEngine
		return readVersion(ctx, forceEngine)
	}

	if m.mode == "podman" || m.mode == "docker" {
		if err := validateEngine(ctx, m.mode); err != nil {
			return EngineInfo{}, err
		}
		m.engine = m.mode
		return readVersion(ctx, m.mode)
	}

	if err := validateEngine(ctx, "podman"); err == nil {
		m.engine = "podman"
		return readVersion(ctx, "podman")
	}
	if err := validateEngine(ctx, "docker"); err == nil {
		m.engine = "docker"
		return readVersion(ctx, "docker")
	}
	return EngineInfo{}, errors.New("neither podman nor docker is available")
}

func validateEngine(ctx context.Context, engine string) error {
	if _, err := exec.LookPath(engine); err != nil {
		return fmt.Errorf("%s not found in PATH", engine)
	}
	out, err := exec.CommandContext(ctx, engine, "info").CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s info failed: %s", engine, strings.TrimSpace(string(out)))
	}
	return nil
}

func readVersion(ctx context.Context, engine string) (EngineInfo, error) {
	out, err := exec.CommandContext(ctx, engine, "--version").CombinedOutput()
	if err != nil {
		return EngineInfo{}, fmt.Errorf("%s --version failed: %w", engine, err)
	}
	return EngineInfo{Name: engine, Version: strings.TrimSpace(string(out))}, nil
}

func (m *Manager) StartLevel(ctx context.Context, spec StartSpec) (Handle, error) {
	engine := m.engine
	if m.mode == "mock" {
		engine = "mock"
	}
	if engine == "" {
		engine = "mock"
	}

	h := &containerHandle{
		engine: engine,
		name:   spec.ContainerName,
		work:   spec.WorkDir,
	}
	if engine == "mock" {
		h.shell = nil
		h.cwd = spec.WorkDir
		h.env = []string{"TERM=xterm-256color", "LANG=C.UTF-8", "LC_ALL=C"}
		for k, v := range spec.ShellEnv {
			h.env = append(h.env, fmt.Sprintf("%s=%s", k, v))
		}
		h.mock = true
		return h, nil
	}

	args := buildRunArgs(engine, spec)
	out, err := exec.CommandContext(ctx, engine, args...).CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("%s run failed: %s", engine, strings.TrimSpace(string(out)))
	}

	shellProgram := spec.ShellProgram
	if shellProgram == "" {
		shellProgram = "bash"
	}
	shellArgs := append([]string(nil), spec.ShellArgs...)
	if shellProgram == "bash" {
		filtered := make([]string, 0, len(shellArgs))
		for _, a := range shellArgs {
			if a == "--login" {
				continue
			}
			filtered = append(filtered, a)
		}
		shellArgs = filtered
	}
	if shellProgram == "bash" && !contains(shellArgs, "--rcfile") {
		shellArgs = append([]string{"--rcfile", "/dojo/bashrc"}, shellArgs...)
	}
	if len(shellArgs) == 0 {
		shellArgs = []string{"--rcfile", "/dojo/bashrc"}
	}
	workCWD := spec.ShellCWD
	if workCWD == "" {
		workCWD = spec.WorkMount
		if workCWD == "" {
			workCWD = "/work"
		}
	}

	h.shell = []string{
		engine,
		"exec", "-it",
		"-e", "TERM=xterm-256color",
		"-e", "LANG=C.UTF-8",
		"-e", "LC_ALL=C",
		"-e", "DOJO_SESSION_ID=" + spec.SessionID,
		"-e", "DOJO_LEVEL_ID=" + spec.LevelID,
		"-w", workCWD,
		spec.ContainerName,
		shellProgram,
	}
	envPairs := make([]string, 0, len(spec.ShellEnv)*2)
	for k, v := range spec.ShellEnv {
		envPairs = append(envPairs, "-e", fmt.Sprintf("%s=%s", k, v))
	}
	if len(envPairs) > 0 {
		base := append([]string{}, h.shell[:3]...)
		base = append(base, envPairs...)
		base = append(base, h.shell[3:]...)
		h.shell = base
	}
	h.shell = append(h.shell, shellArgs...)
	return h, nil
}

func buildRunArgs(engine string, spec StartSpec) []string {
	datasetMount := spec.DatasetMount
	if datasetMount == "" {
		datasetMount = "/levels/current"
	}
	workMount := spec.WorkMount
	if workMount == "" {
		workMount = "/work"
	}

	mountDataset := ""
	mountWork := ""
	if engine == "docker" {
		mountDataset = fmt.Sprintf("type=bind,src=%s,dst=%s,ro", spec.DatasetDir, datasetMount)
		mountWork = fmt.Sprintf("type=bind,src=%s,dst=%s,rw", spec.WorkDir, workMount)
	} else {
		selinux := ""
		if spec.UseSELinuxZ {
			selinux = ":Z"
		}
		mountDataset = fmt.Sprintf("%s:%s:ro%s", spec.DatasetDir, datasetMount, selinux)
		mountWork = fmt.Sprintf("%s:%s:rw%s", spec.WorkDir, workMount, selinux)
	}

	network := spec.Network
	if network == "" {
		network = "none"
	}
	cpu := spec.CPU
	if cpu <= 0 {
		cpu = 1.0
	}
	memoryMB := spec.MemoryMB
	if memoryMB <= 0 {
		memoryMB = 768
	}
	pids := spec.PidsLimit
	if pids <= 0 {
		pids = 256
	}

	args := []string{
		"run", "-d", "--name", spec.ContainerName,
		"--hostname", "dojo",
		"--cap-drop", "ALL",
		"--security-opt", "no-new-privileges",
		"--pids-limit", fmt.Sprintf("%d", pids),
		"--memory", fmt.Sprintf("%dm", memoryMB),
		"--cpus", fmt.Sprintf("%.2f", cpu),
		"--label", "clidojo.session=" + spec.SessionID,
		"--label", "clidojo.level=" + spec.LevelID,
		"--label", "clidojo.pack=" + spec.PackID,
		"-e", "TERM=xterm-256color",
		"-e", "LANG=C.UTF-8",
		"-e", "LC_ALL=C",
	}
	if engine == "podman" {
		// podman docs use lower-case "all", while docker accepts "ALL".
		args[7] = "all"
	}
	if network != "inherit" {
		args = append(args[:4], append([]string{"--network", network}, args[4:]...)...)
	}
	if spec.ReadOnlyRoot {
		args = append(args, "--read-only")
	}
	if len(spec.Tmpfs) == 0 {
		spec.Tmpfs = []TmpfsMount{
			{Mount: "/tmp", Options: "rw,noexec,nosuid,size=128m"},
			{Mount: "/run", Options: "rw,noexec,nosuid,size=16m"},
		}
	}
	for _, tm := range spec.Tmpfs {
		if tm.Mount == "" {
			continue
		}
		opt := tm.Mount
		if tm.Options != "" {
			opt = tm.Mount + ":" + tm.Options
		}
		args = append(args, "--tmpfs", opt)
	}
	for k, v := range spec.ShellEnv {
		args = append(args, "-e", fmt.Sprintf("%s=%s", k, v))
	}

	if engine == "docker" {
		args = append(args, "--mount", mountDataset, "--mount", mountWork)
	} else {
		args = append(args, "-v", mountDataset, "-v", mountWork)
	}

	args = append(args, spec.Image, "sleep", "infinity")
	return args
}

func (m *Manager) CleanupOrphans(ctx context.Context, activeSession string) error {
	if m.engine == "" || m.engine == "mock" {
		return nil
	}
	engine := m.engine

	listCmd := exec.CommandContext(ctx, engine, "ps", "-a", "--filter", "label=clidojo.session", "--format", "{{.ID}}")
	out, err := listCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("list containers: %s", strings.TrimSpace(string(out)))
	}
	ids := strings.Fields(string(out))
	for _, id := range ids {
		if activeSession != "" {
			labelOut, err := exec.CommandContext(ctx, engine, "inspect", "--format", "{{ index .Config.Labels \"clidojo.session\" }}", id).CombinedOutput()
			if err == nil {
				if strings.TrimSpace(string(labelOut)) == activeSession {
					continue
				}
			}
		}
		_ = exec.CommandContext(ctx, engine, "rm", "-f", id).Run()
	}
	return nil
}

type containerHandle struct {
	engine string
	name   string
	work   string
	cwd    string
	env    []string
	shell  []string
	mock   bool
}

func (h *containerHandle) ShellCommand() []string {
	return append([]string(nil), h.shell...)
}

func (h *containerHandle) Stop(ctx context.Context) error {
	if h.engine == "mock" || h.name == "" {
		return nil
	}
	out, err := exec.CommandContext(ctx, h.engine, "rm", "-f", h.name).CombinedOutput()
	if err != nil {
		return fmt.Errorf("container cleanup failed: %s", strings.TrimSpace(string(out)))
	}
	return nil
}

func (h *containerHandle) WorkDir() string       { return h.work }
func (h *containerHandle) ContainerName() string { return h.name }
func (h *containerHandle) Cwd() string {
	if h.cwd != "" {
		return h.cwd
	}
	return ""
}
func (h *containerHandle) Env() []string { return append([]string(nil), h.env...) }
func (h *containerHandle) IsMock() bool  { return h.mock }

func contains(list []string, target string) bool {
	for _, s := range list {
		if s == target {
			return true
		}
	}
	return false
}
