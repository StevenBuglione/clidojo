package levels

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

type FSLoader struct{}

func NewLoader() *FSLoader { return &FSLoader{} }

func (l *FSLoader) LoadPacks(ctx context.Context, root string) ([]Pack, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}

	packs := make([]Pack, 0)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		packPath := filepath.Join(root, entry.Name())
		packYAML := filepath.Join(packPath, "pack.yaml")
		if _, err := os.Stat(packYAML); err != nil {
			continue
		}
		pack, err := readPack(packYAML)
		if err != nil {
			return nil, fmt.Errorf("load pack %s: %w", packPath, err)
		}
		pack.Path = packPath
		applyPackDefaults(&pack)
		if err := validatePackBuildPath(pack); err != nil {
			return nil, fmt.Errorf("%s: %w", packPath, err)
		}

		levels, err := l.readLevels(ctx, pack)
		if err != nil {
			return nil, err
		}
		pack.LoadedLevels = levels
		packs = append(packs, pack)
	}

	sort.Slice(packs, func(i, j int) bool { return packs[i].PackID < packs[j].PackID })
	return packs, nil
}

func readPack(path string) (Pack, error) {
	var pack Pack
	b, err := os.ReadFile(path)
	if err != nil {
		return pack, err
	}
	if err := yaml.Unmarshal(b, &pack); err != nil {
		return pack, err
	}
	if err := pack.Validate(); err != nil {
		return pack, err
	}
	return pack, nil
}

func validatePackBuildPath(pack Pack) error {
	if pack.Image.Build == nil {
		return nil
	}
	if pack.Image.Build.ContextDir == "" {
		return fmt.Errorf("image.build.context_dir is required")
	}
	ctxDir := filepath.Join(pack.Path, pack.Image.Build.ContextDir)
	if _, err := os.Stat(ctxDir); err != nil {
		return fmt.Errorf("image.build.context_dir does not exist: %s", ctxDir)
	}
	return nil
}

func applyPackDefaults(pack *Pack) {
	if pack.Defaults.Shell.Program == "" {
		pack.Defaults.Shell.Program = "bash"
	}
	if len(pack.Defaults.Shell.Args) == 0 {
		pack.Defaults.Shell.Args = []string{"--login"}
	}
	if pack.Defaults.Sandbox.Network == "" {
		pack.Defaults.Sandbox.Network = "none"
	}
	if pack.Defaults.Sandbox.CPU <= 0 {
		pack.Defaults.Sandbox.CPU = 1.0
	}
	if pack.Defaults.Sandbox.MemoryMB <= 0 {
		pack.Defaults.Sandbox.MemoryMB = 768
	}
	if pack.Defaults.Sandbox.PidsLimit <= 0 {
		pack.Defaults.Sandbox.PidsLimit = 256
	}
	if len(pack.Defaults.Sandbox.Tmpfs) == 0 {
		pack.Defaults.Sandbox.Tmpfs = []TmpfsSpec{{Mount: "/tmp", Options: "rw,noexec,nosuid,size=128m"}, {Mount: "/run", Options: "rw,noexec,nosuid,size=16m"}}
	}
	if pack.Defaults.UI.HUDWidth <= 0 {
		pack.Defaults.UI.HUDWidth = 42
	}
	if pack.Defaults.UI.MinCols <= 0 {
		pack.Defaults.UI.MinCols = 90
	}
	if pack.Defaults.UI.MinRows <= 0 {
		pack.Defaults.UI.MinRows = 24
	}
	if pack.Defaults.Sandbox.ReadOnlyRoot == nil {
		v := true
		pack.Defaults.Sandbox.ReadOnlyRoot = &v
	}
}

func (l *FSLoader) readLevels(ctx context.Context, pack Pack) ([]Level, error) {
	if len(pack.Levels) > 0 {
		return l.readLevelsFromManifest(ctx, pack)
	}
	return l.readLevelsFromScan(ctx, pack)
}

