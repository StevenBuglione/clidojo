package devtools

import (
	"context"

	"clidojo/internal/grading"
	"clidojo/internal/term"
)

type Demo interface {
	Resolve(name string) Scenario
	SetState(ctx context.Context, cacheDir string, state string, rendered bool) error
	PlaybackFrames(levelID, scenario string) []term.PlaybackFrame
	MockCmdLog(levelID string) string
	MockGrade(req MockGradeRequest) grading.Result
}

type MockGradeRequest struct {
	LevelID        string
	Checks         []grading.CheckSpec
	Attempt        int
	BasePoints     int
	HintsUsed      int
	Resets         int
	TimePenalty    int
	HintPenalty    int
	ResetPenalty   int
	GraceSeconds   int
	ElapsedSeconds int
	PackID         string
	PackVersion    string
}
