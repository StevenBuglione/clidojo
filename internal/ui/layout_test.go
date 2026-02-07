package ui

import "testing"

func TestDetermineLayoutMode(t *testing.T) {
	if got := DetermineLayoutMode(140, 30); got != LayoutWide {
		t.Fatalf("expected wide, got %v", got)
	}
	if got := DetermineLayoutMode(100, 30); got != LayoutMedium {
		t.Fatalf("expected medium, got %v", got)
	}
	if got := DetermineLayoutMode(80, 30); got != LayoutTooSmall {
		t.Fatalf("expected too-small, got %v", got)
	}
	if got := DetermineLayoutMode(100, 20); got != LayoutTooSmall {
		t.Fatalf("expected too-small by height, got %v", got)
	}
}
