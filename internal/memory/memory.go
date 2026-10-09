package memory

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/peggco/pegg/internal/agent"
	"github.com/peggco/pegg/internal/agent/prompt"
	"github.com/peggco/pegg/internal/core/config"
	"github.com/peggco/pegg/internal/core/event"
	"github.com/peggco/pegg/internal/core/logger"
	decisionapi "github.com/peggco/pegg/internal/decision"
	"github.com/peggco/pegg/internal/llm"
	"github.com/peggco/pegg/internal/session"
)

type Deps struct {
	Config   *config.MemoryConfig
	LLM      *llm.Manager
	Decision *decisionapi.Manager
	Bus      event.Bus
	Log      *logger.Logger
}

type Manager struct {
	deps  Deps
	cfg   *config.MemoryConfig
	store *Store
	jobs  chan func()
	stop  chan struct{}
	wg    sync.WaitGroup

	mu       sync.Mutex
	turns    map[string]*turn
	callArgs map[string]string
	sessions map[string]string

	observerBusy atomic.Bool
	started      atomic.Bool
}

var alwaysSkipTools = map[string]bool{
	"mem-search":      true,
	"mem-read":        true,
	"askuserquestion": true,
	"loadskill":       true,
}

func New(deps Deps) *Manager {
	return newManager(deps, projectDir())
}

func newManager(deps Deps, dir string) *Manager {
	if deps.Config == nil {
		deps.Config = defaultCfg()
	}
	store, err := NewStore(dir)
	if err != nil && deps.Log != nil {
		deps.Log.Fwarn("memory: store init failed: %v", err)
	}
	m := &Manager{
		deps:     deps,
		cfg:      deps.Config,
		store:    store,
		jobs:     make(chan func(), queueSize),
		stop:     make(chan struct{}),
		turns:    make(map[string]*turn),
		callArgs: make(map[string]string),
		sessions: make(map[string]string),
	}
	return m
}

func projectDir() string {
	dir, err := os.Getwd()
	if err != nil {
		return "."
	}
	return dir
}

func (m *Manager) Start() error {
	if m.deps.Bus == nil || m.started.Swap(true) {
		return nil
	}
	subs := []struct {
		topic string
		fn    any
	}{
		{agent.TopicAgentStarted, m.handleStarted},
		{agent.TopicAgentInput, m.handleInput},
		{agent.TopicAgentToolCall, m.handleToolCall},
		{agent.TopicAgentToolResult, m.handleToolResult},
		{agent.TopicAgentMessage, m.handleMessage},
		{agent.TopicAgentFinished, m.handleFinished},
		{agent.TopicAgentError, m.handleError},
		{session.TopicSessionAttached, m.handleSessionAttached},
	}
	for _, s := range subs {
		if err := m.deps.Bus.Subscribe(s.topic, s.fn); err != nil {
			return fmt.Errorf("memory: subscribe %s: %w", s.topic, err)
		}
	}
	agent.OnMessageInput(m.expandInput)
	m.wg.Add(1)
	go m.runWorker()
	return nil
}

func (m *Manager) Stop() {
	if m.deps.Bus != nil {
		_ = m.deps.Bus.Unsubscribe(agent.TopicAgentStarted, m.handleStarted)
		_ = m.deps.Bus.Unsubscribe(agent.TopicAgentInput, m.handleInput)
		_ = m.deps.Bus.Unsubscribe(agent.TopicAgentToolCall, m.handleToolCall)
		_ = m.deps.Bus.Unsubscribe(agent.TopicAgentToolResult, m.handleToolResult)
		_ = m.deps.Bus.Unsubscribe(agent.TopicAgentMessage, m.handleMessage)
		_ = m.deps.Bus.Unsubscribe(agent.TopicAgentFinished, m.handleFinished)
		_ = m.deps.Bus.Unsubscribe(agent.TopicAgentError, m.handleError)
		_ = m.deps.Bus.Unsubscribe(session.TopicSessionAttached, m.handleSessionAttached)
	}
	close(m.stop)
	m.wg.Wait()
}

func (m *Manager) UpdateConfig(cfg *config.MemoryConfig) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if cfg == nil {
		cfg = defaultCfg()
	}
	m.cfg = cfg
}

