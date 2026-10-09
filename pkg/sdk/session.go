package sdk

import (
	"github.com/peggco/pegg/internal/session"
	"github.com/peggco/pegg/internal/utils/query"
)

type CreateSessionOptions struct {
	Title           string
	Provider        string
	Model           string
	ReasoningEffort string
	ProjectDir      string
}

func (e *Engine) CreateSession(opts CreateSessionOptions) (*Session, error) {
	if err := e.checkOpen(); err != nil {
		return nil, err
	}
	return e.sessions.Create(session.CreateOptions(opts))
}

func (e *Engine) GetSession(id string) (*Session, error) {
	if err := e.checkOpen(); err != nil {
		return nil, err
	}
	s, err := e.sessions.Get(id)
	if err != nil {
		if err == session.ErrNotFound {
			return nil, ErrNoSession
		}
		return nil, err
	}
	return s, nil
}

func (e *Engine) DeleteSession(id string) error {
	if err := e.checkOpen(); err != nil {
		return err
	}
	return e.sessions.Delete(id)
}

func (e *Engine) SetSessionTitle(id, title string) error {
	if err := e.checkOpen(); err != nil {
		return err
	}
	return e.sessions.SetTitle(id, title)
}

func (e *Engine) SessionMessages(id string) ([]SessionMessage, error) {
	if err := e.checkOpen(); err != nil {
		return nil, err
	}
	return e.sessions.Messages(id)
}

func (e *Engine) SessionSnapshots(id string) ([]SessionSnapshot, error) {
	if err := e.checkOpen(); err != nil {
		return nil, err
	}
	return e.sessions.Snapshots(id)
}

func (e *Engine) ListSessions(page, size int) ([]Session, int, error) {
	if err := e.checkOpen(); err != nil {
		return nil, 0, err
	}
	return e.sessions.List(query.Query{
		Page: query.Page{Number: page, Size: size},
		Sort: []query.Sort{{Column: "updated_at", Dir: query.Desc}},
		Filters: []query.Filter{
			{Column: "compaction_parent_id", Operator: query.OpEqual, Value: ""},
		},
	})
}

func (e *Engine) ForkSession(sourceID, atMessageID string) (*Session, error) {
	if err := e.checkOpen(); err != nil {
		return nil, err
	}
	return e.sessions.Fork(sourceID, atMessageID)
}

func (e *Engine) AppendMessage(id string, msg Message) (*SessionMessage, error) {
	if err := e.checkOpen(); err != nil {
		return nil, err
	}
	return e.sessions.AppendMessage(id, msg)
}

func sessionListQuery(projectDir string) query.Query {
	q := query.Query{
		Page: query.Page{Number: 1, Size: 20},
		Sort: []query.Sort{{Column: "updated_at", Dir: query.Desc}},
	}
	filters := []query.Filter{
		{Column: "compaction_parent_id", Operator: query.OpEqual, Value: ""},
	}
	if projectDir != "" {
		filters = append(filters, query.Filter{Column: "project_dir", Operator: query.OpEqual, Value: projectDir})
	}
	q.Filters = filters
	return q
}
