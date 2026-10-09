package cache

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	json "github.com/goccy/go-json"

	"github.com/peggco/pegg/internal/core/config"
)

const DriverJSON = "json"

func init() {
	RegisterDriver(DriverJSON, func(cfg config.CacheConfig) (Cache, error) {
		return NewJSONCache()
	})
}

type JSONCache struct {
	mu   sync.RWMutex
	path string
	data map[string]json.RawMessage
}

func NewJSONCache() (*JSONCache, error) {
	path, err := config.GetConfigPath("cache.json")
	if err != nil {
		return nil, err
	}

	return newJSONCache(path)
}

func newJSONCache(path string) (*JSONCache, error) {
	c := &JSONCache{
		path: path,
		data: make(map[string]json.RawMessage),
	}

	if err := c.load(); err != nil {
		return nil, err
	}
	return c, nil
}

func (c *JSONCache) load() error {
	data, err := os.ReadFile(c.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if len(data) == 0 {
		return nil
	}

	var m map[string]json.RawMessage
	if err := json.Unmarshal(data, &m); err != nil {
		return err
	}
	c.data = m
	return nil
}

func (c *JSONCache) persist() error {
	if err := os.MkdirAll(filepath.Dir(c.path), 0755); err != nil {
		return fmt.Errorf("cache: create data directory: %w", err)
	}

	data, err := json.Marshal(c.data)
	if err != nil {
		return err
	}

	return os.WriteFile(c.path, data, 0644)
}

func (c *JSONCache) Get(key string) ([]byte, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	v, ok := c.data[key]
	if !ok {
		return nil, ErrNotFound
	}
	return v, nil
}

func (c *JSONCache) Set(key string, value []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.data[key] = json.RawMessage(value)
	return c.persist()
}

func (c *JSONCache) Delete(key string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.data, key)
	return c.persist()
}

func (c *JSONCache) Clear() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.data = make(map[string]json.RawMessage)
	return c.persist()
}

func (c *JSONCache) Close() error {
	return nil
}
