package acp

import (
	"context"
	"strings"
	"testing"
	"time"

	json "github.com/goccy/go-json"

	"github.com/peggco/pegg/internal/agent"
	"github.com/peggco/pegg/internal/core/config"
	"github.com/peggco/pegg/internal/core/event"
	"github.com/peggco/pegg/internal/core/logger"
	"github.com/peggco/pegg/internal/session"
	"github.com/peggco/pegg/internal/utils/query"
)

type mockStore struct {
	sessions map[string]*session.Session
	messages map[string][]session.Message
}

func newMockStore() *mockStore {
	return &mockStore{
		sessions: make(map[string]*session.Session),
		messages: make(map[string][]session.Message),
	}
}

func (m *mockStore) Create(s session.Session) error {
	m.sessions[s.ID] = &s
	return nil
}

func (m *mockStore) Update(s session.Session) error {
	m.sessions[s.ID] = &s
	return nil
}

func (m *mockStore) Get(id string) (*session.Session, error) {
	s, ok := m.sessions[id]
	if !ok {
		return nil, session.ErrNotFound
	}
	return s, nil
}

func (m *mockStore) Delete(id string) error {
	if _, ok := m.sessions[id]; !ok {
		return session.ErrNotFound
	}
	delete(m.sessions, id)
	return nil
}

func (m *mockStore) List(q query.Query) ([]session.Session, int, error) {
	var out []session.Session
	for _, s := range m.sessions {
		out = append(out, *s)
	}
	return out, len(out), nil
}

func (m *mockStore) InsertMessage(msg session.Message) error {
	m.messages[msg.SessionID] = append(m.messages[msg.SessionID], msg)
	return nil
}

func (m *mockStore) Messages(sessionID string) ([]session.Message, error) {
	if _, ok := m.sessions[sessionID]; !ok {
		return nil, session.ErrNotFound
	}
	return m.messages[sessionID], nil
}

func (m *mockStore) TruncateAfter(sessionID, messageID string) ([]session.Message, error) {
	return nil, nil
}

func (m *mockStore) TruncateFrom(sessionID, messageID string) ([]session.Message, error) {
	return nil, nil
}

func (m *mockStore) SaveSnapshot(s session.Snapshot) error { return nil }
func (m *mockStore) Snapshots(sessionID string) ([]session.Snapshot, error) {
	return nil, nil
}
func (m *mockStore) GetSnapshot(id string) (*session.Snapshot, error) {
	return nil, session.ErrSnapshotNotFound
}
func (m *mockStore) RestoreSnapshot(sessionID string, messages []session.Message) error {
	return nil
}
func (m *mockStore) CompactionChildren(sessionID string) ([]session.Session, error) {
	return nil, nil
}
func (m *mockStore) LatestInChain(sessionID string) (*session.Session, error) {
	return m.Get(sessionID)
}
func (m *mockStore) Close() error { return nil }

func mockAgentFn() func() (*agent.Agent, error) {
	return func() (*agent.Agent, error) {
		a := agent.New("test-agent",
			agent.WithSystemPrompt("you are a test agent"),
		)
		a.Bus = event.New()
		return a, nil
	}
}

func newTestServer(t *testing.T) *Server {
	t.Helper()
	bus := event.New()
	store := newMockStore()
	sessMgr := session.NewManager(store, bus, &logger.Logger{})
	cfg := config.DefaultConfig()
	s := New(cfg, bus, &logger.Logger{}, nil, sessMgr, nil)
	s.newAgentFn = mockAgentFn()
	return s
}

type bufferTransport struct {
	msg   chan Message
	sends []string
}

func newBufferTransport() *bufferTransport {
	return &bufferTransport{msg: make(chan Message, 16)}
}

func (t *bufferTransport) Start(ctx context.Context) (<-chan Message, error) {
	return t.msg, nil
}

func (t *bufferTransport) Send(msg Message) error {
	t.sends = append(t.sends, strings.TrimSpace(string(msg.Body)))
	return nil
}

func (t *bufferTransport) Close() error { return nil }

func toJSON(t *testing.T, v any) json.RawMessage {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}
	return data
}

func sendRequest(t *testing.T, srv *Server, ctx context.Context, method string, params any) string {
	t.Helper()
	raw, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  method,
		"params":  params,
	})
	srv.handleFrame(ctx, Message{Body: raw})
	if bt, ok := srv.transport.(*bufferTransport); ok && len(bt.sends) > 0 {
		return bt.sends[len(bt.sends)-1]
	}
	return ""
}

func sendNotification(t *testing.T, srv *Server, ctx context.Context, method string, params any) {
	t.Helper()
	raw, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"method":  method,
		"params":  params,
	})
	srv.handleFrame(ctx, Message{Body: raw})
}

func init() {
	_ = time.Second
}
