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

func TestDailyDrillUpsertAndGet(t *testing.T) {
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

	day := "2026-02-09"
	initial := DailyDrill{
		Day:            day,
		PlaylistJSON:   `[{"pack_id":"builtin-core","level_id":"level-001"},{"pack_id":"builtin-core","level_id":"level-002"}]`,
		CompletedCount: 1,
		UpdatedTS:      time.Date(2026, time.February, 9, 1, 0, 0, 0, time.UTC),
	}
	if err := store.UpsertDailyDrill(ctx, initial); err != nil {
		t.Fatalf("upsert initial drill: %v", err)
	}

	got, err := store.GetDailyDrill(ctx, day)
	if err != nil {
		t.Fatalf("get daily drill: %v", err)
	}
	if got == nil {
		t.Fatalf("expected drill row")
	}
	if got.Day != day {
		t.Fatalf("expected day %q, got %q", day, got.Day)
	}
	if got.CompletedCount != 1 {
		t.Fatalf("expected completed_count=1, got %d", got.CompletedCount)
	}

	// Lower completed count must not overwrite higher progress.
	if err := store.UpsertDailyDrill(ctx, DailyDrill{
		Day:            day,
		PlaylistJSON:   initial.PlaylistJSON,
		CompletedCount: 0,
		UpdatedTS:      time.Date(2026, time.February, 9, 2, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("upsert lower progress: %v", err)
	}

	got, err = store.GetDailyDrill(ctx, day)
	if err != nil {
		t.Fatalf("get daily drill after lower upsert: %v", err)
	}
	if got.CompletedCount != 1 {
		t.Fatalf("expected completed_count to remain 1, got %d", got.CompletedCount)
	}

	// Higher completed count should persist.
	if err := store.UpsertDailyDrill(ctx, DailyDrill{
		Day:            day,
		PlaylistJSON:   initial.PlaylistJSON,
		CompletedCount: 2,
		UpdatedTS:      time.Date(2026, time.February, 9, 3, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("upsert higher progress: %v", err)
	}
	got, err = store.GetDailyDrill(ctx, day)
	if err != nil {
		t.Fatalf("get daily drill after higher upsert: %v", err)
	}
	if got.CompletedCount != 2 {
		t.Fatalf("expected completed_count=2, got %d", got.CompletedCount)
	}
}
