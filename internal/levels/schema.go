package levels

import (
	"fmt"
	"regexp"
)

const (
	PackKind               = "pack"
	LevelKind              = "level"
	SupportedSchemaVersion = 1
)

var idPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{2,63}$`)

type Pack struct {
	Kind          string         `yaml:"kind"`
	SchemaVersion int            `yaml:"schema_version"`
	PackID        string         `yaml:"pack_id"`
	Name          string         `yaml:"name"`
	Version       string         `yaml:"version"`
	DescriptionMD string         `yaml:"description_md"`
	Image         PackImage      `yaml:"image"`
	Defaults      PackDefaults   `yaml:"defaults"`
	Tools         []PackTool     `yaml:"tools"`
	Levels        []PackLevelRef `yaml:"levels"`
	Extensions    map[string]any `yaml:"extensions"`

	Path         string  `yaml:"-"`
	LoadedLevels []Level `yaml:"-"`
}

type PackImage struct {
	Ref   string          `yaml:"ref"`
	Build *PackImageBuild `yaml:"build"`
	Pull  bool            `yaml:"pull"`
}

type PackImageBuild struct {
	ContextDir string            `yaml:"context_dir"`
	Dockerfile string            `yaml:"dockerfile"`
	Target     string            `yaml:"target"`
	Args       map[string]string `yaml:"args"`
}

type PackDefaults struct {
	Shell   ShellSpec   `yaml:"shell"`
	Sandbox SandboxSpec `yaml:"sandbox"`
	UI      UISpec      `yaml:"ui"`
}

type ShellSpec struct {
	Program string            `yaml:"program"`
	Args    []string          `yaml:"args"`
	CWD     string            `yaml:"cwd"`
	Env     map[string]string `yaml:"env"`
}

type SandboxSpec struct {
	Network      string      `yaml:"network"`
	ReadOnlyRoot *bool       `yaml:"read_only_root"`
	CPU          float64     `yaml:"cpu"`
	MemoryMB     int         `yaml:"memory_mb"`
	PidsLimit    int         `yaml:"pids_limit"`
	Tmpfs        []TmpfsSpec `yaml:"tmpfs"`
}

type TmpfsSpec struct {
	Mount   string `yaml:"mount"`
	Options string `yaml:"options"`
}

type UISpec struct {
	HUDWidth int `yaml:"hud_width"`
	MinCols  int `yaml:"min_cols"`
	MinRows  int `yaml:"min_rows"`
}

type PackTool struct {
	ToolID         string `yaml:"tool_id"`
	Name           string `yaml:"name"`
	SummaryMD      string `yaml:"summary_md"`
	DifficultyBias int    `yaml:"difficulty_bias"`
}

type PackLevelRef struct {
	LevelID string `yaml:"level_id"`
	Path    string `yaml:"path"`
	Enabled *bool  `yaml:"enabled"`
}

type Level struct {
	Kind               string               `yaml:"kind"`
	SchemaVersion      int                  `yaml:"schema_version"`
	LevelID            string               `yaml:"level_id"`
	Title              string               `yaml:"title"`
	SummaryMD          string               `yaml:"summary_md"`
	DescriptionMD      string               `yaml:"description_md"`
	Difficulty         int                  `yaml:"difficulty"`
	EstimatedMinutes   int                  `yaml:"estimated_minutes"`
	Tags               []string             `yaml:"tags"`
	ToolFocus          []string             `yaml:"tool_focus"`
	Image              ImageOverride        `yaml:"image"`
	Shell              ShellSpec            `yaml:"shell"`
	Sandbox            SandboxSpec          `yaml:"sandbox"`
	Filesystem         FilesystemSpec       `yaml:"filesystem"`
	Objective          ObjectiveSpec        `yaml:"objective"`
	Hints              []HintSpec           `yaml:"hints"`
	Checks             []CheckSpec          `yaml:"checks"`
	Scoring            ScoringSpec          `yaml:"scoring"`
	ReferenceSolutions []ReferenceSolution  `yaml:"reference_solutions"`
	UI                 UISpec               `yaml:"ui"`
	XAutoCheck         AutoCheckExtension   `yaml:"x-autocheck"`
	XProgression       ProgressionExtension `yaml:"x-progression"`
	XTeaching          TeachingExtension    `yaml:"x-teaching"`
	Extensions         map[string]any       `yaml:"extensions"`

	Path            string `yaml:"-"`
	DatasetHostPath string `yaml:"-"`
}

type ImageOverride struct {
	Ref string `yaml:"ref"`
}

type FilesystemSpec struct {
	Dataset DatasetSpec `yaml:"dataset"`
	Work    WorkSpec    `yaml:"work"`
}

type DatasetSpec struct {
	Source     string         `yaml:"source"`
	Path       string         `yaml:"path"`
	MountPoint string         `yaml:"mount_point"`
	ReadOnly   *bool          `yaml:"read_only"`
	Generator  *GeneratorSpec `yaml:"generator"`
}

type GeneratorSpec struct {
	Command string   `yaml:"command"`
	Args    []string `yaml:"args"`
	Seed    *int64   `yaml:"seed"`
}

type WorkSpec struct {
	MountPoint    string        `yaml:"mount_point"`
	InitialLayout InitialLayout `yaml:"initial_layout"`
}

type InitialLayout struct {
	Mkdirs          []string      `yaml:"mkdirs"`
	CopyFromDataset []CopyMapping `yaml:"copy_from_dataset"`
}

type CopyMapping struct {
	From string `yaml:"from"`
	To   string `yaml:"to"`
}

type ObjectiveSpec struct {
	Bullets       []string `yaml:"bullets"`
	SuccessHintMD string   `yaml:"success_hint_md"`
}

type HintSpec struct {
	HintID     string     `yaml:"hint_id"`
	TextMD     string     `yaml:"text_md"`
	Unlock     HintUnlock `yaml:"unlock"`
	CostPoints *int       `yaml:"cost_points"`
}

type HintUnlock struct {
	AfterSeconds      int `yaml:"after_seconds"`
	AfterFailedChecks int `yaml:"after_failed_checks"`
	AfterReveals      int `yaml:"after_reveals"`
}

type CheckSpec struct {
	ID            string `yaml:"id"`
	Type          string `yaml:"type"`
	Description   string `yaml:"description"`
	Required      *bool  `yaml:"required"`
	Points        int    `yaml:"points"`
	OnFailMessage string `yaml:"on_fail_message"`
	OnPassMessage string `yaml:"on_pass_message"`

	Path      string        `yaml:"path"`
	Expected  string        `yaml:"expected"`
	Normalize NormalizeSpec `yaml:"normalize"`

	Equals int  `yaml:"equals"`
	Min    *int `yaml:"min"`
	Max    *int `yaml:"max"`

	Pattern    string `yaml:"pattern"`
	Mode       string `yaml:"mode"`
	MinMatches int    `yaml:"min_matches"`

	Order      string        `yaml:"order"`
	Key        string        `yaml:"key"`
	Unique     bool          `yaml:"unique"`
	IgnoreCase bool          `yaml:"ignore_case"`
	Split      FileSplitSpec `yaml:"split"`
	Column     int           `yaml:"column"`

	Command        string `yaml:"command"`
	CompareToPath  string `yaml:"compare_to_path"`
	TimeoutSeconds int    `yaml:"timeout_seconds"`

	MinCount int `yaml:"min_count"`
}

type NormalizeSpec struct {
	Newlines               string `yaml:"newlines"`
	TrimTrailingWhitespace bool   `yaml:"trim_trailing_whitespace"`
	TrimFinalNewline       bool   `yaml:"trim_final_newline"`
}

type FileSplitSpec struct {
	Kind      string `yaml:"kind"`
	Delimiter string `yaml:"delimiter"`
}

type ScoringSpec struct {
	BasePoints           int           `yaml:"base_points"`
	TimeGraceSeconds     int           `yaml:"time_grace_seconds"`
	TimePenaltyPerSecond int           `yaml:"time_penalty_per_second"`
	HintPenaltyPoints    int           `yaml:"hint_penalty_points"`
	ResetPenaltyPoints   int           `yaml:"reset_penalty_points"`
	CmdlogBonuses        []CmdlogBonus `yaml:"cmdlog_bonuses"`
}

type CmdlogBonus struct {
	ID          string `yaml:"id"`
	Description string `yaml:"description"`
	Pattern     string `yaml:"pattern"`
	Points      int    `yaml:"points"`
}

type ReferenceSolution struct {
	SolutionID    string   `yaml:"solution_id"`
	Title         string   `yaml:"title"`
	ScriptSH      string   `yaml:"script_sh"`
	ExplanationMD string   `yaml:"explanation_md"`
	Tags          []string `yaml:"tags"`
}

type AutoCheckExtension struct {
	Mode       string `yaml:"mode"`
	DebounceMS int    `yaml:"debounce_ms"`
	QuietFail  *bool  `yaml:"quiet_fail"`
}

type ProgressionExtension struct {
	Tier          int                `yaml:"tier"`
	Prerequisites []string           `yaml:"prerequisites"`
	Mastery       ProgressionMastery `yaml:"mastery"`
}

type ProgressionMastery struct {
	MinScore  int `yaml:"min_score"`
	MaxHints  int `yaml:"max_hints"`
	MaxResets int `yaml:"max_resets"`
}

type TeachingExtension struct {
	Concepts   []string `yaml:"concepts"`
	ReviewDays []int    `yaml:"review_days"`
}

func (p Pack) Validate() error {
	if p.Kind != PackKind {
		return fmt.Errorf("kind must be %q", PackKind)
	}
	if p.SchemaVersion == 0 {
		return fmt.Errorf("schema_version is required")
	}
	if p.SchemaVersion > SupportedSchemaVersion {
		return fmt.Errorf("unsupported pack schema_version %d (max supported %d)", p.SchemaVersion, SupportedSchemaVersion)
	}
	if !idPattern.MatchString(p.PackID) {
		return fmt.Errorf("invalid pack_id %q", p.PackID)
	}
	if p.Name == "" {
		return fmt.Errorf("name is required")
	}
	if p.Version == "" {
		return fmt.Errorf("version is required")
	}
	if p.Image.Ref == "" {
		return fmt.Errorf("image.ref is required")
	}
	seen := map[string]struct{}{}
	for _, l := range p.Levels {
		if l.LevelID == "" {
			return fmt.Errorf("levels[].level_id is required")
		}
		if _, ok := seen[l.LevelID]; ok {
			return fmt.Errorf("duplicate level_id %q in pack.yaml", l.LevelID)
		}
		seen[l.LevelID] = struct{}{}
	}
	return nil
}

func (l Level) Validate() error {
	if l.Kind != LevelKind {
		return fmt.Errorf("kind must be %q", LevelKind)
	}
	if l.SchemaVersion == 0 {
		return fmt.Errorf("schema_version is required")
	}
	if l.SchemaVersion > SupportedSchemaVersion {
		return fmt.Errorf("unsupported level schema_version %d (max supported %d)", l.SchemaVersion, SupportedSchemaVersion)
	}
	if !idPattern.MatchString(l.LevelID) {
		return fmt.Errorf("invalid level_id %q", l.LevelID)
	}
	if l.Title == "" {
		return fmt.Errorf("title is required")
	}
	if l.Difficulty < 1 || l.Difficulty > 5 {
		return fmt.Errorf("difficulty must be 1..5")
	}
	if l.EstimatedMinutes <= 0 {
		return fmt.Errorf("estimated_minutes must be >0")
	}
	if l.Filesystem.Dataset.Path == "" {
		return fmt.Errorf("filesystem.dataset.path is required")
	}
	if l.Filesystem.Dataset.Source == "" {
		return fmt.Errorf("filesystem.dataset.source is required")
	}
	if l.Filesystem.Dataset.MountPoint == "" {
		return fmt.Errorf("filesystem.dataset.mount_point is required")
	}
	if l.Filesystem.Work.MountPoint == "" {
		return fmt.Errorf("filesystem.work.mount_point is required")
	}
	if l.Filesystem.Dataset.MountPoint[0] != '/' || l.Filesystem.Work.MountPoint[0] != '/' {
		return fmt.Errorf("filesystem mount points must start with /")
	}
	if len(l.Objective.Bullets) == 0 {
		return fmt.Errorf("objective.bullets must contain at least one item")
	}
	seenHints := map[string]struct{}{}
	for _, h := range l.Hints {
		if h.HintID == "" {
			return fmt.Errorf("hints[].hint_id is required")
		}
		if _, ok := seenHints[h.HintID]; ok {
			return fmt.Errorf("duplicate hint_id %q", h.HintID)
		}
		seenHints[h.HintID] = struct{}{}
	}
	seenChecks := map[string]struct{}{}
	requiredCount := 0
	for _, c := range l.Checks {
		if c.ID == "" {
			return fmt.Errorf("checks[].id is required")
		}
		if _, ok := seenChecks[c.ID]; ok {
			return fmt.Errorf("duplicate checks id %q", c.ID)
		}
		seenChecks[c.ID] = struct{}{}
		required := c.Required == nil || *c.Required
		if required {
			requiredCount++
		}
		if c.Path != "" && c.Path[0] != '/' {
			return fmt.Errorf("check %q path must start with /", c.ID)
		}
		if c.CompareToPath != "" && c.CompareToPath[0] != '/' {
			return fmt.Errorf("check %q compare_to_path must start with /", c.ID)
		}
	}
	if requiredCount == 0 {
		return fmt.Errorf("level must have at least one required check")
	}
	switch l.XAutoCheck.Mode {
	case "", "off", "command_debounce", "command_and_fs_debounce":
	default:
		return fmt.Errorf("invalid x-autocheck.mode %q", l.XAutoCheck.Mode)
	}
	if l.XAutoCheck.DebounceMS < 0 {
		return fmt.Errorf("x-autocheck.debounce_ms must be >= 0")
	}
	if l.XProgression.Tier < 0 {
		return fmt.Errorf("x-progression.tier must be >= 0")
	}
	for _, day := range l.XTeaching.ReviewDays {
		if day <= 0 {
			return fmt.Errorf("x-teaching.review_days entries must be > 0")
		}
	}
	return nil
}
