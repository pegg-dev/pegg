package schedule

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/robfig/cron/v3"

	"github.com/peggco/pegg/internal/core/config"
	"github.com/peggco/pegg/internal/core/event"
	"github.com/peggco/pegg/internal/core/logger"
	"github.com/peggco/pegg/internal/llm"
	"github.com/peggco/pegg/internal/session"
	"github.com/peggco/pegg/internal/utils/query"
)

const reloadInterval = 5 * time.Second

type Deps struct {
	Config           *config.Config
	Bus              event.Bus
	Log              *logger.Logger
	Sessions         *session.Manager
	LLM              *llm.Manager
	Executor         Executor
	DefaultWorkspace string
}

type Manager struct {
	deps  Deps
	cfg   *config.SchedulerConfig
	store Store
	log   *logger.Logger
	bus   event.Bus
	exec  Executor
	cron  *cron.Cron

	mu        sync.Mutex
	schedules []Schedule
	entryIDs  map[string]cron.EntryID
	running   map[string]int
	lastMod   time.Time
	started   bool

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

func New(deps Deps) (*Manager, error) {
	cfg := deps.Config.Scheduler
	if cfg == nil {
		cfg = config.DefaultConfig().Scheduler
	}
	store, err := NewStore(cfg.StoreDir)
	if err != nil {
		return nil, err
	}
	exec := deps.Executor
	if exec == nil {
		exec = newSubprocessExecutor(deps.Log)
	}
	c := cron.New(
		cron.WithParser(parser),
		cron.WithChain(cron.Recover(cron.DiscardLogger)),
		cron.WithLogger(cron.DiscardLogger),
	)
	return &Manager{
		deps:     deps,
		cfg:      cfg,
		store:    store,
		log:      deps.Log,
		bus:      deps.Bus,
		exec:     exec,
		cron:     c,
		entryIDs: make(map[string]cron.EntryID),
		running:  make(map[string]int),
	}, nil
}

func (m *Manager) Store() Store { return m.store }

func (m *Manager) Enabled() bool { return m.cfg != nil && m.cfg.Enabled }

func (m *Manager) Start(ctx context.Context) error {
	if !m.Enabled() {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	m.mu.Lock()
	if m.started {
		m.mu.Unlock()
		return nil
	}
	m.ctx, m.cancel = context.WithCancel(ctx)
	m.started = true
	m.mu.Unlock()

	list, err := m.store.Load()
	if err != nil {
		return err
	}
	m.mu.Lock()
	m.schedules = list
	m.recomputeLocked()
	m.syncLocked()
	mod, _ := m.store.ModTime()
	m.lastMod = mod
	m.mu.Unlock()

	m.cron.Start()

	m.wg.Add(1)
	go m.reloadLoop()
	return nil
}

func (m *Manager) Stop() {
	m.mu.Lock()
	if !m.started {
		m.mu.Unlock()
		return
	}
	m.started = false
	cancel := m.cancel
	m.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	stopCtx := m.cron.Stop()
	select {
	case <-stopCtx.Done():
	case <-time.After(2 * time.Second):
	}
	m.wg.Wait()
}

func (m *Manager) reloadLoop() {
	defer m.wg.Done()
	ticker := time.NewTicker(reloadInterval)
	defer ticker.Stop()
	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			m.reloadIfChanged()
		}
	}
}

func (m *Manager) reloadIfChanged() {
	mod, err := m.store.ModTime()
	if err != nil {
		return
	}
	m.mu.Lock()
	changed := mod.After(m.lastMod)
	m.mu.Unlock()
	if !changed {
		return
	}
	list, err := m.store.Load()
	if err != nil {
		return
	}
	m.mu.Lock()
	m.schedules = list
	m.lastMod = mod
	m.dropEntriesLocked()
	m.recomputeLocked()
	m.syncLocked()
	m.mu.Unlock()
}

func (m *Manager) dropEntriesLocked() {
	for id, entry := range m.entryIDs {
		m.cron.Remove(entry)
		delete(m.entryIDs, id)
	}
}

func (m *Manager) List() []Schedule {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Schedule, len(m.schedules))
	copy(out, m.schedules)
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out
}

