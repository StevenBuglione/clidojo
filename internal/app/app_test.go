package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"clidojo/internal/levels"
)

type fakeHandle struct{ work string }

func (f fakeHandle) ShellCommand() []string     { return nil }
func (f fakeHandle) Stop(context.Context) error { return nil }
func (f fakeHandle) WorkDir() string            { return f.work }
func (f fakeHandle) ContainerName() string      { return "mock" }
func (f fakeHandle) Cwd() string                { return "" }
func (f fakeHandle) Env() []string              { return nil }
func (f fakeHandle) IsMock() bool               { return true }

func TestTagsForCommand(t *testing.T) {
	tags := tagsForCommand("find . -type f -print0 | xargs -0 sha1sum")
	if len(tags) < 3 {
		t.Fatalf("expected pipe/find/null-safe tags, got %#v", tags)
	}
}

func TestReadJournalEntriesParsesCmdLog(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".dojo_cmdlog"), []byte("1700000001\tls -la\n1700000002\tfind . -type f | wc -l\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	a := &App{handle: fakeHandle{work: dir}}
	entries := a.readJournalEntries()
	if len(entries) != 2 {
		t.Fatalf("expected 2 journal entries, got %d", len(entries))
	}
	if entries[1].Command != "find . -type f | wc -l" {
		t.Fatalf("unexpected command: %q", entries[1].Command)
	}
	if len(entries[1].Tags) == 0 {
		t.Fatalf("expected tags for second entry")
	}
}

func TestContainerNameSanitizesLevelID(t *testing.T) {
	name := containerName("1234567890", "level/with spaces")
	if name == "" {
		t.Fatalf("expected container name")
	}
	if name == "level/with spaces" {
		t.Fatalf("expected sanitization")
	}
}

func TestLevelAutoCheckConfigDefaultsAndOverride(t *testing.T) {
	a := &App{
		cfg: Config{
			Gameplay: GameplayConfig{
				AutoCheckDefault:    "command_and_fs_debounce",
				AutoCheckDebounceMS: 800,
			},
		},
		level: levels.Level{},
	}
	mode, debounce, quiet := a.levelAutoCheckConfig()
	if mode != "command_and_fs_debounce" {
		t.Fatalf("unexpected default mode: %q", mode)
	}
	if debounce != 800*time.Millisecond {
		t.Fatalf("unexpected default debounce: %v", debounce)
	}
	if !quiet {
		t.Fatalf("expected default quiet fail true")
	}

	q := false
	a.level.XAutoCheck = levels.AutoCheckExtension{
		Mode:       "command_debounce",
		DebounceMS: 250,
		QuietFail:  &q,
	}
	mode, debounce, quiet = a.levelAutoCheckConfig()
	if mode != "command_debounce" {
		t.Fatalf("unexpected override mode: %q", mode)
	}
	if debounce != 250*time.Millisecond {
		t.Fatalf("unexpected override debounce: %v", debounce)
	}
	if quiet {
		t.Fatalf("expected quiet fail override false")
	}
}

func TestWorkDirSignatureIgnoresDojoLogs(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".dojo_cmdlog"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".dojo_bash_history"), []byte("y"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "answer.txt"), []byte("ok"), 0o644); err != nil {
		t.Fatal(err)
	}
	a := &App{handle: fakeHandle{work: dir}}
	s1 := a.workDirSignature()
	if err := os.WriteFile(filepath.Join(dir, ".dojo_cmdlog"), []byte("changed"), 0o644); err != nil {
		t.Fatal(err)
	}
	s2 := a.workDirSignature()
	if s1 != s2 {
		t.Fatalf("expected cmdlog edits to be ignored in work signature")
	}
}

func TestEnqueueCheckCoalescesWhileRunning(t *testing.T) {
	a := &App{}
	a.checkRunning = true

	a.enqueueCheck(false, "auto")
	if !a.checkQueued {
		t.Fatalf("expected queued check when a run is in progress")
	}
	if a.queuedManual {
		t.Fatalf("did not expect manual flag from auto enqueue")
	}

	a.enqueueCheck(true, "manual")
	if !a.checkQueued {
		t.Fatalf("expected queued check to remain true")
	}
	if !a.queuedManual {
		t.Fatalf("expected manual intent to be preserved while queueing")
	}
}

func TestAutoCheckBlockedByOverlay(t *testing.T) {
	a := &App{}
	if a.autoCheckBlockedByOverlay() {
		t.Fatalf("expected no overlay to mean auto-check is not blocked")
	}
	a.menuOpen = true
	if !a.autoCheckBlockedByOverlay() {
		t.Fatalf("expected menu overlay to block auto-check")
	}
	a.menuOpen = false
	a.hintsOpen = true
	if !a.autoCheckBlockedByOverlay() {
		t.Fatalf("expected hints overlay to block auto-check")
	}
}

func TestApplyResultStreak(t *testing.T) {
	a := &App{}
	a.applyResultStreak(true, false)
	if a.passStreak != 1 {
		t.Fatalf("expected streak=1 after pass, got %d", a.passStreak)
	}
	a.applyResultStreak(true, false)
	if a.passStreak != 2 {
		t.Fatalf("expected streak=2 after second pass, got %d", a.passStreak)
	}
	a.applyResultStreak(false, false)
	if a.passStreak != 2 {
		t.Fatalf("optional-fail should not reset streak, got %d", a.passStreak)
	}
	a.applyResultStreak(false, true)
	if a.passStreak != 0 {
		t.Fatalf("required fail should reset streak, got %d", a.passStreak)
	}
}

func TestNextChallengeHint(t *testing.T) {
	a := &App{
		pack: levels.Pack{
			LoadedLevels: []levels.Level{
				{LevelID: "l1", Title: "Level One", Difficulty: 1, EstimatedMinutes: 5},
				{LevelID: "l2", Title: "Level Two", Difficulty: 2, EstimatedMinutes: 8},
			},
		},
		level: levels.Level{LevelID: "l1"},
	}
	hint := a.nextChallengeHint()
	if hint == "" {
		t.Fatalf("expected next challenge hint")
	}
	if hint != "Next challenge: Level Two (difficulty 2, ~8 min)." {
		t.Fatalf("unexpected hint: %q", hint)
	}
}

func TestResultSummaryIncludesNextChallengeOnPass(t *testing.T) {
	a := &App{
		pack: levels.Pack{
			LoadedLevels: []levels.Level{
				{LevelID: "l1", Title: "Level One", Difficulty: 1, EstimatedMinutes: 5},
				{LevelID: "l2", Title: "Level Two", Difficulty: 2, EstimatedMinutes: 8},
			},
		},
		level: levels.Level{LevelID: "l1"},
	}
	got := a.resultSummary(true)
	if got == "All required checks passed." {
		t.Fatalf("expected pass summary to include next challenge")
	}
	if gotFail := a.resultSummary(false); gotFail != "Some required checks failed." {
		t.Fatalf("unexpected fail summary: %q", gotFail)
	}
}
