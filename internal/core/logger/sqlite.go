package logger

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/peggco/pegg/internal/core/config"
	"github.com/peggco/pegg/internal/utils/query"
	_ "modernc.org/sqlite"
)

const DriverSQLite = "sqlite"

func init() {
	RegisterDriver(DriverSQLite, func(cfg config.LoggerConfig) (Handler, error) {
		return NewSQLiteHandler(cfg)
	})
}

var createTableSQL = `
CREATE TABLE IF NOT EXISTS logs (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	timestamp DATETIME,
	level TEXT,
	message TEXT
);`

type SQLiteHandler struct {
	db          *sql.DB
	maxLogCount int
	insertCount int64
	mu          sync.Mutex
	builder     *query.Builder
}

func NewSQLiteHandler(cfg config.LoggerConfig) (*SQLiteHandler, error) {
	dbPath, err := config.GetConfigPath("logs.db")
	if err != nil {
		return nil, err
	}

	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		return nil, fmt.Errorf("logger: create data directory: %w", err)
	}

	db, err := sql.Open("sqlite", sqliteDSN(dbPath))
	if err != nil {
		return nil, err
	}

	return newSQLiteHandler(db, cfg.MaxLogCount)
}

func sqliteDSN(path string) string {
	return "file:" + filepath.ToSlash(path) +
		"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)"
}

func newSQLiteHandler(db *sql.DB, maxLogCount int) (*SQLiteHandler, error) {
	if _, err := db.Exec(createTableSQL); err != nil {
		db.Close()
		return nil, err
	}

	return &SQLiteHandler{
		db:          db,
		maxLogCount: maxLogCount,
		insertCount: 0,
		builder: query.NewBuilder("logs", "id", "timestamp", "level", "message").
			DefaultSort("timestamp", query.Desc),
	}, nil
}

func (s *SQLiteHandler) Write(rec Record) error {
	_, err := s.db.Exec("INSERT INTO logs (timestamp, level, message) VALUES (?, ?, ?)",
		rec.Timestamp, rec.Level.String(), rec.Message)
	if err != nil {
		return err
	}

	s.mu.Lock()
	s.insertCount++
	shouldCleanup := s.insertCount%100 == 0
	s.mu.Unlock()

	if shouldCleanup {
		go s.cleanupOldLogs()
	}

	return nil
}

func (s *SQLiteHandler) cleanupOldLogs() {
	if s.maxLogCount <= 0 {
		return
	}

	q := `DELETE FROM logs WHERE id NOT IN (
        SELECT id FROM logs ORDER BY id DESC LIMIT ?
    )`
	_, _ = s.db.Exec(q, s.maxLogCount)
}

func (s *SQLiteHandler) Close() error {
	return s.db.Close()
}

func (s *SQLiteHandler) Query(q query.Query) ([]Record, int, error) {
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

	records := make([]Record, 0, q.Page.Size)
	for rows.Next() {
		var rec Record
		var level string
		if err := rows.Scan(&rec.ID, &rec.Timestamp, &level, &rec.Message); err != nil {
			return nil, 0, err
		}
		rec.Level = ParseLevel(level)
		records = append(records, rec)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	return records, total, nil
}