func (m *Manager) Get(id string) (Schedule, error) {
	m.mu.Lock()
	s, ok := m.findLocked(id)
	m.mu.Unlock()
	if ok {
		return s, nil
	}

	list, err := m.store.Load()
	if err != nil {
		return Schedule{}, fmt.Errorf("schedule: load store: %w", err)
	}
	for _, s := range list {
		if s.ID == id {
			return s, nil
		}
	}
	return Schedule{}, fmt.Errorf("schedule: %q not found", id)
}

func (m *Manager) Create(opts CreateOptions) (Schedule, error) {
	if strings.TrimSpace(opts.Name) == "" {
		return Schedule{}, fmt.Errorf("schedule: name is required")
	}
	if opts.Cron == "" {
		return Schedule{}, fmt.Errorf("schedule: cron is required")
	}
	if err := Validate(opts.Cron); err != nil {
		return Schedule{}, err
	}
	if strings.TrimSpace(opts.Prompt) == "" {
		return Schedule{}, fmt.Errorf("schedule: prompt is required")
	}
	workspace := opts.Workspace
	if workspace == "" {
		workspace = m.deps.DefaultWorkspace
	}
	if workspace == "" {
		workspace, _ = os.Getwd()
	}
	now := time.Now()
	s := Schedule{
		ID:          uuid.NewString(),
		Name:        opts.Name,
		Cron:        opts.Cron,
		Prompt:      opts.Prompt,
		Workspace:   workspace,
		Provider:    opts.Provider,
		Model:       opts.Model,
		Tags:        opts.Tags,
		TimeoutSecs: opts.TimeoutSecs,
		MaxParallel: opts.MaxParallel,
		Enabled:     !opts.Disabled,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if next, err := NextRun(s.Cron, now); err == nil {
		s.NextRun = next
	}

	m.mu.Lock()
	m.schedules = append(m.schedules, s)
	if err := m.persistLocked(); err != nil {
		m.schedules = m.schedules[:len(m.schedules)-1]
		m.mu.Unlock()
		return Schedule{}, err
	}
	m.syncLocked()
	mod, _ := m.store.ModTime()
	m.lastMod = mod
	m.mu.Unlock()

	m.publish(TopicScheduleCreated, s)
	return s, nil
}

func (m *Manager) Update(id string, opts UpdateOptions) (Schedule, error) {
	m.mu.Lock()
	idx := -1
	for i := range m.schedules {
		if m.schedules[i].ID == id {
			idx = i
			break
		}
	}
	if idx < 0 {
		m.mu.Unlock()
		return Schedule{}, fmt.Errorf("schedule: %q not found", id)
	}
	s := m.schedules[idx]
	if opts.Name != nil {
		s.Name = *opts.Name
	}
	if opts.Cron != nil {
		if err := Validate(*opts.Cron); err != nil {
			m.mu.Unlock()
			return Schedule{}, err
		}
		s.Cron = *opts.Cron
	}
	if opts.Prompt != nil {
		s.Prompt = *opts.Prompt
	}
	if opts.Workspace != nil {
		s.Workspace = *opts.Workspace
	}
	if opts.Provider != nil {
		s.Provider = *opts.Provider
	}
	if opts.Model != nil {
		s.Model = *opts.Model
	}
	if opts.Tags != nil {
		s.Tags = *opts.Tags
	}
	if opts.TimeoutSecs != nil {
		s.TimeoutSecs = *opts.TimeoutSecs
	}
	if opts.MaxParallel != nil {
		s.MaxParallel = *opts.MaxParallel
	}
	if opts.Enabled != nil {
		s.Enabled = *opts.Enabled
	}
	s.UpdatedAt = time.Now()
	if next, err := NextRun(s.Cron, s.UpdatedAt); err == nil {
		s.NextRun = next
	}
	prev := m.schedules[idx]
	m.schedules[idx] = s
	if opts.Cron != nil {
		if entry, ok := m.entryIDs[id]; ok {
			m.cron.Remove(entry)
			delete(m.entryIDs, id)
		}
	}
	if err := m.persistLocked(); err != nil {
		m.schedules[idx] = prev
		m.mu.Unlock()
		return Schedule{}, err
	}
	m.syncLocked()
	mod, _ := m.store.ModTime()
	m.lastMod = mod
	m.mu.Unlock()

	m.publish(TopicScheduleUpdated, s)
	return s, nil
}

func (m *Manager) Delete(id string) error {
	m.mu.Lock()
	idx := -1
	for i := range m.schedules {
		if m.schedules[i].ID == id {
			idx = i
			break
		}
	}
	if idx < 0 {
		m.mu.Unlock()
		return fmt.Errorf("schedule: %q not found", id)
	}
	removed := m.schedules[idx]
	m.schedules = append(m.schedules[:idx], m.schedules[idx+1:]...)
	if err := m.persistLocked(); err != nil {
		m.schedules = append(m.schedules, removed)
		m.mu.Unlock()
		return err
	}
	if entry, ok := m.entryIDs[id]; ok {
		m.cron.Remove(entry)
		delete(m.entryIDs, id)
	}
	delete(m.running, id)
	mod, _ := m.store.ModTime()
	m.lastMod = mod
	m.mu.Unlock()

	if err := m.store.DeleteRuns(id); err != nil && m.log != nil {
		m.log.Fwarn("schedule: delete run history for %q: %v", id, err)
	}

	m.publish(TopicScheduleDeleted, removed)
	return nil
}

func (m *Manager) Pause(id string) (Schedule, error) {
	disabled := false
	return m.Update(id, UpdateOptions{Enabled: &disabled})
}

func (m *Manager) Resume(id string) (Schedule, error) {
	enabled := true
	return m.Update(id, UpdateOptions{Enabled: &enabled})
}

func (m *Manager) Trigger(id string) error {
	if _, err := m.Get(id); err != nil {
		return err
	}
	go m.fire(id, "manual")
	return nil
}

func (m *Manager) Upcoming(limit int) ([]Schedule, error) {
	if limit <= 0 {
		limit = 10
	}
	now := time.Now()
	out := make([]Schedule, 0, limit)
	m.mu.Lock()
	for _, s := range m.schedules {
		if !s.Enabled {
			continue
		}
		next, err := NextRun(s.Cron, now)
		if err != nil {
			continue
		}
		s.NextRun = next
		out = append(out, s)
	}
	m.mu.Unlock()
	sort.Slice(out, func(i, j int) bool {
		if out[i].NextRun == nil || out[j].NextRun == nil {
			return out[i].NextRun == nil
		}
		return out[i].NextRun.Before(*out[j].NextRun)
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (m *Manager) History(scheduleID string, limit int) ([]RunRecord, error) {
	runs, err := m.store.ListRuns(scheduleID)
	if err != nil {
		return nil, err
	}
	if limit > 0 && len(runs) > limit {
		runs = runs[:limit]
	}
	return runs, nil
}

func (m *Manager) Active() ([]RunRecord, error) {
	m.mu.Lock()
	ids := make([]string, 0, len(m.schedules))
	for _, s := range m.schedules {
		ids = append(ids, s.ID)
	}
	m.mu.Unlock()
	var active []RunRecord
	for _, id := range ids {
		runs, err := m.store.ListRuns(id)
		if err != nil {
			continue
		}
		for _, r := range runs {
			if r.Status == StatusRunning {
				active = append(active, r)
			}
		}
	}
	sort.Slice(active, func(i, j int) bool { return active[i].StartedAt.After(active[j].StartedAt) })
	return active, nil
}

func (m *Manager) Stats(scheduleID string) (Stats, error) {
	runs, err := m.store.ListRuns(scheduleID)
	if err != nil {
		return Stats{}, err
	}
	var st Stats
	var totalDur int64
	for _, r := range runs {
		st.Total++
		switch r.Status {
		case StatusSuccess:
			st.Success++
			totalDur += r.DurationMs
		case StatusRunning:
			st.Running++
		default:
			st.Failed++
			if st.LastFailure == nil || r.StartedAt.After(*st.LastFailure) {
				t := r.StartedAt
				st.LastFailure = &t
			}
		}
		if st.LastRun == nil || r.StartedAt.After(*st.LastRun) {
			t := r.StartedAt
			st.LastRun = &t
		}
	}
	finished := st.Success + st.Failed
	if finished > 0 {
		st.SuccessRate = float64(st.Success) / float64(finished)
		st.AvgDurationMs = totalDur / int64(finished)
	}
	return st, nil
}

func (m *Manager) BeginRun(rec RunRecord) error {
	if rec.Status == "" {
		rec.Status = StatusRunning
	}
	if rec.StartedAt.IsZero() {
		rec.StartedAt = time.Now()
	}
	return m.store.AppendRun(rec)
}

func (m *Manager) FinishRun(rec RunRecord) error {
	now := time.Now()
	if rec.FinishedAt == nil {
		rec.FinishedAt = &now
	}
	if rec.DurationMs == 0 && !rec.StartedAt.IsZero() {
		rec.DurationMs = now.Sub(rec.StartedAt).Milliseconds()
	}
	if err := m.store.AppendRun(rec); err != nil {
		return err
	}
	m.mu.Lock()
	if history := m.cfg.HistoryLimit; history > 0 {
		go m.store.PruneRuns(rec.ScheduleID, history)
	}
	m.mu.Unlock()
	return nil
}

func (m *Manager) fire(scheduleID, trigger string) {
	m.mu.Lock()
	s, ok := m.findLocked(scheduleID)
	if !ok || !s.Enabled {
		m.mu.Unlock()
		return
	}
	if m.running[scheduleID] >= s.MaxParallelValue() {
		m.mu.Unlock()
		if m.log != nil {
			m.log.Fdebug("schedule: %q already running, skipping fire", s.Name)
		}
		return
	}
	m.running[scheduleID]++
	m.mu.Unlock()

	defer func() {
		m.mu.Lock()
		if m.running[scheduleID] > 0 {
			m.running[scheduleID]--
		}
		m.mu.Unlock()
	}()

	m.runOnce(s, trigger)
}

func (m *Manager) runOnce(s Schedule, trigger string) {
	runID := uuid.NewString()
	startedAt := time.Now()
	rec := RunRecord{
		RunID:      runID,
		ScheduleID: s.ID,
		Trigger:    trigger,
		Status:     StatusRunning,
		StartedAt:  startedAt,
	}

	provider, model, err := m.resolveModel(s)
	if err != nil {
		rec.Status = StatusFailed
		rec.Error = "resolve model: " + err.Error()
		_ = m.FinishRun(rec)
		m.publishRun(TopicScheduleFailed, s, rec)
		return
	}
	rec.Provider = provider
	rec.Model = model

	sessionID := ""
	if m.deps.Sessions != nil {
		created, cerr := m.deps.Sessions.Create(session.CreateOptions{
			Title:      "schedule " + startedAt.Format("15.04.05"),
			Provider:   provider,
			Model:      model,
			ProjectDir: s.Workspace,
		})
		if cerr != nil {
			rec.Status = StatusFailed
			rec.Error = "create session: " + cerr.Error()
			_ = m.FinishRun(rec)
			m.publishRun(TopicScheduleFailed, s, rec)
			return
		}
		sessionID = created.ID
		rec.SessionID = sessionID
	}

	if err := m.BeginRun(rec); err != nil && m.log != nil {
		m.log.Fwarn("schedule: record run start: %v", err)
	}
	m.publishRun(TopicScheduleStarted, s, rec)

	ctx := context.Background()
	if timeout := s.TimeoutValue(); timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	result := m.exec.Execute(ctx, ExecuteRequest{
		Schedule:  s,
		RunID:     runID,
		SessionID: sessionID,
	})

	if existing, lerr := m.store.LoadRun(s.ID, runID); lerr == nil && existing.Status != StatusRunning {
		rec = existing
	} else {
		base := rec
		base.FinishedAt = timePtr(time.Now())
		base.DurationMs = time.Since(startedAt).Milliseconds()
		base.ExitCode = result.ExitCode
		base.Usage = result.Usage
		switch {
		case ctx.Err() == context.DeadlineExceeded:
			base.Status = StatusTimeout
			base.Error = "scheduled run timed out"
		case result.ExitCode != 0:
			base.Status = StatusFailed
			base.Error = strings.TrimSpace(result.Output)
			if base.Error == "" {
				base.Error = fmt.Sprintf("scheduled run exited with code %d", result.ExitCode)
			}
		default:
			base.Status = StatusSuccess
		}
		rec = base
		_ = m.FinishRun(rec)
	}

	m.mu.Lock()
	for i := range m.schedules {
		if m.schedules[i].ID == s.ID {
			t := startedAt
			m.schedules[i].LastRun = &t
			if next, nerr := NextRun(m.schedules[i].Cron, time.Now()); nerr == nil {
				m.schedules[i].NextRun = next
			}
			break
		}
	}
	_ = m.persistLocked()
	mod, _ := m.store.ModTime()
	m.lastMod = mod
	m.mu.Unlock()

	if rec.Status == StatusSuccess {
		m.publishRun(TopicScheduleFinished, s, rec)
	} else {
		m.publishRun(TopicScheduleFailed, s, rec)
	}
}

func (m *Manager) resolveModel(s Schedule) (string, string, error) {
	if m.deps.LLM == nil {
		if s.Provider == "" && s.Model == "" {
			return "", "", fmt.Errorf("no model configured")
		}
		return s.Provider, s.Model, nil
	}
	m.deps.LLM.WaitUntilReady()

	if s.Provider == "" && s.Model == "" {
		if p, model, ok := m.projectModel(s.Workspace); ok {
			return p, model, nil
		}
	}

	mode := llm.SelectModePreferred
	if s.Model != "" {
		mode = llm.SelectModeExact
	}
	res := m.deps.LLM.Select(llm.SelectRequest{Mode: mode, Provider: s.Provider, Model: s.Model})
	if res.Err != nil {
		return "", "", res.Err
	}
	return res.Provider, res.Model.ID, nil
}

func (m *Manager) projectModel(dir string) (string, string, bool) {
	if m.deps.Sessions == nil {
		return "", "", false
	}
	q := query.Query{
		Page: query.Page{Number: 1, Size: 20},
		Sort: []query.Sort{{Column: "updated_at", Dir: query.Desc}},
		Filters: []query.Filter{
			{Column: "compaction_parent_id", Operator: query.OpEqual, Value: ""},
		},
	}
	if dir != "" {
		q.Filters = append(q.Filters, query.Filter{Column: "project_dir", Operator: query.OpEqual, Value: dir})
	}
	sessions, _, err := m.deps.Sessions.List(q)
	if err != nil {
		return "", "", false
	}
	for _, s := range sessions {
		if s.Provider == "" || s.Model == "" {
			continue
		}
		return s.Provider, s.Model, true
	}
	return "", "", false
}

func (m *Manager) findLocked(id string) (Schedule, bool) {
	for _, s := range m.schedules {
		if s.ID == id {
			return s, true
		}
	}
	return Schedule{}, false
}

func (m *Manager) recomputeLocked() {
	now := time.Now()
	for i := range m.schedules {
		if !m.schedules[i].Enabled {
			m.schedules[i].NextRun = nil
			continue
		}
		if next, err := NextRun(m.schedules[i].Cron, now); err == nil {
			m.schedules[i].NextRun = next
		}
	}
}

func (m *Manager) syncLocked() {
	seen := make(map[string]bool, len(m.schedules))
	for _, s := range m.schedules {
		seen[s.ID] = true
		if !s.Enabled {
			if entry, ok := m.entryIDs[s.ID]; ok {
				m.cron.Remove(entry)
				delete(m.entryIDs, s.ID)
			}
			continue
		}
		if _, ok := m.entryIDs[s.ID]; ok {
			continue
		}
		id := s.ID
		entry, err := m.cron.AddFunc(s.Cron, func() { m.fire(id, "cron") })
		if err != nil {
			if m.log != nil {
				m.log.Fwarn("schedule: add cron entry %q: %v", s.Name, err)
			}
			continue
		}
		m.entryIDs[s.ID] = entry
	}
	for id, entry := range m.entryIDs {
		if !seen[id] {
			m.cron.Remove(entry)
			delete(m.entryIDs, id)
		}
	}
}

func (m *Manager) persistLocked() error {
	return m.store.Save(m.schedules)
}

func (m *Manager) publish(topic string, s Schedule) {
	if m.bus == nil {
		return
	}
	m.bus.Publish(topic, ScheduleEvent{ScheduleID: s.ID, Name: s.Name, Workspace: s.Workspace})
}

func (m *Manager) publishRun(topic string, s Schedule, rec RunRecord) {
	if m.bus == nil {
		return
	}
	var dur time.Duration
	if rec.DurationMs > 0 {
		dur = time.Duration(rec.DurationMs) * time.Millisecond
	}
	m.bus.Publish(topic, RunEvent{
		RunID:      rec.RunID,
		ScheduleID: rec.ScheduleID,
		Name:       s.Name,
		SessionID:  rec.SessionID,
		Status:     rec.Status,
		Error:      rec.Error,
		Duration:   dur,
	})
}

func timePtr(t time.Time) *time.Time { return &t }
