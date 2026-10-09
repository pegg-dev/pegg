package session

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	json "github.com/goccy/go-json"

	"github.com/peggco/pegg/internal/core/config"
	"github.com/peggco/pegg/internal/llm"
	"github.com/peggco/pegg/internal/utils/query"
	_ "modernc.org/sqlite"
)

const DriverSQLite = "sqlite"

func init() {
	RegisterDriver(DriverSQLite, func(cfg config.SessionConfig) (Store, error) {
		return NewSQLiteStore()
	})
}

var createTableSQL = "CREATE TABLE IF NOT EXISTS sessions (" +
	"id TEXT PRIMARY KEY," +
	"title TEXT NOT NULL DEFAULT ''," +
	"provider TEXT NOT NULL DEFAULT ''," +
	"model TEXT NOT NULL DEFAULT ''," +
	"reasoning_effort TEXT NOT NULL DEFAULT ''," +
	"project_dir TEXT NOT NULL DEFAULT ''," +
	"parent_id TEXT NOT NULL DEFAULT ''," +
	"compaction_parent_id TEXT NOT NULL DEFAULT ''," +
	"created_at DATETIME NOT NULL," +
	"updated_at DATETIME NOT NULL," +
	"usage TEXT NOT NULL DEFAULT '{}');" +
	"CREATE TABLE IF NOT EXISTS session_messages (" +
	"id TEXT PRIMARY KEY," +
	"session_id TEXT NOT NULL," +
	"seq INTEGER NOT NULL," +
	"role TEXT NOT NULL," +
	"content TEXT NOT NULL DEFAULT ''," +
	"reasoning TEXT NOT NULL DEFAULT ''," +
	"name TEXT NOT NULL DEFAULT ''," +
	"tool_call_id TEXT NOT NULL DEFAULT ''," +
	"tool_calls TEXT NOT NULL DEFAULT ''," +
	"created_at DATETIME NOT NULL);" +
	"CREATE INDEX IF NOT EXISTS idx_session_messages_session_seq ON session_messages (session_id, seq);" +
	"CREATE TABLE IF NOT EXISTS session_snapshots (" +
	"id TEXT PRIMARY KEY," +
	"session_id TEXT NOT NULL," +
	"head_message_id TEXT NOT NULL," +
	"created_at DATETIME NOT NULL," +
	"messages TEXT NOT NULL);"

type SQLiteStore struct {
	db      *sql.DB
	builder *query.Builder
}

func NewSQLiteStore() (*SQLiteStore, error) {
	dbPath, err := config.GetConfigPath("sessions.db")
	if err != nil {
		return nil, err
	}
	return newSQLiteStoreAt(dbPath)
}

func NewSQLiteStoreAt(dbPath string) (*SQLiteStore, error) {
	return newSQLiteStoreAt(dbPath)
}

func newSQLiteStoreAt(dbPath string) (*SQLiteStore, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		return nil, fmt.Errorf("session: create data directory: %w", err)
	}

	db, err := sql.Open("sqlite", sqliteDSN(dbPath))
	if err != nil {
		return nil, err
	}

	return newSQLiteStore(db)
}

func sqliteDSN(path string) string {
	return "file:" + filepath.ToSlash(path) +
		"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)"
}

func newSQLiteStore(db *sql.DB) (*SQLiteStore, error) {
	if _, err := db.Exec(createTableSQL); err != nil {
		db.Close()
		return nil, err
	}

	return &SQLiteStore{
		db: db,
		builder: query.NewBuilder("sessions",
			"id", "title", "provider", "model", "reasoning_effort", "project_dir",
			"parent_id", "compaction_parent_id", "created_at", "updated_at",
		).DefaultSort("created_at", query.Desc),
	}, nil
}

