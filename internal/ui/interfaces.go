package ui

import "time"

type Controller interface {
	OnCheck()
	OnReset()
	OnMenu()
	OnHints()
	OnGoal()
	OnJournal()
	OnQuit()
	OnResize(cols, rows int)
	OnTerminalInput(data []byte)
	OnChangeLevel()
	OnOpenSettings()
	OnOpenStats()
	OnRevealHint()
	OnNextLevel()
	OnTryAgain()
	OnShowReferenceSolutions()
	OnOpenDiff()
	OnJournalExplainAI()
}

type View interface {
	Run() error
	Stop()
	SetController(Controller)
	SetPlayingState(PlayingState)
	SetTooSmall(cols, rows int)
	SetSetupError(msg, details string)
	SetMenuOpen(open bool)
	SetHintsOpen(open bool)
	SetGoalOpen(open bool)
	SetJournalOpen(open bool)
	SetResetConfirmOpen(open bool)
	SetResult(state ResultState)
	SetJournalEntries(entries []JournalEntry)
	SetReferenceText(text string, open bool)
	SetDiffText(text string, open bool)
	SetInfo(title, text string, open bool)
	FlashStatus(msg string)
}

type LayoutMode int

const (
	LayoutWide LayoutMode = iota
	LayoutMedium
	LayoutTooSmall
)

type PlayingState struct {
	ModeLabel  string
	PackID     string
	LevelID    string
	Objective  []string
	Checks     []CheckRow
	Hints      []HintRow
	Engine     string
	StartedAt  time.Time
	HintsUsed  int
	Resets     int
	Score      int
	Streak     int
	Badges     []string
	HudFocused bool
}

type HintRow struct {
	Text       string
	Revealed   bool
	Locked     bool
	LockReason string
}

type CheckRow struct {
	ID          string
	Description string
	Status      string
}

type ResultState struct {
	Visible          bool
	Passed           bool
	Summary          string
	Checks           []CheckResultRow
	Score            int
	Breakdown        []BreakdownRow
	CanShowReference bool
	CanOpenDiff      bool
	PrimaryAction    string
}

type BreakdownRow struct {
	Label string
	Value string
}

type CheckResultRow struct {
	ID      string
	Passed  bool
	Message string
}

type JournalEntry struct {
	Timestamp string
	Command   string
	Tags      []string
}
