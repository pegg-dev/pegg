package server

import (
	"net/http"
	"testing"

	"github.com/peggco/pegg/internal/core/config"
	"github.com/peggco/pegg/internal/core/event"
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

func newTestServer(t *testing.T) *Server {
	t.Helper()
	bus := event.New()
	store := newMockStore()
	sessMgr := session.NewManager(store, bus, nil)
	cfg := &config.Config{
		Server: config.ServerConfig{
			Host: "127.0.0.1",
			Port: 0,
		},
	}
	return &Server{
		cfg:      cfg,
		bus:      bus,
		log:      nil,
		sessions: sessMgr,
		llmMgr:   nil,
	}
}

type nonFlushableResponseWriter struct {
	http.ResponseWriter
}