func (s *SQLiteStore) Create(sess Session) error {
	_, err := s.db.Exec(
		"INSERT INTO sessions (id, title, provider, model, reasoning_effort, project_dir, parent_id, compaction_parent_id, created_at, updated_at, usage) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
		sess.ID, sess.Title, sess.Provider, sess.Model, sess.ReasoningEffort, sess.ProjectDir,
		sess.ParentID, sess.CompactionParentID, sess.CreatedAt, sess.UpdatedAt, marshalJSON(sess.Usage),
	)
	return err
}

func (s *SQLiteStore) Update(sess Session) error {
	res, err := s.db.Exec(
		"UPDATE sessions SET title = ?, provider = ?, model = ?, reasoning_effort = ?, project_dir = ?, parent_id = ?, compaction_parent_id = ?, created_at = ?, updated_at = ?, usage = ? WHERE id = ?",
		sess.Title, sess.Provider, sess.Model, sess.ReasoningEffort, sess.ProjectDir,
		sess.ParentID, sess.CompactionParentID, sess.CreatedAt, sess.UpdatedAt, marshalJSON(sess.Usage), sess.ID,
	)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *SQLiteStore) Get(id string) (*Session, error) {
	row := s.db.QueryRow(
		"SELECT id, title, provider, model, reasoning_effort, project_dir, parent_id, compaction_parent_id, created_at, updated_at, usage FROM sessions WHERE id = ?", id,
	)
	var sess Session
	var usage string
	if err := row.Scan(
		&sess.ID, &sess.Title, &sess.Provider, &sess.Model, &sess.ReasoningEffort, &sess.ProjectDir,
		&sess.ParentID, &sess.CompactionParentID, &sess.CreatedAt, &sess.UpdatedAt, &usage,
	); err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if err := unmarshalJSON(usage, &sess.Usage); err != nil {
		return nil, err
	}
	return &sess, nil
}

