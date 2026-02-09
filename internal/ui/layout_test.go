package ui

import "testing"

func TestDetermineLayoutMode(t *testing.T) {
	if got := DetermineLayoutMode(140, 30); got != LayoutWide {
		t.Fatalf("expected wide, got %v", got)
	}
	if got := DetermineLayoutMode(100, 30); got != LayoutCompact {
		t.Fatalf("expected compact, got %v", got)
	}
	if got := DetermineLayoutMode(80, 30); got != LayoutCompact {
		t.Fatalf("expected compact at 80 columns, got %v", got)
	}
	if got := DetermineLayoutMode(120, 24); got != LayoutCompact {
		t.Fatalf("expected compact when rows are short, got %v", got)
	}
	if got := DetermineLayoutMode(80, 24); got != LayoutCompact {
		t.Fatalf("expected compact at 80x24, got %v", got)
	}
	if got := DetermineLayoutMode(79, 30); got != LayoutTooSmall {
		t.Fatalf("expected too-small, got %v", got)
	}
	if got := DetermineLayoutMode(100, 20); got != LayoutTooSmall {
		t.Fatalf("expected too-small by height, got %v", got)
	}
}
