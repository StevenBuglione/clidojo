package grading

import "time"

const (
	ResultKind    = "grader_result"
	SchemaVersion = 1
)

type Request struct {
	AppVersion  string
	PackID      string
	PackVersion string
	LevelID     string

	RunID      string
	Attempt    int
	StartedAt  time.Time
	FinishedAt time.Time

	Engine    string
	Container string
	ImageRef  string
	WorkDir   string
	Checks    []CheckSpec

	BasePoints           int
	TimeGraceSeconds     int
	TimePenaltyPerSecond int
	HintPenaltyPoints    int
	ResetPenaltyPoints   int
	HintsUsed            int
	Resets               int
}

type CheckSpec struct {
	ID            string
	Type          string
	Description   string
	Required      bool
	Points        int
	OnFailMessage string
	OnPassMessage string

	Path      string
	Expected  string
	Normalize NormalizeSpec

	Equals int
	Min    *int
	Max    *int

	Pattern    string
	Mode       string
	MinMatches int

	Order      string
	Key        string
	Unique     bool
	IgnoreCase bool
	Split      FileSplitSpec
	Column     int

	Command        string
	CompareToPath  string
	TimeoutSeconds int
	MinCount       int
}

type NormalizeSpec struct {
	Newlines               string
	TrimTrailingWhitespace bool
	TrimFinalNewline       bool
}

type FileSplitSpec struct {
	Kind      string
	Delimiter string
}

type Result struct {
	Kind          string `json:"kind"`
	SchemaVersion int    `json:"schema_version"`

	AppVersion  string `json:"app_version,omitempty"`
	PackID      string `json:"pack_id"`
	PackVersion string `json:"pack_version"`
	LevelID     string `json:"level_id"`

	Run            RunInfo         `json:"run"`
	Passed         bool            `json:"passed"`
	Score          Score           `json:"score"`
	Checks         []CheckResult   `json:"checks"`
	Artifacts      []Artifact      `json:"artifacts,omitempty"`
	CmdlogAnalysis *CmdlogAnalysis `json:"cmdlog_analysis,omitempty"`
	EngineDebug    EngineDebug     `json:"engine_debug,omitempty"`
}

type RunInfo struct {
	RunID            string `json:"run_id"`
	Attempt          int    `json:"attempt"`
	StartedAtUnixMS  int64  `json:"started_at_unix_ms"`
	FinishedAtUnixMS int64  `json:"finished_at_unix_ms"`
	DurationMS       int64  `json:"duration_ms"`
}

type Score struct {
	BasePoints          int          `json:"base_points"`
	TimeGraceSeconds    int          `json:"time_grace_seconds,omitempty"`
	TimePenaltyPoints   int          `json:"time_penalty_points,omitempty"`
	HintPenaltyPoints   int          `json:"hint_penalty_points,omitempty"`
	ResetPenaltyPoints  int          `json:"reset_penalty_points,omitempty"`
	OptionalBonusPoints int          `json:"optional_bonus_points,omitempty"`
	TotalPoints         int          `json:"total_points"`
	Breakdown           []ScoreDelta `json:"breakdown,omitempty"`
}

type ScoreDelta struct {
	Kind        string `json:"kind"`
	Points      int    `json:"points"`
	Description string `json:"description"`
}

type CheckResult struct {
	ID            string        `json:"id"`
	Type          string        `json:"type"`
	Required      bool          `json:"required"`
	Passed        bool          `json:"passed"`
	PointsAwarded int           `json:"points_awarded,omitempty"`
	Summary       string        `json:"summary,omitempty"`
	Message       string        `json:"message,omitempty"`
	Artifacts     []ArtifactRef `json:"artifacts,omitempty"`
}

type ArtifactRef struct {
	Kind string `json:"kind"`
	Ref  string `json:"ref"`
}

type Artifact struct {
	Ref         string `json:"ref"`
	Kind        string `json:"kind"`
	Title       string `json:"title,omitempty"`
	TextPreview string `json:"text_preview,omitempty"`
}

type CmdlogAnalysis struct {
	CmdCount        int            `json:"cmd_count"`
	MatchedPatterns []PatternCount `json:"matched_patterns,omitempty"`
}

type PatternCount struct {
	PatternID string `json:"pattern_id"`
	Count     int    `json:"count"`
}

type EngineDebug struct {
	Engine        string `json:"engine,omitempty"`
	ContainerName string `json:"container_name,omitempty"`
	ImageRef      string `json:"image_ref,omitempty"`
}

type evaluation struct {
	Passed        bool
	Summary       string
	Message       string
	PointsAwarded int
	Artifact      *Artifact
	PatternCount  *PatternCount
}
