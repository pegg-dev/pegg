package subagent

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	json "github.com/goccy/go-json"

	"github.com/peggco/pegg/internal/agent"
	"github.com/peggco/pegg/internal/agent/reminder"
	"github.com/peggco/pegg/internal/core/config"
	"github.com/peggco/pegg/internal/core/event"
	"github.com/peggco/pegg/internal/session"
)

type Status string

const (
	StatusPending     Status = "pending"
	StatusRunning     Status = "running"
	StatusCompleted   Status = "completed"
	StatusFailed      Status = "failed"
	StatusInterrupted Status = "interrupted"
)

const defaultSessionID = "default"

type SubAgent struct {
	Name          string    `json:"name"`
	AgentType     string    `json:"agent"`
	TaskIDs       []string  `json:"task_id,omitempty"`
	Background    bool      `json:"background"`
	ParentAgentID string    `json:"parent_agent_id,omitempty"`
	Status        Status    `json:"status"`
	Output        string    `json:"output,omitempty"`
	Err           string    `json:"error,omitempty"`
	SessionID     string    `json:"session_id,omitempty"`
	CreatedAt     time.Time `json:"createdAt"`
	UpdatedAt     time.Time `json:"updatedAt"`

	sessionID string
	agentID   string
	done      chan struct{}
	finished  bool
}

type sessionStore struct {
	file   string
	agents map[string]*SubAgent
	loaded bool
}

type registry struct {
	mu            sync.Mutex
	agentSessions map[string]string
	parents       map[string]*agent.Agent
	sessions      map[string]*sessionStore
	subBus        event.Bus
}

var store = newRegistry()

func newRegistry() *registry {
	return &registry{
		agentSessions: make(map[string]string),
		parents:       make(map[string]*agent.Agent),
		sessions:      make(map[string]*sessionStore),
	}
}

func (r *registry) init() error {
	if err := config.EnsureProjectConfigDir(); err != nil {
		return fmt.Errorf("subagent: ensure config dir: %w", err)
	}
	return nil
}

func (r *registry) storeFor(sessionID string) (*sessionStore, error) {
	if err := r.init(); err != nil {
		return nil, err
	}
	r.mu.Lock()
	ss, ok := r.sessions[sessionID]
	if !ok {
		ss = &sessionStore{agents: make(map[string]*SubAgent)}
		r.sessions[sessionID] = ss
	}
	r.mu.Unlock()
	if ss.loaded {
		return ss, nil
	}

	file, err := config.GetProjectConfigPath("subagents", sessionID+".json")
	if err != nil {
		return nil, fmt.Errorf("subagent: config path: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		return nil, fmt.Errorf("subagent: ensure subagents dir: %w", err)
	}
	ss.file = file

	data, err := os.ReadFile(file)
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("subagent: read file: %w", err)
		}
	}
	if len(data) == 0 && sessionID == defaultSessionID {
		if legacy, lerr := config.GetProjectConfigPath("subagents.json"); lerr == nil {
			if ldata, rerr := os.ReadFile(legacy); rerr == nil {
				data = ldata
			}
		}
	}

	var list []*SubAgent
	if len(data) > 0 {
		if err := json.Unmarshal(data, &list); err != nil {
			return nil, fmt.Errorf("subagent: parse: %w", err)
		}
	}
	changed := false
	for _, sa := range list {
		if sa.Status == StatusPending || sa.Status == StatusRunning {
			sa.Status = StatusInterrupted
			sa.Err = "interrupted: app restarted"
			sa.UpdatedAt = time.Now()
			changed = true
		}
		sa.done = make(chan struct{})
		if sa.Status == StatusCompleted || sa.Status == StatusFailed || sa.Status == StatusInterrupted {
			sa.finished = true
			close(sa.done)
		}
		sa.sessionID = sessionID
		ss.agents[sa.Name] = sa
	}
	r.mu.Lock()
	ss.loaded = true
	r.mu.Unlock()
	if changed {
		_ = r.saveStore(ss)
	}
	return ss, nil
}

func (r *registry) saveStore(ss *sessionStore) error {
	list := make([]*SubAgent, 0, len(ss.agents))
	for _, sa := range ss.agents {
		list = append(list, sa)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].CreatedAt.Before(list[j].CreatedAt) })
	data, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return fmt.Errorf("subagent: marshal: %w", err)
	}
	if err := os.WriteFile(ss.file, data, 0o644); err != nil {
		return fmt.Errorf("subagent: write: %w", err)
	}
	return nil
}

func (r *registry) saveFor(sa *SubAgent) {
	if sa == nil || sa.sessionID == "" {
		return
	}
	if ss, err := r.storeFor(sa.sessionID); err == nil {
		_ = r.saveStore(ss)
	}
}

