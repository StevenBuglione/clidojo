package grading

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestGradeFileChecks(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "out.txt"), []byte("hello\nworld\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	g := NewGrader()
	start := time.Now().Add(-75 * time.Second)
	res, err := g.Grade(context.Background(), Request{
		AppVersion:           "0.1.0",
		PackID:               "builtin-core",
		PackVersion:          "0.1.0",
		LevelID:              "level-001-pipes-101",
		RunID:                "run-1",
		Attempt:              1,
		StartedAt:            start,
		FinishedAt:           time.Now(),
		Engine:               "mock",
		WorkDir:              dir,
		BasePoints:           1000,
		TimeGraceSeconds:     60,
		TimePenaltyPerSecond: 1,
		HintPenaltyPoints:    80,
		ResetPenaltyPoints:   120,
		Checks: []CheckSpec{
			{ID: "exists", Type: "file_exists", Required: true, Path: "/work/out.txt"},
			{ID: "count", Type: "file_lines_count", Required: true, Path: "/work/out.txt", Equals: 2},
			{ID: "regex", Type: "file_lines_match_regex", Required: true, Path: "/work/out.txt", Pattern: `^[a-z]+$`, Mode: "all_lines"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Passed {
		t.Fatalf("expected pass, got %#v", res)
	}
	if res.Score.TotalPoints != 985 {
		t.Fatalf("expected total 985, got %d", res.Score.TotalPoints)
	}
	if res.Kind != ResultKind || res.SchemaVersion != SchemaVersion {
		t.Fatalf("unexpected result metadata: kind=%s schema=%d", res.Kind, res.SchemaVersion)
	}
}

func TestGradeGeneratesDiffArtifact(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "out.txt"), []byte("actual\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	g := NewGrader()
	res, err := g.Grade(context.Background(), Request{
		PackID:      "p",
		PackVersion: "0.1.0",
		LevelID:     "l",
		RunID:       "r",
		Attempt:     1,
		StartedAt:   time.Now(),
		FinishedAt:  time.Now().Add(1 * time.Second),
		Engine:      "mock",
		WorkDir:     dir,
		Checks: []CheckSpec{
			{ID: "exact", Type: "file_text_exact", Required: true, Path: "/work/out.txt", Expected: "expected\n"},
		},
		BasePoints: 1000,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Passed {
		t.Fatalf("expected failed result")
	}
	if len(res.Artifacts) == 0 {
		t.Fatalf("expected diff artifact")
	}
}
