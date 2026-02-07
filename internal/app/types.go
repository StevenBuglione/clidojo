package app

import "time"

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
	DatasetDir    string
	WorkDir       string
	UseSELinuxZ   bool
}

type LevelRun struct {
	SessionID string
	PackID    string
	LevelID   string
	StartTS   time.Time
}

type PlayingState struct {
	ModeLabel     string
	PackID        string
	LevelID       string
	Objective     []string
	Checks        []CheckRow
	Hints         []string
	Score         ScoreState
	Engine        string
	StartedAt     time.Time
	HudFocused    bool
	DrawerVisible bool
	LayoutMode    LayoutMode
}

type LayoutMode int

const (
	LayoutWide LayoutMode = iota
	LayoutMedium
	LayoutTooSmall
)

type CheckRow struct {
	ID          string
	Description string
	Status      string
}

type ScoreState struct {
	Elapsed   time.Duration
	HintsUsed int
	Resets    int
	Current   int
	Streak    int
	Badges    []string
}

type ResultState struct {
	Visible  bool
	Passed   bool
	Summary  string
	Checks   []CheckResultRow
	Score    int
	CanNext  bool
	CanRetry bool
}

type CheckResultRow struct {
	ID      string
	Passed  bool
	Message string
}
