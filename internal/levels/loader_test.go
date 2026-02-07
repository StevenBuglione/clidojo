package levels

import (
	"context"
	"path/filepath"
	"testing"
)

func TestBuiltinCorePackLoadsExpectedLevels(t *testing.T) {
	loader := NewLoader()
	packRoot := filepath.Join("..", "..", "packs")
	packs, err := loader.LoadPacks(context.Background(), packRoot)
	if err != nil {
		t.Fatalf("load packs: %v", err)
	}

	var builtin *Pack
	for i := range packs {
		if packs[i].PackID == "builtin-core" {
			builtin = &packs[i]
			break
		}
	}
	if builtin == nil {
		t.Fatalf("builtin-core pack not found")
	}
	if len(builtin.LoadedLevels) != 3 {
		t.Fatalf("expected 3 levels, got %d", len(builtin.LoadedLevels))
	}

	got := []string{builtin.LoadedLevels[0].LevelID, builtin.LoadedLevels[1].LevelID, builtin.LoadedLevels[2].LevelID}
	want := []string{"level-001-pipes-101", "level-002-find-safe", "level-003-top-ips"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("level order mismatch at %d: got %q want %q", i, got[i], want[i])
		}
	}
}