func (s *SQLiteStore) Delete(id string) error {
	res, err := s.db.Exec("DELETE FROM sessions WHERE id = ?", id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *SQLiteStore) List(q query.Query) ([]Session, int, error) {
	built, err := s.builder.Build(q)
	if err != nil {
		return nil, 0, err
	}

	var total int
	if err := s.db.QueryRow(built.CountSQL, built.CountArgs...).Scan(&total); err != nil {
		return nil, 0, err
	}

	rows, err := s.db.Query(built.SelectSQL, built.SelectArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	sessions := make([]Session, 0, q.Page.Size)
	for rows.Next() {
		var sess Session
		var usage string
		if err := rows.Scan(
			&sess.ID, &sess.Title, &sess.Provider, &sess.Model, &sess.ReasoningEffort, &sess.ProjectDir,
			&sess.ParentID, &sess.CompactionParentID, &sess.CreatedAt, &sess.UpdatedAt, &usage,
		); err != nil {
			return nil, 0, err
		}
		if err := unmarshalJSON(usage, &sess.Usage); err != nil {
			return nil, 0, err
		}
		sessions = append(sessions, sess)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	return sessions, total, nil
}

func (s *SQLiteStore) InsertMessage(m Message) error {
	_, err := s.db.Exec(
		"INSERT INTO session_messages (id, session_id, seq, role, content, reasoning, name, tool_call_id, tool_calls, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
		m.ID, m.SessionID, m.Seq, string(m.Role), marshalJSON(m.Content),
		marshalJSON(m.Reasoning), m.Name, m.ToolCallID, marshalJSON(m.ToolCalls), m.CreatedAt,
	)
	return err
}

func (s *SQLiteStore) Messages(sessionID string) ([]Message, error) {
	rows, err := s.db.Query(
		"SELECT id, session_id, seq, role, content, reasoning, name, tool_call_id, tool_calls, created_at FROM session_messages WHERE session_id = ? ORDER BY seq ASC",
		sessionID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var msgs []Message
	for rows.Next() {
		var m Message
		var role, content, reasoning, toolCalls string
		if err := rows.Scan(&m.ID, &m.SessionID, &m.Seq, &role, &content, &reasoning, &m.Name, &m.ToolCallID, &toolCalls, &m.CreatedAt); err != nil {
			return nil, err
		}
		m.Role = llmRole(role)
		if err := unmarshalJSON(content, &m.Content); err != nil {
			return nil, err
		}
		if err := unmarshalJSON(reasoning, &m.Reasoning); err != nil {
			return nil, err
		}
		if toolCalls != "" {
			if err := unmarshalJSON(toolCalls, &m.ToolCalls); err != nil {
				return nil, err
			}
		}
		msgs = append(msgs, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return msgs, nil
}

func (s *SQLiteStore) TruncateAfter(sessionID, messageID string) ([]Message, error) {
	var targetSeq int
	if err := s.db.QueryRow("SELECT seq FROM session_messages WHERE id = ? AND session_id = ?", messageID, sessionID).Scan(&targetSeq); err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrMessageNotFound
		}
		return nil, err
	}

	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	rows, err := tx.Query(
		"SELECT id, session_id, seq, role, content, reasoning, name, tool_call_id, tool_calls, created_at FROM session_messages WHERE session_id = ? AND seq > ? ORDER BY seq ASC",
		sessionID, targetSeq,
	)
	if err != nil {
		return nil, err
	}
	var removed []Message
	for rows.Next() {
		var m Message
		var role, content, reasoning, toolCalls string
		if err := rows.Scan(&m.ID, &m.SessionID, &m.Seq, &role, &content, &reasoning, &m.Name, &m.ToolCallID, &toolCalls, &m.CreatedAt); err != nil {
			rows.Close()
			return nil, err
		}
		m.Role = llmRole(role)
		if err := unmarshalJSON(content, &m.Content); err != nil {
			rows.Close()
			return nil, err
		}
		if err := unmarshalJSON(reasoning, &m.Reasoning); err != nil {
			rows.Close()
			return nil, err
		}
		if toolCalls != "" {
			if err := unmarshalJSON(toolCalls, &m.ToolCalls); err != nil {
				rows.Close()
				return nil, err
			}
		}
		removed = append(removed, m)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	if _, err := tx.Exec("DELETE FROM session_messages WHERE session_id = ? AND seq > ?", sessionID, targetSeq); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return removed, nil
}

func (s *SQLiteStore) TruncateFrom(sessionID, messageID string) ([]Message, error) {
	var targetSeq int
	if err := s.db.QueryRow("SELECT seq FROM session_messages WHERE id = ? AND session_id = ?", messageID, sessionID).Scan(&targetSeq); err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrMessageNotFound
		}
		return nil, err
	}

	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	rows, err := tx.Query(
		"SELECT id, session_id, seq, role, content, reasoning, name, tool_call_id, tool_calls, created_at FROM session_messages WHERE session_id = ? AND seq >= ? ORDER BY seq ASC",
		sessionID, targetSeq,
	)
	if err != nil {
		return nil, err
	}
	var removed []Message
	for rows.Next() {
		var m Message
		var role, content, reasoning, toolCalls string
		if err := rows.Scan(&m.ID, &m.SessionID, &m.Seq, &role, &content, &reasoning, &m.Name, &m.ToolCallID, &toolCalls, &m.CreatedAt); err != nil {
			rows.Close()
			return nil, err
		}
		m.Role = llmRole(role)
		if err := unmarshalJSON(content, &m.Content); err != nil {
			rows.Close()
			return nil, err
		}
		if err := unmarshalJSON(reasoning, &m.Reasoning); err != nil {
			rows.Close()
			return nil, err
		}
		if toolCalls != "" {
			if err := unmarshalJSON(toolCalls, &m.ToolCalls); err != nil {
				rows.Close()
				return nil, err
			}
		}
		removed = append(removed, m)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	if _, err := tx.Exec("DELETE FROM session_messages WHERE session_id = ? AND seq >= ?", sessionID, targetSeq); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return removed, nil
}

func (s *SQLiteStore) SaveSnapshot(snap Snapshot) error {
	_, err := s.db.Exec(
		"INSERT INTO session_snapshots (id, session_id, head_message_id, created_at, messages) VALUES (?, ?, ?, ?, ?)",
		snap.ID, snap.SessionID, snap.HeadMessageID, snap.CreatedAt, marshalJSON(snap.Messages),
	)
	return err
}

func (s *SQLiteStore) Snapshots(sessionID string) ([]Snapshot, error) {
	rows, err := s.db.Query(
		"SELECT id, session_id, head_message_id, created_at, messages FROM session_snapshots WHERE session_id = ? ORDER BY created_at ASC",
		sessionID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var snaps []Snapshot
	for rows.Next() {
		var snap Snapshot
		var msgs string
		if err := rows.Scan(&snap.ID, &snap.SessionID, &snap.HeadMessageID, &snap.CreatedAt, &msgs); err != nil {
			return nil, err
		}
		if err := unmarshalJSON(msgs, &snap.Messages); err != nil {
			return nil, err
		}
		snaps = append(snaps, snap)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return snaps, nil
}

func (s *SQLiteStore) GetSnapshot(id string) (*Snapshot, error) {
	var snap Snapshot
	var msgs string
	err := s.db.QueryRow(
		"SELECT id, session_id, head_message_id, created_at, messages FROM session_snapshots WHERE id = ?", id,
	).Scan(&snap.ID, &snap.SessionID, &snap.HeadMessageID, &snap.CreatedAt, &msgs)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrSnapshotNotFound
		}
		return nil, err
	}
	if err := unmarshalJSON(msgs, &snap.Messages); err != nil {
		return nil, err
	}
	return &snap, nil
}

func (s *SQLiteStore) RestoreSnapshot(sessionID string, messages []Message) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for _, m := range messages {
		if _, err := tx.Exec(
			"INSERT OR REPLACE INTO session_messages (id, session_id, seq, role, content, reasoning, name, tool_call_id, tool_calls, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
			m.ID, sessionID, m.Seq, string(m.Role), marshalJSON(m.Content),
			marshalJSON(m.Reasoning), m.Name, m.ToolCallID, marshalJSON(m.ToolCalls), m.CreatedAt,
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *SQLiteStore) Close() error {
	return s.db.Close()
}

func (s *SQLiteStore) CompactionChildren(sessionID string) ([]Session, error) {
	rows, err := s.db.Query(
		"SELECT id, title, provider, model, reasoning_effort, project_dir, parent_id, compaction_parent_id, created_at, updated_at, usage FROM sessions WHERE compaction_parent_id = ? ORDER BY created_at ASC",
		sessionID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []Session
	for rows.Next() {
		var sess Session
		var usage string
		if err := rows.Scan(
			&sess.ID, &sess.Title, &sess.Provider, &sess.Model, &sess.ReasoningEffort, &sess.ProjectDir,
			&sess.ParentID, &sess.CompactionParentID, &sess.CreatedAt, &sess.UpdatedAt, &usage,
		); err != nil {
			return nil, err
		}
		if err := unmarshalJSON(usage, &sess.Usage); err != nil {
			return nil, err
		}
		sessions = append(sessions, sess)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return sessions, nil
}

func (s *SQLiteStore) LatestInChain(sessionID string) (*Session, error) {
	current, err := s.Get(sessionID)
	if err != nil {
		return nil, err
	}

	for {
		children, err := s.CompactionChildren(current.ID)
		if err != nil {
			return nil, err
		}
		if len(children) == 0 {
			return current, nil
		}
		current = &children[len(children)-1]
	}
}

func marshalJSON(v any) string {
	data, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}
	return string(data)
}

func unmarshalJSON(s string, v any) error {
	if s == "" {
		return nil
	}
	return json.Unmarshal([]byte(s), v)
}

func llmRole(s string) llm.Role {
	return llm.Role(s)
}
