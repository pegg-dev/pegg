package todo

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	json "github.com/goccy/go-json"

	"github.com/peggco/pegg/internal/agent"
	"github.com/peggco/pegg/internal/agent/tools"
	"github.com/peggco/pegg/internal/core/config"
	"github.com/peggco/pegg/internal/core/event"
	"github.com/peggco/pegg/internal/session"
	"github.com/peggco/pegg/internal/vfs"
)

type Todo struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Status      string    `json:"status"`
	Priority    string    `json:"priority"`
	DependsOn   []string  `json:"dependsOn,omitempty"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

const defaultSessionID = "default"

type sessionStore struct {
	file   string
	todos  map[string]*Todo
	loaded bool
}

type todoStore struct {
	mu            sync.Mutex
	bus           event.Bus
	agentSessions map[string]string
	sessions      map[string]*sessionStore
}

var store = &todoStore{
	agentSessions: make(map[string]string),
	sessions:      make(map[string]*sessionStore),
}

func TodoTools(fs *vfs.VFS, bus event.Bus) {
	store.subscribe(bus)
	tools.Register(todoreadTool(fs))
	tools.Register(todowriteTool(fs))
}

func (s *todoStore) subscribe(bus event.Bus) {
	if bus == nil {
		return
	}
	s.mu.Lock()
	if s.bus == bus {
		s.mu.Unlock()
		return
	}
	s.bus = bus
	s.mu.Unlock()
	_ = bus.Subscribe(session.TopicSessionAttached, func(attached session.SessionAttached) {
		s.onSessionAttached(attached.AgentID, attached.SessionID)
	})
}

func (s *todoStore) onSessionAttached(agentID, sessionID string) {
	if sessionID == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.agentSessions[agentID] = sessionID
}

func (s *todoStore) sessionIDLocked(ctx context.Context) string {
	if a := agent.FromContext(ctx); a != nil {
		if id := s.agentSessions[a.ID]; id != "" {
			return id
		}
	}
	return defaultSessionID
}

func (s *todoStore) initFor(sessionID string) (*sessionStore, error) {
	ss, ok := s.sessions[sessionID]
	if !ok {
		ss = &sessionStore{todos: make(map[string]*Todo)}
		s.sessions[sessionID] = ss
	}
	if ss.loaded {
		return ss, nil
	}
	if err := config.EnsureProjectConfigDir(); err != nil {
		return nil, fmt.Errorf("todo: ensure config dir: %w", err)
	}
	file, err := config.GetProjectConfigPath("todos", sessionID+".json")
	if err != nil {
		return nil, fmt.Errorf("todo: config path: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		return nil, fmt.Errorf("todo: ensure todos dir: %w", err)
	}
	ss.file = file

	data, err := os.ReadFile(file)
	if err != nil {
		if os.IsNotExist(err) {
			ss.loaded = true
			return ss, nil
		}
		return nil, fmt.Errorf("todo: read file: %w", err)
	}
	var list []*Todo
	if err := json.Unmarshal(data, &list); err != nil {
		return nil, fmt.Errorf("todo: parse: %w", err)
	}
	for _, t := range list {
		ss.todos[t.ID] = t
	}
	ss.loaded = true
	return ss, nil
}

func (s *todoStore) save(ss *sessionStore) error {
	list := make([]*Todo, 0, len(ss.todos))
	for _, t := range ss.todos {
		list = append(list, t)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].CreatedAt.After(list[j].CreatedAt) })
	data, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return fmt.Errorf("todo: marshal: %w", err)
	}
	if err := os.WriteFile(ss.file, data, 0o644); err != nil {
		return fmt.Errorf("todo: write: %w", err)
	}
	return nil
}

func (s *todoStore) all(ctx context.Context) ([]*Todo, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ss, err := s.initFor(s.sessionIDLocked(ctx))
	if err != nil {
		return nil, err
	}
	out := make([]*Todo, 0, len(ss.todos))
	for _, t := range ss.todos {
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}

func (s *todoStore) setAll(ctx context.Context, todos []*Todo) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	ss, err := s.initFor(s.sessionIDLocked(ctx))
	if err != nil {
		return err
	}
	now := time.Now()
	ss.todos = make(map[string]*Todo, len(todos))
	for _, t := range todos {
		if t.CreatedAt.IsZero() {
			t.CreatedAt = now
		}
		t.UpdatedAt = now
		ss.todos[t.ID] = t
	}
	return s.save(ss)
}

func formatTodoList(todos []*Todo) string {
	if len(todos) == 0 {
		return "No todos.\n"
	}
	var b strings.Builder
	for _, t := range todos {
		status := statusIcon(t.Status)
		priority := priorityLabel(t.Priority)
		fmt.Fprintf(&b, "%s [%s] %s (priority: %s)\n", status, t.ID, t.Title, priority)
		if t.Description != "" {
			fmt.Fprintf(&b, "   %s\n", t.Description)
		}
		if len(t.DependsOn) > 0 {
			fmt.Fprintf(&b, "   depends on: %s\n", strings.Join(t.DependsOn, ", "))
		}
	}
	return b.String()
}

func statusIcon(status string) string {
	switch status {
	case "completed":
		return "[x]"
	case "in_progress":
		return "[~]"
	case "cancelled":
		return "[-]"
	default:
		return "[ ]"
	}
}

func priorityLabel(p string) string {
	switch p {
	case "high":
		return "high"
	case "low":
		return "low"
	default:
		return "medium"
	}
}
