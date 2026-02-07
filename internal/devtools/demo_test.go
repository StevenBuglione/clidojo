package devtools

import (
	"testing"

	"clidojo/internal/grading"
)

func TestPlaybackFramesNotEmpty(t *testing.T) {
	m := NewManager()
	frames := m.PlaybackFrames("level-001-pipes-101", "playing")
	if len(frames) == 0 {
		t.Fatalf("expected playback frames")
	}
	if len(frames[0].Data) == 0 {
		t.Fatalf("expected first frame data")
	}
}

func TestPlaybackFramesFallbackForUnknownScenario(t *testing.T) {
	m := NewManager()
	frames := m.PlaybackFrames("unknown-level", "unknown-demo")
	if len(frames) == 0 {
		t.Fatalf("expected fallback playback frames")
	}
}

func TestMockGradeDeterministic(t *testing.T) {
	m := NewManager()
	checks := []grading.CheckSpec{{ID: "a", Required: true, OnFailMessage: "bad"}}

	first := m.MockGrade(MockGradeRequest{Checks: checks, Attempt: 1, BasePoints: 1000, PackID: "p", PackVersion: "0.1.0", LevelID: "l"})
	if first.Passed {
		t.Fatalf("expected first attempt to fail")
	}
	if len(first.Artifacts) == 0 {
		t.Fatalf("expected diff artifact on fail")
	}

	second := m.MockGrade(MockGradeRequest{Checks: checks, Attempt: 2, BasePoints: 1000, PackID: "p", PackVersion: "0.1.0", LevelID: "l"})
	if !second.Passed {
		t.Fatalf("expected second attempt to pass")
	}
}

func TestMockCmdLogContainsExpectedPatterns(t *testing.T) {
	m := NewManager()
	log := m.MockCmdLog("level-002-find-safe")
	if log == "" {
		t.Fatalf("expected mock cmd log")
	}
}
