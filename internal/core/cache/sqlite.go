package cache

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	"github.com/peggco/pegg/internal/core/config"
	_ "modernc.org/sqlite"
)

const DriverSQLite = "sqlite"

func init() {
	RegisterDriver(DriverSQLite, func(cfg config.CacheConfig) (Cache, error) {
		return NewSQLiteCache()
	})
}

var createCacheTableSQL = `
CREATE TABLE IF NOT EXISTS cache (
	key TEXT PRIMARY KEY,
	value BLOB
);`

type SQLiteCache struct {
	db *sql.DB
}

func NewSQLiteCache() (*SQLiteCache, error) {
	dbPath, err := config.GetConfigPath("cache.db")
	if err != nil {
		return nil, err
	}

	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		return nil, fmt.Errorf("cache: create data directory: %w", err)
	}

	db, err := sql.Open("sqlite", cacheSQLiteDSN(dbPath))
	if err != nil {
		return nil, err
	}

	return newSQLiteCache(db)
}

func cacheSQLiteDSN(path string) string {
	return "file:" + filepath.ToSlash(path) +
		"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)"
}

func newSQLiteCache(db *sql.DB) (*SQLiteCache, error) {
	if _, err := db.Exec(createCacheTableSQL); err != nil {
		db.Close()
		return nil, err
	}
	return &SQLiteCache{db: db}, nil
}

func (s *SQLiteCache) Get(key string) ([]byte, error) {
	var value []byte
	err := s.db.QueryRow("SELECT value FROM cache WHERE key = ?", key).Scan(&value)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return value, nil
}

func (s *SQLiteCache) Set(key string, value []byte) error {
	_, err := s.db.Exec(
		"INSERT INTO cache (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value",
		key, value,
	)
	return err
}

func (s *SQLiteCache) Delete(key string) error {
	_, err := s.db.Exec("DELETE FROM cache WHERE key = ?", key)
	return err
}

func (s *SQLiteCache) Clear() error {
	_, err := s.db.Exec("DELETE FROM cache")
	return err
}

func (s *SQLiteCache) Close() error {
	return s.db.Close()
}
