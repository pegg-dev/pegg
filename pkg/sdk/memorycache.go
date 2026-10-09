package sdk

import (
	"errors"
	"sync"

	"github.com/peggco/pegg/internal/core/cache"
)

var errCacheClosed = errors.New("cache: closed")

type MemoryCache struct {
	mu     sync.RWMutex
	m      map[string][]byte
	closed bool
}

var _ cache.Cache = (*MemoryCache)(nil)

func NewMemoryCache() *MemoryCache {
	return &MemoryCache{m: make(map[string][]byte)}
}

func (c *MemoryCache) Get(key string) ([]byte, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.closed {
		return nil, cache.ErrNotFound
	}
	data, ok := c.m[key]
	if !ok {
		return nil, cache.ErrNotFound
	}
	out := make([]byte, len(data))
	copy(out, data)
	return out, nil
}

func (c *MemoryCache) Set(key string, value []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return errCacheClosed
	}
	data := make([]byte, len(value))
	copy(data, value)
	c.m[key] = data
	return nil
}

func (c *MemoryCache) Delete(key string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return errCacheClosed
	}
	delete(c.m, key)
	return nil
}

func (c *MemoryCache) Clear() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return errCacheClosed
	}
	c.m = make(map[string][]byte)
	return nil
}

func (c *MemoryCache) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closed = true
	return nil
}
