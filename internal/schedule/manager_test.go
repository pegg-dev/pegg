package schedule

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/peggco/pegg/internal/core/config"
)

type fakeExecutor struct {
	mu    sync.Mutex
	calls []ExecuteRequest
}

func (f *fakeExecutor) Execute(_ context.Context, req ExecuteRequest) ExecuteResult {
	f.mu.Lock()
	f.calls = append(f.calls, req)
	f.mu.Unlock()
	return ExecuteResult{}
}

func (f *fakeExecutor) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

func newTestManager(t *testing.T, exec Executor) *Manager {
	t.Helper()
	cfg := config.DefaultConfig()
	cfg.Scheduler.StoreDir = t.TempDir()
	m, err := New(Deps{Config: cfg, Executor: exec})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return m
}

func TestManagerCRUD(t *testing.T) {
	m := newTestManager(t, &fakeExecutor{})

	s, err := m.Create(CreateOptions{
		Name:        "daily",
		Cron:        "0 9 * * *",
		Prompt:      "do work",
		Workspace:   t.TempDir(),
		Provider:    "p",
		Model:       "m",
		Tags:        []string{"ops"},
		MaxParallel: 1,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if s.ID == "" || s.NextRun == nil {
		t.Fatalf("unexpected schedule: %+v", s)
	}

	list := m.List()
	if len(list) != 1 {
		t.Fatalf("List len = %d", len(list))
	}

	name := "weekly"
	updated, err := m.Update(s.ID, UpdateOptions{Name: &name})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.Name != "weekly" {
		t.Fatalf("Update name = %q", updated.Name)
	}

	paused, err := m.Pause(s.ID)
	if err != nil {
		t.Fatalf("Pause: %v", err)
	}
	if paused.Enabled {
		t.Fatal("expected paused")
	}
	resumed, err := m.Resume(s.ID)
	if err != nil {
		t.Fatalf("Resume: %v", err)
	}
	if !resumed.Enabled {
		t.Fatal("expected resumed")
	}

	if err := m.Delete(s.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if len(m.List()) != 0 {
		t.Fatal("expected empty after delete")
	}
}

func TestManagerPersistsSchedules(t *testing.T) {
	cfg := config.DefaultConfig()
	dir := t.TempDir()
	cfg.Scheduler.StoreDir = dir

	m, err := New(Deps{Config: cfg, Executor: &fakeExecutor{}})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := m.Create(CreateOptions{Name: "n", Cron: "* * * * *", Prompt: "p", Provider: "p", Model: "m"}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	m2, err := New(Deps{Config: cfg, Executor: &fakeExecutor{}})
	if err != nil {
		t.Fatalf("New second: %v", err)
	}
	if err := m2.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer m2.Stop()
	if len(m2.List()) != 1 {
		t.Fatalf("expected persisted schedule, got %d", len(m2.List()))
	}
}

func TestManagerTriggerRecordsRun(t *testing.T) {
	exec := &fakeExecutor{}
	m := newTestManager(t, exec)

	s, err := m.Create(CreateOptions{
		Name:      "run-now",
		Cron:      "0 0 1 1 *",
		Prompt:    "do it",
		Provider:  "p",
		Model:     "m",
		Workspace: t.TempDir(),
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := m.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer m.Stop()

	if err := m.Trigger(s.ID); err != nil {
		t.Fatalf("Trigger: %v", err)
	}

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		st, _ := m.Stats(s.ID)
		if st.Total >= 1 {
			if st.Success != 1 {
				t.Fatalf("expected success, got %+v", st)
			}
			if exec.count() != 1 {
				t.Fatalf("executor calls = %d", exec.count())
			}
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("timed out waiting for run record")
}

func TestManagerGetFallsBackToStore(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Scheduler.StoreDir = t.TempDir()

	m1, err := New(Deps{Config: cfg, Executor: &fakeExecutor{}})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	s, err := m1.Create(CreateOptions{Name: "n", Cron: "* * * * *", Prompt: "p", Provider: "p", Model: "m"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	m2, err := New(Deps{Config: cfg, Executor: &fakeExecutor{}})
	if err != nil {
		t.Fatalf("New second: %v", err)
	}
	got, err := m2.Get(s.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.ID != s.ID || got.Prompt != "p" {
		t.Fatalf("unexpected schedule: %+v", got)
	}
}

func TestManagerDeleteRemovesRuns(t *testing.T) {
	m := newTestManager(t, &fakeExecutor{})
	s, err := m.Create(CreateOptions{Name: "n", Cron: "* * * * *", Prompt: "p", Provider: "p", Model: "m"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := m.BeginRun(RunRecord{RunID: "r1", ScheduleID: s.ID, Status: StatusSuccess, StartedAt: time.Now()}); err != nil {
		t.Fatalf("BeginRun: %v", err)
	}
	runs, _ := m.History(s.ID, 0)
	if len(runs) != 1 {
		t.Fatalf("expected 1 run, got %d", len(runs))
	}

	if err := m.Delete(s.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	runs, err = m.History(s.ID, 0)
	if err != nil {
		t.Fatalf("History: %v", err)
	}
	if len(runs) != 0 {
		t.Fatalf("expected run history deleted, got %d", len(runs))
	}
}

func TestManagerUpcomingSorted(t *testing.T) {
	m := newTestManager(t, &fakeExecutor{})
	_, err := m.Create(CreateOptions{Name: "later", Cron: "0 9 * * *", Prompt: "p", Provider: "p", Model: "m"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	list, err := m.Upcoming(5)
	if err != nil {
		t.Fatalf("Upcoming: %v", err)
	}
	if len(list) != 1 || list[0].NextRun == nil {
		t.Fatalf("unexpected upcoming: %+v", list)
	}
}