func (m *Manager) enabled() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.cfg != nil && m.cfg.Enabled
}

func (m *Manager) cfgSnapshot() *config.MemoryConfig {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.cfg == nil {
		return defaultCfg()
	}
	return m.cfg
}

func (m *Manager) logf(format string, args ...any) {
	if m.deps.Log != nil {
		m.deps.Log.Fdebug("memory: "+format, args...)
	}
}

func (m *Manager) handleStarted(e agent.AgentStarted) {
	if !m.enabled() {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.turns[e.AgentID] = &turn{
		agentID:     e.AgentID,
		agentName:   e.AgentName,
		displayName: e.DisplayName,
		parentID:    e.ParentAgentID,
		startedAt:   timeNow(),
		sessionID:   m.sessions[e.AgentID],
	}
}

func (m *Manager) handleInput(e agent.AgentInput) {
	if !m.enabled() {
		return
	}
	m.mu.Lock()
	t, ok := m.turns[e.AgentID]
	if ok && t.prompt == "" {
		t.prompt = clampTo(e.Input, maxPromptChars)
	}
	m.mu.Unlock()
}

func (m *Manager) handleToolCall(e agent.AgentToolCall) {
	m.mu.Lock()
	m.callArgs[e.AgentID+"/"+e.Call.ID] = clampTo(e.Call.Function.Arguments, maxToolInChars)
	m.mu.Unlock()
}

func (m *Manager) handleToolResult(e agent.AgentToolResult) {
	if !m.enabled() {
		return
	}
	cfg := m.cfgSnapshot()
	if cfgSkipsTool(cfg, e.ToolName) || alwaysSkipTools[e.ToolName] {
		return
	}
	m.mu.Lock()
	t, ok := m.turns[e.AgentID]
	if !ok {
		m.turns[e.AgentID] = &turn{agentID: e.AgentID, agentName: e.AgentName, sessionID: m.sessions[e.AgentID], startedAt: timeNow()}
		t = m.turns[e.AgentID]
	}
	args := m.callArgs[e.AgentID+"/"+e.CallID]
	delete(m.callArgs, e.AgentID+"/"+e.CallID)
	errText := ""
	if e.Err != nil {
		errText = e.Err.Error()
	}
	t.toolUses = append(t.toolUses, ToolUse{
		Name:    e.ToolName,
		Args:    args,
		Output:  clampTo(e.Output, maxToolOutChars),
		Err:     errText,
		At:      timeNow(),
		Session: t.sessionID,
		Agent:   e.AgentName,
	})
	m.mu.Unlock()
}

func (m *Manager) handleMessage(e agent.AgentMessage) {
	m.mu.Lock()
	if t, ok := m.turns[e.AgentID]; ok {
		if text := llm.MessageText(e.Message); text != "" {
			t.lastAssistant = clampTo(text, 3000)
		}
	}
	m.mu.Unlock()
}

func (m *Manager) handleSessionAttached(e session.SessionAttached) {
	m.mu.Lock()
	m.sessions[e.AgentID] = e.SessionID
	if t, ok := m.turns[e.AgentID]; ok && t.sessionID == "" {
		t.sessionID = e.SessionID
	}
	m.mu.Unlock()
}

func (m *Manager) handleFinished(e agent.AgentFinished) {
	t := m.takeTurn(e.AgentID)
	if t == nil {
		return
	}
	m.enqueue(func() { m.flush(t, true) })
}

func (m *Manager) handleError(e agent.AgentError) {
	t := m.takeTurn(e.AgentID)
	if t == nil {
		return
	}
	m.enqueue(func() { m.flush(t, false) })
}

func (m *Manager) takeTurn(agentID string) *turn {
	m.mu.Lock()
	defer m.mu.Unlock()
	t, ok := m.turns[agentID]
	if !ok {
		return nil
	}
	delete(m.turns, agentID)
	return t
}

func (m *Manager) enqueue(job func()) {
	select {
	case m.jobs <- job:
	default:
		m.logf("queue full, dropping memory event")
	}
}

func (m *Manager) runWorker() {
	defer m.wg.Done()
	for {
		select {
		case <-m.stop:
			return
		case job := <-m.jobs:
			m.observerBusy.Store(true)
			job()
			m.observerBusy.Store(false)
		}
	}
}

func (m *Manager) flush(t *turn, withSummary bool) {
	if m.store == nil || !m.enabled() {
		return
	}
	if len(t.toolUses) == 0 && t.lastAssistant == "" {
		return
	}
	prov, model, err := m.observerModel()
	if err != nil {
		m.logf("observer model unavailable: %v", err)
		return
	}
	ctx := context.Background()

	var stored int
	var newBlocks []string
	if len(t.toolUses) > 0 {
		blocks, entries := m.observe(ctx, prov, model, t)
		if len(entries) > 0 {
			n, err := m.store.AppendEntries(blocks, entries)
			if err != nil {
				m.logf("append observations: %v", err)
			} else {
				stored += n
				newBlocks = append(newBlocks, blocks...)
			}
		}
	}

	if withSummary && (len(t.toolUses) > 0 || t.prompt != "") {
		if sum, ok := m.summarize(ctx, prov, model, t); ok {
			block := summaryBlock(sum)
			entry := IndexEntry{
				ID:    sum.ID,
				Date:  sum.At.Format("2006-01-02T15:04"),
				Type:  "summary",
				Title: "Session summary",
				File:  m.store.dailySessionFile(),
			}
			n, err := m.store.AppendEntries([]string{block}, []IndexEntry{entry})
			if err != nil {
				m.logf("append summary: %v", err)
			} else {
				stored += n
				newBlocks = append(newBlocks, block)
			}
		}
	}

	cfg := m.cfgSnapshot()
	if stored > 0 && cfg.ConsolidateValue() {
		m.consolidate(ctx, prov, model, t, newBlocks)
	}
	m.logf("flush: stored %d entries for %s", stored, t.agentName)
}

func (m *Manager) observe(ctx context.Context, prov llm.Provider, model llm.Model, t *turn) ([]string, []IndexEntry) {
	uses := make([]observedUse, 0, len(t.toolUses))
	for _, u := range t.toolUses {
		outcome := u.Output
		if u.Err != "" {
			outcome = "Error: " + u.Err + "\n" + outcome
		}
		uses = append(uses, observedUse{
			ToolName:   u.Name,
			At:         u.At,
			WorkingDir: projectDir(),
			Params:     u.Args,
			Outcome:    outcome,
		})
	}
	input, err := buildObservationInput(uses)
	if err != nil {
		m.logf("build observation input: %v", err)
		return nil, nil
	}
	out, err := runObserver(ctx, prov, model, input)
	if err != nil {
		m.logf("observer failed: %v", err)
		return nil, nil
	}
	parsed, ok := parseObservations(out)
	if !ok {
		return nil, nil
	}

	var blocks []string
	var entries []IndexEntry
	for i := range parsed {
		obs := &parsed[i]
		for _, u := range t.toolUses {
			read, mod := fileEvidence(u.Name, u.Args)
			obs.FilesRead = appendUnique(obs.FilesRead, read...)
			obs.FilesMod = appendUnique(obs.FilesMod, mod...)
		}
		obs.At = t.startedAt
		obs.Session = shortID(t.sessionID)
		obs.Agent = t.agentName
		obs.Hash = hashOf(t.sessionID, obs.Title, obs.Narrative)
		obs.ID = obsPrefix + obs.Hash[:6]
		blocks = append(blocks, observationBlock(*obs))
		entries = append(entries, IndexEntry{
			ID:       obs.ID,
			Date:     obs.At.Format("2006-01-02T15:04"),
			Type:     obs.Type,
			Title:    obs.Title,
			Keywords: strings.Join(obs.Concepts, ", "),
			File:     m.store.dailySessionFile(),
		})
	}
	return blocks, entries
}

func (m *Manager) summarize(ctx context.Context, prov llm.Provider, model llm.Model, t *turn) (Summary, bool) {
	out, err := runSummarizer(ctx, prov, model, buildSummaryInput(t))
	if err != nil {
		m.logf("summarizer failed: %v", err)
		return Summary{}, false
	}
	sum, ok := parseSummary(out)
	if !ok {
		return Summary{}, false
	}
	sum.At = timeNow()
	sum.Session = shortID(t.sessionID)
	sum.Agent = t.agentName
	sum.Hash = hashOf(t.sessionID, "summary", sum.Learned, sum.Completed)
	sum.ID = sumPrefix + sum.Hash[:6]
	return sum, true
}

func (m *Manager) consolidate(ctx context.Context, prov llm.Provider, model llm.Model, t *turn, newBlocks []string) {
	active := m.store.ReadCurated(activeCtxFile)
	decisions := m.store.ReadCurated(decisionsFile)
	lessons := m.store.ReadCurated(lessonsFile)
	notes := m.store.ReadCurated(notesFile)
	input, err := buildLibrarianInput(newBlocks, active, decisions, lessons, notes)
	if err != nil {
		return
	}
	out, err := runLibrarian(ctx, prov, model, input)
	if err != nil {
		m.logf("librarian failed: %v", err)
		return
	}
	upd, ok := parseMemoryUpdate(out)
	if !ok {
		m.logf("librarian returned unparsable output")
		return
	}
	if upd.ActiveContext != "" {
		_ = m.store.WriteCurated(activeCtxFile, upd.ActiveContext)
	}
	if upd.Decisions != "" {
		_ = m.store.WriteCurated(decisionsFile, upd.Decisions)
	}
	if upd.Lessons != "" {
		_ = m.store.WriteCurated(lessonsFile, upd.Lessons)
		m.promoteGlobalLessons(upd.Lessons)
	}
	if upd.Notes != "" {
		_ = m.store.WriteCurated(notesFile, upd.Notes)
	}
}

func (m *Manager) promoteGlobalLessons(content string) {
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "- "+globalMarker) {
			continue
		}
		lesson := strings.TrimSpace(strings.TrimPrefix(line, "- "+globalMarker))
		lesson = "- " + clampTo(lesson, maxGlobalLesson)
		if err := m.store.AppendGlobalLessons(lesson); err != nil {
			m.logf("append global lesson: %v", err)
		}
	}
}

