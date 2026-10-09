package session

import (
	"errors"
	"testing"

	"github.com/peggco/pegg/internal/core/config"
	"github.com/peggco/pegg/internal/core/event"
	"github.com/peggco/pegg/internal/core/logger"
)

func TestSessionModule(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	bus := event.New()
	log := logger.New(logger.LevelDebug, discardHandler{})

	mgr, err := SessionModule(config.SessionConfig{Driver: DriverJSON}, bus, log)
	if err != nil {
		t.Fatal(err)
	}
	s, err := mgr.Create(CreateOptions{Title: "t"})
	if err != nil {
		t.Fatal(err)
	}
	if s.ID == "" {
		t.Fatal("session id empty")
	}
	if err := mgr.Close(); err != nil {
		t.Fatal(err)
	}

	if _, err := SessionModule(config.SessionConfig{}, bus, log); err == nil {
		t.Fatal("want error for empty driver")
	}
	if _, err := SessionModule(config.SessionConfig{Driver: "nope"}, bus, log); err == nil {
		t.Fatal("want error for unknown driver")
	}
}

func TestRegisterDriver(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	RegisterDriver("test-driver", func(cfg config.SessionConfig) (Store, error) {
		return newJSONStore(t.TempDir())
	})
	store, err := NewFromConfig(config.SessionConfig{Driver: "test-driver"})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	if _, err := NewFromConfig(config.SessionConfig{Driver: "missing"}); err == nil {
		t.Fatal("want error for unknown driver")
	}

	RegisterDriver("", func(config.SessionConfig) (Store, error) { return nil, nil })
	RegisterDriver("test-driver", nil)
	if _, err := NewFromConfig(config.SessionConfig{Driver: "test-driver"}); err != nil {
		t.Fatalf("driver must remain registered: %v", err)
	}
}

func TestSessionErrors(t *testing.T) {
	if !errors.Is(ErrNotFound, ErrNotFound) {
		t.Fatal("ErrNotFound must be comparable via errors.Is")
	}
}
