package logger

import (
	"strings"
	"testing"

	"github.com/peggco/pegg/internal/core/config"
)

func TestNewFromConfigConsole(t *testing.T) {
	l, err := NewFromConfig(config.LoggerConfig{Driver: DriverConsole})
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()

	if len(l.handlers) != 1 {
		t.Fatalf("handlers: got %d, want 1", len(l.handlers))
	}
	if _, ok := l.handlers[0].(*ConsoleHandler); !ok {
		t.Fatalf("handler type: got %T, want *ConsoleHandler", l.handlers[0])
	}
}

func TestNewFromConfigSQLite(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	l, err := NewFromConfig(config.LoggerConfig{Driver: DriverSQLite, MaxLogCount: 100})
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()

	if len(l.handlers) != 1 {
		t.Fatalf("handlers: got %d, want 1", len(l.handlers))
	}
	if _, ok := l.handlers[0].(*SQLiteHandler); !ok {
		t.Fatalf("handler type: got %T, want *SQLiteHandler", l.handlers[0])
	}
}

func TestNewFromConfigUnknownDriver(t *testing.T) {
	_, err := NewFromConfig(config.LoggerConfig{Driver: "nope"})
	if err == nil {
		t.Fatal("expected error for unknown driver")
	}
	if !strings.Contains(err.Error(), "no driver registered") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNewFromConfigFactoryError(t *testing.T) {
	RegisterDriver("broken", func(config.LoggerConfig) (Handler, error) {
		return nil, errTestDriver
	})

	_, err := NewFromConfig(config.LoggerConfig{Driver: "broken"})
	if err != errTestDriver {
		t.Fatalf("got %v, want errTestDriver", err)
	}
}

func TestRegisterDriverCustom(t *testing.T) {
	RegisterDriver("custom", func(cfg config.LoggerConfig) (Handler, error) {
		return NewConsoleHandler(cfg), nil
	})

	l, err := NewFromConfig(config.LoggerConfig{Driver: "custom"})
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()

	if len(l.handlers) != 1 {
		t.Fatalf("handlers: got %d, want 1", len(l.handlers))
	}
}

func TestLoggerModuleEmptyDriver(t *testing.T) {
	_, err := LoggerModule(config.LoggerConfig{})
	if err == nil {
		t.Fatal("expected error for empty driver name")
	}
}

var errTestDriver = &testDriverError{}

type testDriverError struct{}

func (e *testDriverError) Error() string { return "test driver error" }