func (l *FSLoader) readLevelsFromManifest(ctx context.Context, pack Pack) ([]Level, error) {
	levels := make([]Level, 0, len(pack.Levels))
	for _, ref := range pack.Levels {
		if ref.Enabled != nil && !*ref.Enabled {
			continue
		}
		levelDir := filepath.Join(pack.Path, ref.Path)
		levelYAML := filepath.Join(levelDir, "level.yaml")
		level, err := loadLevelFile(levelYAML)
		if err != nil {
			return nil, err
		}
		if level.LevelID != ref.LevelID {
			return nil, fmt.Errorf("level id mismatch for %s: manifest=%s file=%s", levelYAML, ref.LevelID, level.LevelID)
		}
		if err := hydrateLevel(ctx, &level, pack, levelDir); err != nil {
			return nil, err
		}
		levels = append(levels, level)
	}
	return levels, nil
}

func (l *FSLoader) readLevelsFromScan(ctx context.Context, pack Pack) ([]Level, error) {
	levelRoot := filepath.Join(pack.Path, "levels")
	entries, err := os.ReadDir(levelRoot)
	if err != nil {
		return nil, err
	}
	levels := make([]Level, 0)
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		ly := filepath.Join(levelRoot, e.Name(), "level.yaml")
		if _, err := os.Stat(ly); err != nil {
			continue
		}
		level, err := loadLevelFile(ly)
		if err != nil {
			return nil, err
		}
		if err := hydrateLevel(ctx, &level, pack, filepath.Dir(ly)); err != nil {
			return nil, err
		}
		levels = append(levels, level)
	}
	sort.Slice(levels, func(i, j int) bool { return levels[i].LevelID < levels[j].LevelID })
	return levels, nil
}

func loadLevelFile(path string) (Level, error) {
	var level Level
	b, err := os.ReadFile(path)
	if err != nil {
		return level, err
	}
	if err := yaml.Unmarshal(b, &level); err != nil {
		return level, fmt.Errorf("parse %s: %w", path, err)
	}
	if err := level.Validate(); err != nil {
		return level, fmt.Errorf("validate %s: %w", path, err)
	}
	return level, nil
}

func hydrateLevel(ctx context.Context, level *Level, pack Pack, levelDir string) error {
	level.Path = levelDir
	level.DatasetHostPath = filepath.Join(levelDir, level.Filesystem.Dataset.Path)

	if level.Filesystem.Dataset.Source == "generator" {
		if level.Filesystem.Dataset.Generator == nil {
			return fmt.Errorf("level %s dataset source=generator requires generator section", level.LevelID)
		}
		if err := runGenerator(ctx, *level); err != nil {
			return err
		}
	}

	if _, err := os.Stat(level.DatasetHostPath); err != nil {
		return fmt.Errorf("dataset path not found for level %s: %s", level.LevelID, level.DatasetHostPath)
	}

	applyLevelDefaults(level, pack)
	return nil
}

