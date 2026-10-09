package notification

import (
	"strings"
	"sync"
	"testing"

	"github.com/peggco/pegg/internal/core/config"
	"github.com/peggco/pegg/internal/core/event"
)

func TestNewFromConfigOS(t *testing.T) {
	ds, err := NewFromConfig(config.NotificationConfig{Drivers: []string{DriverOS}})
	if err != nil {
		t.Fatal(err)
	}
	defer closeAll(ds)

	if len(ds) != 1 {
		t.Fatalf("drivers: got %d, want 1", len(ds))
	}
	if _, ok := ds[0].(*OSDriver); !ok {
		t.Fatalf("driver type: got %T, want *OSDriver", ds[0])
	}
}

func TestNewFromConfigMultiple(t *testing.T) {
	RegisterDriver("multi-a", func(config.NotificationConfig) (Driver, error) {
		return &fakeDriver{}, nil
	})
	RegisterDriver("multi-b", func(config.NotificationConfig) (Driver, error) {
		return &fakeDriver{}, nil
	})

	ds, err := NewFromConfig(config.NotificationConfig{Drivers: []string{DriverOS, "multi-a", "multi-b"}})
	if err != nil {
		t.Fatal(err)
	}
	defer closeAll(ds)

	if len(ds) != 3 {
		t.Fatalf("drivers: got %d, want 3", len(ds))
	}
}

func TestNewFromConfigDedupes(t *testing.T) {
	ds, err := NewFromConfig(config.NotificationConfig{Drivers: []string{DriverOS, DriverOS, "", DriverOS}})
	if err != nil {
		t.Fatal(err)
	}
	defer closeAll(ds)

	if len(ds) != 1 {
		t.Fatalf("drivers: got %d, want 1 after dedupe", len(ds))
	}
}

func TestNewFromConfigUnknownDriver(t *testing.T) {
	_, err := NewFromConfig(config.NotificationConfig{Drivers: []string{"nope"}})
	if err == nil {
		t.Fatal("expected error for unknown driver")
	}
	if !strings.Contains(err.Error(), "no driver registered") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNewFromConfigUnknownInList(t *testing.T) {
	_, err := NewFromConfig(config.NotificationConfig{Drivers: []string{DriverOS, "nope"}})
	if err == nil {
		t.Fatal("expected error for unknown driver in list")
	}
	if !strings.Contains(err.Error(), "nope") {
		t.Fatalf("error should name the bad driver: %v", err)
	}
}

func TestNewFromConfigFactoryError(t *testing.T) {
	RegisterDriver("broken", func(config.NotificationConfig) (Driver, error) {
		return nil, errTestDriver
	})

	_, err := NewFromConfig(config.NotificationConfig{Drivers: []string{"broken"}})
	if err != errTestDriver {
		t.Fatalf("got %v, want errTestDriver", err)
	}
}

func TestNewFromConfigEmpty(t *testing.T) {
	_, err := NewFromConfig(config.NotificationConfig{Drivers: []string{}})
	if err == nil {
		t.Fatal("expected error for empty drivers list")
	}
	if !strings.Contains(err.Error(), "no drivers configured") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRegisterDriverCustom(t *testing.T) {
	RegisterDriver("custom", func(cfg config.NotificationConfig) (Driver, error) {
		return &fakeDriver{}, nil
	})

	ds, err := NewFromConfig(config.NotificationConfig{Drivers: []string{"custom"}})
	if err != nil {
		t.Fatal(err)
	}
	defer closeAll(ds)

	if len(ds) != 1 {
		t.Fatalf("drivers: got %d, want 1", len(ds))
	}
	if _, ok := ds[0].(*fakeDriver); !ok {
		t.Fatalf("driver type: got %T, want *fakeDriver", ds[0])
	}
}

func TestRegisterDriverInvalid(t *testing.T) {
	before := len(drivers)
	RegisterDriver("", func(config.NotificationConfig) (Driver, error) { return &fakeDriver{}, nil })
	RegisterDriver("nil", nil)
	if len(drivers) != before {
		t.Fatalf("drivers: got %d, want %d", len(drivers), before)
	}
}

func TestNotificationModuleDisabled(t *testing.T) {
	n, err := NotificationModule(config.NotificationConfig{Drivers: []string{DriverOS}, Enabled: false}, event.New(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if n != nil {
		t.Fatal("expected nil notifier when disabled")
	}
}

func TestNotificationModuleEmptyDrivers(t *testing.T) {
	_, err := NotificationModule(config.NotificationConfig{Enabled: true}, event.New(), nil)
	if err == nil {
		t.Fatal("expected error for empty drivers list")
	}
}

func TestNotificationModule(t *testing.T) {
	RegisterDriver("fake", func(config.NotificationConfig) (Driver, error) {
		return &fakeDriver{}, nil
	})

	n, err := NotificationModule(config.NotificationConfig{Drivers: []string{"fake"}, Enabled: true}, event.New(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer n.Close()

	if len(n.drivers) != 1 {
		t.Fatalf("drivers: got %d, want 1", len(n.drivers))
	}
	if !n.Focused() {
		t.Fatal("expected notifier to start focused")
	}
	if len(n.handlers) != 6 {
		t.Fatalf("handlers: got %d, want 6", len(n.handlers))
	}
}

func TestNotificationModuleMultipleDrivers(t *testing.T) {
	RegisterDriver("mod-a", func(config.NotificationConfig) (Driver, error) {
		return &fakeDriver{}, nil
	})
	RegisterDriver("mod-b", func(config.NotificationConfig) (Driver, error) {
		return &fakeDriver{}, nil
	})

	n, err := NotificationModule(config.NotificationConfig{Drivers: []string{"mod-a", "mod-b"}, Enabled: true}, event.New(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer n.Close()

	if len(n.drivers) != 2 {
		t.Fatalf("drivers: got %d, want 2", len(n.drivers))
	}
}

func closeAll(ds []Driver) {
	for _, d := range ds {
		_ = d.Close()
	}
}

type fakeDriver struct {
	mu   sync.Mutex
	sent []Notification
	err  error
}

func (d *fakeDriver) Notify(n Notification) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.err != nil {
		return d.err
	}
	d.sent = append(d.sent, n)
	return nil
}

func (d *fakeDriver) Close() error { return nil }

func (d *fakeDriver) notifications() []Notification {
	d.mu.Lock()
	defer d.mu.Unlock()
	out := make([]Notification, len(d.sent))
	copy(out, d.sent)
	return out
}

var errTestDriver = &testDriverError{}

type testDriverError struct{}

func (e *testDriverError) Error() string { return "test driver error" }
