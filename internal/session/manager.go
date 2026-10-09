package session

import (
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/peggco/pegg/internal/core/event"
	"github.com/peggco/pegg/internal/core/logger"
	"github.com/peggco/pegg/internal/llm"
	"github.com/peggco/pegg/internal/utils/query"
)

type Manager struct {
	store   Store
	bus     event.Bus
	log     *logger.Logger
	mu      sync.RWMutex
	current string
}

func NewManager(store Store, bus event.Bus, log *logger.Logger) *Manager {
	return &Manager{store: store, bus: bus, log: log}
}

func (m *Manager) Create(opts CreateOptions) (*Session, error) {
	if opts.Title == "" {
		opts.Title = defaultSessionTitle()
	}
	now := time.Now()
	s := &Session{
		ID:              uuid.NewString(),
		Title:           opts.Title,
		Provider:        opts.Provider,
		Model:           opts.Model,
		ReasoningEffort: opts.ReasoningEffort,
		ProjectDir:      opts.ProjectDir,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	if err := m.store.Create(*s); err != nil {
		return nil, err
	}
	m.publish(TopicSessionCreated, SessionCreated{SessionID: s.ID})
	return s, nil
}

func (m *Manager) Get(id string) (*Session, error) {
	return m.store.Get(id)
}

func (m *Manager) List(q query.Query) ([]Session, int, error) {
	return m.store.List(q)
}

func (m *Manager) Delete(id string) error {
	children, err := m.store.CompactionChildren(id)
	if err != nil {
		return err
	}
	for _, c := range children {
		if err := m.Delete(c.ID); err != nil {
			return err
		}
	}
	if err := m.store.Delete(id); err != nil {
		return err
	}
	m.mu.Lock()
	if m.current == id {
		m.current = ""
	}
	m.mu.Unlock()
	m.publish(TopicSessionDeleted, SessionDeleted{SessionID: id})
	return nil
}

func (m *Manager) SetTitle(id, title string) error {
	if title == "" {
		return ErrEmptyTitle
	}
	s, err := m.store.Get(id)
	if err != nil {
		return err
	}
	s.Title = title
	s.UpdatedAt = time.Now()
	if err := m.store.Update(*s); err != nil {
		return err
	}
	m.publish(TopicSessionUpdated, SessionUpdated{SessionID: id})
	return nil
}

func (m *Manager) AppendMessage(sessionID string, msg llm.Message) (*Message, error) {
	if _, err := m.store.Get(sessionID); err != nil {
		return nil, err
	}
	msgs, err := m.store.Messages(sessionID)
	if err != nil {
		return nil, err
	}
	seq := 0
	for _, mm := range msgs {
		if mm.Seq > seq {
			seq = mm.Seq
		}
	}
	rec := Message{
		ID:         uuid.NewString(),
		SessionID:  sessionID,
		Seq:        seq + 1,
		Role:       msg.Role,
		Content:    msg.Content,
		Reasoning:  msg.Reasoning,
		Name:       msg.Name,
		ToolCallID: msg.ToolCallID,
		ToolCalls:  msg.ToolCalls,
		CreatedAt:  time.Now(),
	}
	if err := m.store.InsertMessage(rec); err != nil {
		return nil, err
	}
	s, err := m.store.Get(sessionID)
	if err != nil {
		return nil, err
	}
	s.UpdatedAt = rec.CreatedAt
	_ = m.store.Update(*s)
	m.publish(TopicSessionMessageAdded, SessionMessageAdded{SessionID: sessionID, Message: rec})
	return &rec, nil
}

func (m *Manager) AccumulateUsage(sessionID string, u llm.Usage) error {
	s, err := m.store.Get(sessionID)
	if err != nil {
		return err
	}
	s.Usage.PromptTokens = u.PromptTokens
	s.Usage.CompletionTokens = u.CompletionTokens
	s.Usage.TotalTokens = u.TotalTokens
	s.Usage.Cost += u.Cost
	s.UpdatedAt = time.Now()
	return m.store.Update(*s)
}

func (m *Manager) Fork(sourceID, atMessageID string) (*Session, error) {
	src, err := m.store.Get(sourceID)
	if err != nil {
		return nil, err
	}
	msgs, err := m.store.Messages(sourceID)
	if err != nil {
		return nil, err
	}
	idx := -1
	for i, mm := range msgs {
		if mm.ID == atMessageID {
			idx = i
			break
		}
	}
	if idx == -1 {
		return nil, ErrMessageNotFound
	}

	now := time.Now()
	fork := *src
	fork.ID = uuid.NewString()
	fork.Title = src.Title + " - forked"
	fork.ParentID = src.ID
	fork.CreatedAt = now
	fork.UpdatedAt = now
	fork.Usage = llm.Usage{}
	if err := m.store.Create(fork); err != nil {
		return nil, err
	}
	for i, mm := range msgs[:idx+1] {
		cp := mm
		cp.ID = uuid.NewString()
		cp.SessionID = fork.ID
		cp.Seq = i + 1
		cp.CreatedAt = now
		if err := m.store.InsertMessage(cp); err != nil {
			return nil, err
		}
	}
	m.publish(TopicSessionForked, SessionForked{
		SessionID:       fork.ID,
		ParentID:        src.ID,
		ParentMessageID: atMessageID,
	})
	return &fork, nil
}

func (m *Manager) ForkBefore(sourceID, atMessageID string) (*Session, error) {
	src, err := m.store.Get(sourceID)
	if err != nil {
		return nil, err
	}
	msgs, err := m.store.Messages(sourceID)
	if err != nil {
		return nil, err
	}
	idx := -1
	for i, mm := range msgs {
		if mm.ID == atMessageID {
			idx = i
			break
		}
	}
	if idx == -1 {
		return nil, ErrMessageNotFound
	}

	now := time.Now()
	fork := *src
	fork.ID = uuid.NewString()
	fork.Title = src.Title + " - fork"
	fork.ParentID = src.ID
	fork.CreatedAt = now
	fork.UpdatedAt = now
	fork.Usage = llm.Usage{}
	if err := m.store.Create(fork); err != nil {
		return nil, err
	}
	for i, mm := range msgs[:idx] {
		cp := mm
		cp.ID = uuid.NewString()
		cp.SessionID = fork.ID
		cp.Seq = i + 1
		cp.CreatedAt = now
		if err := m.store.InsertMessage(cp); err != nil {
			return nil, err
		}
	}
	m.publish(TopicSessionForked, SessionForked{
		SessionID:       fork.ID,
		ParentID:        src.ID,
		ParentMessageID: atMessageID,
	})
	return &fork, nil
}

func (m *Manager) Revert(sessionID, toMessageID string) (string, error) {
	sess, err := m.store.Get(sessionID)
	if err != nil {
		return "", err
	}
	msgs, err := m.store.Messages(sessionID)
	if err != nil {
		return "", err
	}
	found := false
	for _, mm := range msgs {
		if mm.ID == toMessageID {
			found = true
			break
		}
	}
	if !found {
		return "", ErrMessageNotFound
	}

	removed, err := m.store.TruncateAfter(sessionID, toMessageID)
	if err != nil {
		return "", err
	}
	if len(removed) == 0 {
		return "", nil
	}

	snap := Snapshot{
		ID:            uuid.NewString(),
		SessionID:     sessionID,
		HeadMessageID: toMessageID,
		CreatedAt:     time.Now(),
		Messages:      removed,
	}
	if err := m.store.SaveSnapshot(snap); err != nil {
		return "", err
	}

	sess.UpdatedAt = snap.CreatedAt
	_ = m.store.Update(*sess)
	m.publish(TopicSessionReverted, SessionReverted{
		SessionID:   sessionID,
		ToMessageID: toMessageID,
		SnapshotID:  snap.ID,
	})
	return snap.ID, nil
}

func (m *Manager) RevertFrom(sessionID, fromMessageID string) (string, []Message, error) {
	sess, err := m.store.Get(sessionID)
	if err != nil {
		return "", nil, err
	}
	msgs, err := m.store.Messages(sessionID)
	if err != nil {
		return "", nil, err
	}
	found := false
	for _, mm := range msgs {
		if mm.ID == fromMessageID {
			found = true
			break
		}
	}
	if !found {
		return "", nil, ErrMessageNotFound
	}

	removed, err := m.store.TruncateFrom(sessionID, fromMessageID)
	if err != nil {
		return "", nil, err
	}
	if len(removed) == 0 {
		return "", nil, nil
	}

	snap := Snapshot{
		ID:            uuid.NewString(),
		SessionID:     sessionID,
		HeadMessageID: fromMessageID,
		CreatedAt:     time.Now(),
		Messages:      removed,
	}
	if err := m.store.SaveSnapshot(snap); err != nil {
		return "", nil, err
	}

	sess.UpdatedAt = snap.CreatedAt
	_ = m.store.Update(*sess)
	m.publish(TopicSessionReverted, SessionReverted{
		SessionID:   sessionID,
		ToMessageID: fromMessageID,
		SnapshotID:  snap.ID,
	})
	return snap.ID, removed, nil
}

func (m *Manager) UndoRevert(sessionID, snapshotID string) error {
	snap, err := m.store.GetSnapshot(snapshotID)
	if err != nil {
		return err
	}
	if snap.SessionID != sessionID {
		return ErrSnapshotNotFound
	}
	if err := m.store.RestoreSnapshot(sessionID, snap.Messages); err != nil {
		return err
	}
	sess, err := m.store.Get(sessionID)
	if err != nil {
		return err
	}
	sess.UpdatedAt = time.Now()
	_ = m.store.Update(*sess)
	m.publish(TopicSessionRestored, SessionRestored{SessionID: sessionID, SnapshotID: snapshotID})
	return nil
}

func (m *Manager) Snapshots(sessionID string) ([]Snapshot, error) {
	return m.store.Snapshots(sessionID)
}

func (m *Manager) Messages(sessionID string) ([]Message, error) {
	return m.store.Messages(sessionID)
}

func (m *Manager) CompactionChildren(sessionID string) ([]Session, error) {
	return m.store.CompactionChildren(sessionID)
}

func (m *Manager) LatestInChain(sessionID string) (*Session, error) {
	return m.store.LatestInChain(sessionID)
}

func (m *Manager) CompactSession(sourceID string, compactedMessages []llm.Message, strategy string) (*Session, error) {
	src, err := m.store.Get(sourceID)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	newSess := *src
	newSess.ID = uuid.NewString()
	newSess.CompactionParentID = src.ID
	newSess.Title = src.Title + " (compacted)"
	newSess.CreatedAt = now
	newSess.UpdatedAt = now
	newSess.Usage = llm.Usage{}
	if err := m.store.Create(newSess); err != nil {
		return nil, err
	}

	for i, msg := range compactedMessages {
		rec := Message{
			ID:         uuid.NewString(),
			SessionID:  newSess.ID,
			Seq:        i + 1,
			Role:       msg.Role,
			Content:    msg.Content,
			Reasoning:  msg.Reasoning,
			Name:       msg.Name,
			ToolCallID: msg.ToolCallID,
			ToolCalls:  msg.ToolCalls,
			CreatedAt:  now,
		}
		if err := m.store.InsertMessage(rec); err != nil {
			return nil, err
		}
	}

	m.publish(TopicSessionCompacted, SessionCompacted{
		SessionID:       newSess.ID,
		ParentSessionID: src.ID,
		Strategy:        strategy,
		MessageCount:    len(compactedMessages),
	})
	return &newSess, nil
}

func (m *Manager) Bus() event.Bus {
	return m.bus
}

func (m *Manager) SetCurrent(id string) error {
	if id != "" {
		if _, err := m.store.Get(id); err != nil {
			return err
		}
	}
	m.mu.Lock()
	m.current = id
	m.mu.Unlock()
	m.publish(TopicSessionCurrentChanged, SessionCurrentChanged{SessionID: id})
	return nil
}

func (m *Manager) Current() (string, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.current, m.current != ""
}

func (m *Manager) Close() error {
	return m.store.Close()
}

func (m *Manager) publish(topic string, payload any) {
	if m.bus == nil {
		return
	}
	m.bus.Publish(topic, payload)
}
