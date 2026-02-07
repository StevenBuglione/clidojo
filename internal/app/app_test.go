package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"
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
