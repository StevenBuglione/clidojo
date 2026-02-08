package state

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestReviewQueueEnqueueAndCountDue(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "state.db")
	store, err := NewSQLite(dbPath)
	if err != nil {
		t.Fatalf("new sqlite: %v", err)
	}
	defer func() { _ = store.Close() }()

	ctx := context.Background()
	if err := store.EnsureSchema(ctx); err != nil {
		t.Fatalf("ensure schema: %v", err)
	}

	now := time.Date(2026, time.January, 1, 12, 0, 0, 0, time.UTC)
	if err := store.EnqueueReviewConcepts(ctx, "level-001", []string{"pipes", "sort"}, []int{1, 3, 7}, now); err != nil {
		t.Fatalf("enqueue reviews: %v", err)
	}
	// Duplicate enqueue should be ignored by UNIQUE + INSERT OR IGNORE.
	if err := store.EnqueueReviewConcepts(ctx, "level-001", []string{"pipes", "sort"}, []int{1, 3, 7}, now); err != nil {
		t.Fatalf("enqueue duplicate reviews: %v", err)
	}

	due1, err := store.CountDueReviews(ctx, now.AddDate(0, 0, 1))
	if err != nil {
		t.Fatalf("count due day1: %v", err)
	}
	if due1 != 2 {
		t.Fatalf("expected 2 due reviews after day 1, got %d", due1)
	}

	due7, err := store.CountDueReviews(ctx, now.AddDate(0, 0, 7))
	if err != nil {
		t.Fatalf("count due day7: %v", err)
	}
	if due7 != 6 {
		t.Fatalf("expected 6 due reviews after day 7, got %d", due7)
	}
}
