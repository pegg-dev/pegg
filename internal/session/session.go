package session

import (
	"errors"
	"strings"
	"time"

	"github.com/peggco/pegg/internal/llm"
	"github.com/peggco/pegg/internal/utils/query"
)

const defaultTitlePrefix = "New Session "

func defaultSessionTitle() string {
	return defaultTitlePrefix + time.Now().Format("2006-01-02 15:04:05")
}

func isPlaceholderTitle(title string) bool {
	return strings.HasPrefix(title, defaultTitlePrefix)
}

var (
	ErrNotFound         = errors.New("session: not found")
	ErrDuplicate        = errors.New("session: already exists")
	ErrMessageNotFound  = errors.New("session: message not found")
	ErrSnapshotNotFound = errors.New("session: snapshot not found")
	ErrEmptyTitle       = errors.New("session: title must not be empty")
)

type Session struct {
	ID                 string
	Title              string
	Provider           string
	Model              string
	ReasoningEffort    string
	ProjectDir         string
	ParentID           string
	CompactionParentID string
	CreatedAt          time.Time
	UpdatedAt          time.Time
	Usage              llm.Usage
}

type Message struct {
	ID         string
	SessionID  string
	Seq        int
	Role       llm.Role
	Content    any
	Reasoning  any
	Name       string
	ToolCallID string
	ToolCalls  []llm.ToolCall
	CreatedAt  time.Time
}

type Snapshot struct {
	ID            string
	SessionID     string
	HeadMessageID string
	CreatedAt     time.Time
	Messages      []Message
}

type Store interface {
	Create(s Session) error
	Update(s Session) error
	Get(id string) (*Session, error)
	Delete(id string) error
	List(q query.Query) ([]Session, int, error)

	InsertMessage(m Message) error
	Messages(sessionID string) ([]Message, error)
	TruncateAfter(sessionID, messageID string) ([]Message, error)
	TruncateFrom(sessionID, messageID string) ([]Message, error)

	SaveSnapshot(s Snapshot) error
	Snapshots(sessionID string) ([]Snapshot, error)
	GetSnapshot(id string) (*Snapshot, error)
	RestoreSnapshot(sessionID string, messages []Message) error

	CompactionChildren(sessionID string) ([]Session, error)
	LatestInChain(sessionID string) (*Session, error)

	Close() error
}

type CreateOptions struct {
	Title           string
	Provider        string
	Model           string
	ReasoningEffort string
	ProjectDir      string
}