func (m *Manager) expandInput(input agent.MessageInput) agent.MessageInput {
	if m.observerBusy.Load() || !m.enabled() || m.store == nil {
		return input
	}
	panel := m.BuildPanel()
	if panel == "" {
		return input
	}
	input.SystemReminders = append(input.SystemReminders, panel)
	return input
}

func (m *Manager) BuildPanel() string {
	if m.store == nil || !m.enabled() || !m.store.HasMemory() {
		return ""
	}
	cfg := m.cfgSnapshot()
	budget := cfg.BudgetValue()
	if budget <= 0 {
		budget = config.DefaultMemoryBudget
	}

	var b strings.Builder
	b.WriteString("# Project memory\n")
	remaining := budget - b.Len()
	if remaining <= 0 {
		return ""
	}

	type section struct{ name, label string }
	secs := []section{
		{activeCtxFile, "Active context"},
		{decisionsFile, "Decisions"},
		{lessonsFile, "Lessons"},
		{notesFile, "Notes"},
	}
	for _, s := range secs {
		if remaining < 40 {
			break
		}
		content := strings.TrimSpace(m.store.ReadCurated(s.name))
		if content == "" {
			continue
		}
		content = clampTo(content, maxCuratedChars)
		block := "## " + s.label + "\n" + content + "\n"
		if len(block) > remaining {
			continue
		}
		b.WriteString(block)
		remaining -= len(block)
	}

	if remaining >= 40 {
		entries, _ := m.store.ReadIndex()
		for i := len(entries) - 1; i >= 0 && remaining > 60; i-- {
			e := entries[i]
			line := "- [" + e.Type + "] " + e.Title
			if e.ID != "" {
				line += " (" + e.ID + ")"
			}
			if e.Date != "" {
				line += " · " + e.Date
			}
			if e.Keywords != "" {
				line += " — " + clampTo(e.Keywords, 60)
			}
			line += "\n"
			if len(line) > remaining {
				break
			}
			b.WriteString(line)
			remaining -= len(line)
		}
	}

	if b.Len() <= len("# Project memory\n") {
		return ""
	}
	return strings.TrimRight(b.String(), "\n")
}

