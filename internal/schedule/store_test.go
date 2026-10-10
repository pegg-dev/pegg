package schedule

import (
	"testing"
	"time"
)

func TestStoreScheduleRoundTrip(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	list, err := store.Load()
	if err != nil {
		t.Fatalf("Load empty: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("expected no schedules, got %d", len(list))
	}

	in := []Schedule{{
		ID:        "abc",
		Name:      "daily",
		Cron:      "0 9 * * *",
		Prompt:    "do work",
		Workspace: "/tmp/proj",
		Enabled:   true,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}}
	if err := store.Save(in); err != nil {
		t.Fatalf("Save: %v", err)
	}
	out, err := store.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(out) != 1 || out[0].ID != "abc" || out[0].Name != "daily" {
		t.Fatalf("unexpected round trip: %+v", out)
	}
}

func TestStoreRuns(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	base := time.Now()
	for i := 0; i < 3; i++ {
		rec := RunRecord{
			RunID:      string(rune('a' + i)),
			ScheduleID: "s1",
			Status:     StatusSuccess,
			StartedAt:  base.Add(time.Duration(i) * time.Minute),
		}
		if err := store.AppendRun(rec); err != nil {
			t.Fatalf("AppendRun: %v", err)
		}
	}

	runs, err := store.ListRuns("s1")
	if err != nil {
		t.Fatalf("ListRuns: %v", err)
	}
	if len(runs) != 3 {
		t.Fatalf("expected 3 runs, got %d", len(runs))
	}
	if !runs[0].StartedAt.After(runs[1].StartedAt) {
		t.Fatalf("expected runs sorted newest first")
	}

	got, err := store.LoadRun("s1", "a")
	if err != nil {
		t.Fatalf("LoadRun: %v", err)
	}
	if got.RunID != "a" {
		t.Fatalf("LoadRun = %+v", got)
	}

	if err := store.PruneRuns("s1", 1); err != nil {
		t.Fatalf("PruneRuns: %v", err)
	}
	runs, _ = store.ListRuns("s1")
	if len(runs) != 1 {
		t.Fatalf("expected 1 run after prune, got %d", len(runs))
	}
}