func (r *registry) sessionFor(parent *agent.Agent) string {
	if parent == nil {
		return defaultSessionID
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if id := r.agentSessions[parent.ID]; id != "" {
		return id
	}
	return defaultSessionID
}

func (r *registry) subscribe(bus event.Bus) {
	if bus == nil {
		return
	}
	r.mu.Lock()
	if r.subBus == bus {
		r.mu.Unlock()
		return
	}
	r.subBus = bus
	r.mu.Unlock()

	bus.Subscribe(agent.TopicAgentStarted, func(started agent.AgentStarted) {
		r.onStarted(started.AgentID)
	})
	bus.Subscribe(agent.TopicAgentFinished, func(finished agent.AgentFinished) {
		r.onFinished(finished.AgentID, finished.Output)
	})
	bus.Subscribe(agent.TopicAgentError, func(errEv agent.AgentError) {
		r.onError(errEv.AgentID, errEv.Err)
	})
	bus.Subscribe(session.TopicSessionAttached, func(attached session.SessionAttached) {
		r.onSessionAttached(attached.AgentID, attached.SessionID)
	})
	bus.Subscribe(session.TopicSessionResume, func(resume session.SessionResume) {
		r.onSessionResume(resume.AgentID, resume.SessionID)
	})
}

func (r *registry) onSessionAttached(agentID, sessionID string) {
	if sessionID == "" {
		return
	}
	r.mu.Lock()
	r.agentSessions[agentID] = sessionID
	r.mu.Unlock()
	if sa := r.byAgentID(agentID); sa != nil {
		sa.SessionID = sessionID
	}
}

func (r *registry) onSessionResume(agentID, sessionID string) {
	if agentID == "" || sessionID == "" {
		return
	}
	r.mu.Lock()
	r.agentSessions[agentID] = sessionID
	r.mu.Unlock()
}

func (r *registry) byAgentID(agentID string) *SubAgent {
	if agentID == "" {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, ss := range r.sessions {
		for _, sa := range ss.agents {
			if sa.agentID == agentID {
				return sa
			}
		}
	}
	return nil
}

func IsSubagentSession(sessionID string) bool {
	if sessionID == "" {
		return false
	}
	if err := store.init(); err != nil {
		return false
	}
	store.mu.Lock()
	for _, ss := range store.sessions {
		for _, sa := range ss.agents {
			if sa.SessionID == sessionID {
				store.mu.Unlock()
				return true
			}
		}
	}
	store.mu.Unlock()

	dir, err := config.GetProjectConfigPath("subagents")
	if err != nil {
		return false
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		var list []*SubAgent
		if err := json.Unmarshal(data, &list); err != nil {
			continue
		}
		for _, sa := range list {
			if sa.SessionID == sessionID {
				return true
			}
		}
	}
	return false
}

func (r *registry) onStarted(agentID string) {
	if sa := r.byAgentID(agentID); sa != nil {
		r.start(sa)
	}
}

func (r *registry) onFinished(agentID, output string) {
	if sa := r.byAgentID(agentID); sa != nil {
		r.finish(sa, output, nil)
	}
}

func (r *registry) onError(agentID string, err error) {
	if sa := r.byAgentID(agentID); sa != nil {
		r.finish(sa, "", err)
	}
}

func (r *registry) spawn(name, agentType string, taskIDs []string, background bool, parentAgent *agent.Agent) (*SubAgent, error) {
	sessionID := r.sessionFor(parentAgent)
	ss, err := r.storeFor(sessionID)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	var parentID string
	if parentAgent != nil {
		parentID = parentAgent.ID
		r.mu.Lock()
		r.parents[parentID] = parentAgent
		r.mu.Unlock()
	}
	sa := &SubAgent{
		Name:          name,
		AgentType:     agentType,
		TaskIDs:       taskIDs,
		Background:    background,
		ParentAgentID: parentID,
		Status:        StatusPending,
		CreatedAt:     now,
		UpdatedAt:     now,
		done:          make(chan struct{}),
		sessionID:     sessionID,
	}
	r.mu.Lock()
	if _, ok := ss.agents[name]; ok {
		r.mu.Unlock()
		return nil, fmt.Errorf("subagent: duplicate name %q (names must be unique)", name)
	}
	ss.agents[name] = sa
	r.mu.Unlock()
	if err := r.saveStore(ss); err != nil {
		return nil, err
	}
	return sa, nil
}

func (r *registry) resume(name string, parent *agent.Agent) (*SubAgent, error) {
	ss, err := r.storeFor(r.sessionFor(parent))
	if err != nil {
		return nil, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	sa, ok := ss.agents[name]
	if !ok {
		return nil, fmt.Errorf("subagent: no subagent named %q", name)
	}
	if !sa.finished {
		return nil, fmt.Errorf("subagent: %q is still running", name)
	}
	sa.finished = false
	sa.Status = StatusPending
	sa.done = make(chan struct{})
	sa.UpdatedAt = time.Now()
	if err := r.saveStore(ss); err != nil {
		return nil, err
	}
	return sa, nil
}

func (r *registry) setAgentID(name, agentID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, ss := range r.sessions {
		if sa, ok := ss.agents[name]; ok {
			sa.agentID = agentID
			return
		}
	}
}

func (r *registry) start(sa *SubAgent) {
	r.mu.Lock()
	sa.Status = StatusRunning
	sa.UpdatedAt = time.Now()
	r.mu.Unlock()
	r.saveFor(sa)
}

func (r *registry) finish(sa *SubAgent, output string, err error) {
	r.mu.Lock()
	if sa.finished {
		if output != "" {
			sa.Output = output
		}
		if err != nil && sa.Err == "" {
			sa.Err = err.Error()
		}
		r.mu.Unlock()
		r.saveFor(sa)
		return
	}
	sa.finished = true
	sa.Output = output
	sa.UpdatedAt = time.Now()
	if err != nil {
		sa.Status = StatusFailed
		sa.Err = err.Error()
	} else {
		sa.Status = StatusCompleted
	}
	close(sa.done)

	if sa.Background && sa.ParentAgentID != "" {
		if parent, ok := r.parents[sa.ParentAgentID]; ok {
			var rem reminder.Reminder
			if err != nil {
				rem = reminder.SubAgentFailed(sa.Name, sa.TaskIDs, err.Error())
			} else {
				rem = reminder.SubAgentDone(sa.Name, sa.TaskIDs, output)
			}
			parent.QueueNotification(rem)
			if !r.otherBackgroundRunning(sa.ParentAgentID, sa) {
				parent.DetachReminder()
			}
			if parent.Bus != nil {
				n := agent.SubAgentNotification{
					ParentAgentID: sa.ParentAgentID,
					SubAgentName:  sa.Name,
					TaskIDs:       append([]string(nil), sa.TaskIDs...),
					Output:        output,
				}
				if err != nil {
					n.Err = err.Error()
				}
				parent.Bus.Publish(agent.TopicSubAgentNotification, n)
			}
		}
	}

	r.mu.Unlock()
	r.saveFor(sa)
}

func (r *registry) otherBackgroundRunning(parentID string, except *SubAgent) bool {
	for _, ss := range r.sessions {
		for _, sa := range ss.agents {
			if sa == except || sa.ParentAgentID != parentID || !sa.Background {
				continue
			}
			if !sa.finished {
				return true
			}
		}
	}
	return false
}

func (r *registry) get(name string, parent *agent.Agent) (SubAgent, bool) {
	ss, err := r.storeFor(r.sessionFor(parent))
	if err != nil {
		return SubAgent{}, false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	sa, ok := ss.agents[name]
	if !ok {
		return SubAgent{}, false
	}
	return copyOf(sa), true
}

func (r *registry) all(parent *agent.Agent) []SubAgent {
	ss, err := r.storeFor(r.sessionFor(parent))
	if err != nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]SubAgent, 0, len(ss.agents))
	for _, sa := range ss.agents {
		out = append(out, copyOf(sa))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out
}

func (r *registry) filter(names []string, parent *agent.Agent) ([]SubAgent, error) {
	out := make([]SubAgent, 0, len(names))
	for _, n := range names {
		sa, ok := r.get(n, parent)
		if !ok {
			return nil, fmt.Errorf("subagent: no subagent named %q", n)
		}
		out = append(out, sa)
	}
	return out, nil
}

func copyOf(sa *SubAgent) SubAgent {
	return SubAgent{
		Name:          sa.Name,
		AgentType:     sa.AgentType,
		TaskIDs:       append([]string(nil), sa.TaskIDs...),
		Background:    sa.Background,
		ParentAgentID: sa.ParentAgentID,
		Status:        sa.Status,
		Output:        sa.Output,
		Err:           sa.Err,
		SessionID:     sa.SessionID,
		CreatedAt:     sa.CreatedAt,
		UpdatedAt:     sa.UpdatedAt,
	}
}

func formatStatuses(sas []SubAgent) string {
	if len(sas) == 0 {
		return "No subagents.\n"
	}
	var b strings.Builder
	for _, sa := range sas {
		fmt.Fprintf(&b, "%s %s (agent: %s, background: %v)\n", sa.Status, sa.Name, sa.AgentType, sa.Background)
		if len(sa.TaskIDs) > 0 {
			fmt.Fprintf(&b, "   task_id: %s\n", strings.Join(sa.TaskIDs, ", "))
		}
		if sa.Status == StatusCompleted {
			fmt.Fprintf(&b, "   response:\n%s\n", sa.Output)
		}
		if (sa.Status == StatusFailed || sa.Status == StatusInterrupted) && sa.Err != "" {
			fmt.Fprintf(&b, "   error: %s\n", sa.Err)
		}
	}
	return b.String()
}