func (m *Manager) Store() *Store { return m.store }

func (m *Manager) gateHits(ctx context.Context, query string, hits []SearchHit) []SearchHit {
	if len(hits) == 0 {
		return hits
	}
	switch m.cfgSnapshot().GateValue() {
	case config.GateOff:
		return hits
	case config.GateDecision:
		if prov, model, ok := m.decider(); ok {
			return m.gateDecision(ctx, prov, model, query, hits)
		}
		return hits
	case config.GateLLM:
		return m.gateLLM(ctx, query, hits)
	case config.GateScore:
		return gateByScore(hits)
	default:
		if prov, model, ok := m.decider(); ok {
			return m.gateDecision(ctx, prov, model, query, hits)
		}
		return gateByScore(hits)
	}
}

func (m *Manager) gateDecision(ctx context.Context, prov decisionapi.Provider, model, query string, hits []SearchHit) []SearchHit {
	var sb strings.Builder
	sb.WriteString("Query: ")
	sb.WriteString(query)
	sb.WriteString("\n\nCandidate memories:\n")
	for i, h := range hits {
		fmt.Fprintf(&sb, "%d. %s\n", i+1, h.Title)
	}
	questions := make(map[string]decisionapi.Question, len(hits))
	for i, h := range hits {
		questions[fmt.Sprintf("rel_%d", i)] = decisionapi.BoolQuestion(
			fmt.Sprintf("Is memory %q relevant to the query?", h.Title),
			map[string]string{
				"true":  "Relevant and useful for the current question",
				"false": "Unrelated or noise",
			},
		)
	}
	res, err := prov.Decide(ctx, &decisionapi.Request{
		Model:     model,
		State:     sb.String(),
		Questions: questions,
	})
	if err != nil {
		return hits
	}
	threshold := m.cfgSnapshot().ThresholdValue()
	kept := hits[:0]
	for i, h := range hits {
		ans, ok := res.Answers[fmt.Sprintf("rel_%d", i)]
		if !ok {
			kept = append(kept, h)
			continue
		}
		p, ok := ans.NoulValue()
		if !ok {
			kept = append(kept, h)
			continue
		}
		if p >= threshold {
			kept = append(kept, h)
		}
	}
	return kept
}

