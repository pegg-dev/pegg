package cache

import (
	"strings"
	"testing"

	"github.com/peggco/pegg/internal/core/config"
)

func TestNewFromConfigJSON(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	c, err := NewFromConfig(config.CacheConfig{Driver: DriverJSON})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	if _, ok := c.(*JSONCache); !ok {
		t.Fatalf("type: got %T, want *JSONCache", c)
	}
}

func TestNewFromConfigSQLite(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	c, err := NewFromConfig(config.CacheConfig{Driver: DriverSQLite})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	if _, ok := c.(*SQLiteCache); !ok {
		t.Fatalf("type: got %T, want *SQLiteCache", c)
	}
}

func TestNewFromConfigUnknownDriver(t *testing.T) {
	_, err := NewFromConfig(config.CacheConfig{Driver: "nope"})
	if err == nil {
		t.Fatal("expected error for unknown driver")
	}
	if !strings.Contains(err.Error(), "no driver registered") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNewFromConfigFactoryError(t *testing.T) {
	RegisterDriver("broken", func(config.CacheConfig) (Cache, error) {
		return nil, errTestDriver
	})

	_, err := NewFromConfig(config.CacheConfig{Driver: "broken"})
	if err != errTestDriver {
		t.Fatalf("got %v, want errTestDriver", err)
	}
}

func TestRegisterDriverCustom(t *testing.T) {
	RegisterDriver("custom", func(cfg config.CacheConfig) (Cache, error) {
		return NewJSONCache()
	})

	c, err := NewFromConfig(config.CacheConfig{Driver: "custom"})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
}

func TestCacheModuleEmptyDriver(t *testing.T) {
	if _, err := CacheModule(config.CacheConfig{}); err == nil {
		t.Fatal("expected error for empty driver name")
	}
}

var errTestDriver = &testDriverError{}

type testDriverError struct{}

func (e *testDriverError) Error() string { return "test driver error" }