func applyLevelDefaults(level *Level, pack Pack) {
	if level.Shell.Program == "" {
		level.Shell.Program = pack.Defaults.Shell.Program
	}
	if len(level.Shell.Args) == 0 {
		level.Shell.Args = append([]string(nil), pack.Defaults.Shell.Args...)
	}
	if level.Shell.CWD == "" {
		level.Shell.CWD = "/work"
	}

	if level.Sandbox.Network == "" {
		level.Sandbox.Network = pack.Defaults.Sandbox.Network
	}
	if level.Sandbox.ReadOnlyRoot == nil {
		level.Sandbox.ReadOnlyRoot = pack.Defaults.Sandbox.ReadOnlyRoot
	}
	if level.Sandbox.CPU <= 0 {
		level.Sandbox.CPU = pack.Defaults.Sandbox.CPU
	}
	if level.Sandbox.MemoryMB <= 0 {
		level.Sandbox.MemoryMB = pack.Defaults.Sandbox.MemoryMB
	}
	if level.Sandbox.PidsLimit <= 0 {
		level.Sandbox.PidsLimit = pack.Defaults.Sandbox.PidsLimit
	}
	if len(level.Sandbox.Tmpfs) == 0 {
		level.Sandbox.Tmpfs = append([]TmpfsSpec(nil), pack.Defaults.Sandbox.Tmpfs...)
	}

	if level.Scoring.BasePoints <= 0 {
		level.Scoring.BasePoints = 1000
	}
	if level.Scoring.TimeGraceSeconds <= 0 {
		level.Scoring.TimeGraceSeconds = 60
	}
	if level.Scoring.TimePenaltyPerSecond <= 0 {
		level.Scoring.TimePenaltyPerSecond = 1
	}
	if level.Scoring.HintPenaltyPoints <= 0 {
		level.Scoring.HintPenaltyPoints = 80
	}
	if level.Scoring.ResetPenaltyPoints <= 0 {
		level.Scoring.ResetPenaltyPoints = 120
	}

	if level.Filesystem.Dataset.MountPoint == "" {
		level.Filesystem.Dataset.MountPoint = "/levels/current"
	}
	if level.Filesystem.Work.MountPoint == "" {
		level.Filesystem.Work.MountPoint = "/work"
	}
	for i := range level.Checks {
		if level.Checks[i].Required == nil {
			v := true
			level.Checks[i].Required = &v
		}
	}
}

func runGenerator(ctx context.Context, level Level) error {
	gen := level.Filesystem.Dataset.Generator
	if gen == nil {
		return nil
	}
	if gen.Command == "" {
		return fmt.Errorf("level %s generator.command is required", level.LevelID)
	}
	cmd := exec.CommandContext(ctx, gen.Command, gen.Args...)
	cmd.Dir = level.Path
	cmd.Env = os.Environ()
	if gen.Seed != nil {
		cmd.Env = append(cmd.Env, fmt.Sprintf("DOJO_DATASET_SEED=%d", *gen.Seed))
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("generator failed for level %s: %s", level.LevelID, strings.TrimSpace(string(out)))
	}
	return nil
}

func (l *FSLoader) FindLevel(packs []Pack, packID string, levelID string) (Pack, Level, error) {
	for _, p := range packs {
		if p.PackID != packID {
			continue
		}
		for _, lv := range p.LoadedLevels {
			if lv.LevelID == levelID {
				return p, lv, nil
			}
		}
	}
	return Pack{}, Level{}, fmt.Errorf("level %s/%s not found", packID, levelID)
}

func (l *FSLoader) StageWorkdir(level Level, workdir string) error {
	if err := os.RemoveAll(workdir); err != nil {
		return err
	}
	if err := os.MkdirAll(workdir, 0o755); err != nil {
		return err
	}

	for _, dir := range level.Filesystem.Work.InitialLayout.Mkdirs {
		target := filepath.Join(workdir, dir)
		if err := os.MkdirAll(target, 0o755); err != nil {
			return err
		}
	}
	for _, cp := range level.Filesystem.Work.InitialLayout.CopyFromDataset {
		src := filepath.Join(level.DatasetHostPath, cp.From)
		dst := filepath.Join(workdir, cp.To)
		if err := copyPath(src, dst); err != nil {
			return fmt.Errorf("copy_from_dataset from=%s to=%s: %w", cp.From, cp.To, err)
		}
	}
	return nil
}

func copyPath(src, dst string) error {
	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	if info.IsDir() {
		if err := os.MkdirAll(dst, info.Mode()); err != nil {
			return err
		}
		return filepath.WalkDir(src, func(path string, d fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			rel, err := filepath.Rel(src, path)
			if err != nil {
				return err
			}
			if rel == "." {
				return nil
			}
			target := filepath.Join(dst, rel)
			if d.IsDir() {
				return os.MkdirAll(target, 0o755)
			}
			return copyFile(path, target)
		})
	}
	return copyFile(src, dst)
}

func copyFile(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Chmod(0o644)
}