func gateByScore(hits []SearchHit) []SearchHit {
	if len(hits) == 0 {
		return hits
	}
	best := hits[0].Score
	floor := best / 2
	if floor < 2 {
		floor = 2
	}
	kept := hits[:0]
	for _, h := range hits {
		if h.Score >= floor {
			kept = append(kept, h)
		}
	}
	return kept
}

func (m *Manager) gateLLM(ctx context.Context, query string, hits []SearchHit) []SearchHit {
	prov, model, err := m.observerModel()
	if err != nil {
		return hits
	}
	var sb strings.Builder
	sb.WriteString("Which of the candidate memories below are relevant to the query?\n")
	sb.WriteString("Query: ")
	sb.WriteString(query)
	sb.WriteString("\n\nCandidates:\n")
	for i, h := range hits {
		fmt.Fprintf(&sb, "%d. %s\n", i+1, h.Title)
	}
	sb.WriteString("\nReturn only the comma-separated numbers of the relevant candidates, or \"none\" if none are relevant. Never reply with prose.")

	sys := "You are a relevance filter for a memory search tool. Reply only with comma-separated candidate numbers or \"none\"."
	input, err := prompt.New().Raw(sb.String()).Build(prompt.FormatMarkdown)
	if err != nil {
		return hits
	}

	m.observerBusy.Store(true)
	defer m.observerBusy.Store(false)
	gateAgent := agent.New("memory-gate",
		agent.WithProvider(prov),
		agent.WithModel(model),
		agent.WithSystemPrompt(sys),
		agent.WithMaxIterations(1),
	)
	res, err := gateAgent.Run(ctx, input)
	if err != nil {
		return hits
	}
	keep := parseGateIndices(res.Output)
	if keep == nil {
		return hits
	}
	kept := hits[:0]
	for _, i := range keep {
		if i >= 0 && i < len(hits) {
			kept = append(kept, hits[i])
		}
	}
	return kept
}

func shortID(id string) string {
	if len(id) <= 8 {
		return id
	}
	return id[:8]
}

func appendUnique(dst []string, src ...string) []string {
	seen := make(map[string]bool, len(dst))
	for _, d := range dst {
		seen[d] = true
	}
	for _, s := range src {
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		dst = append(dst, s)
	}
	return dst
}
