package agents

import (
	"errors"
	"testing"

	"github.com/peggco/pegg/internal/agent"
)

func TestRegistry(t *testing.T) {
	if err := Register(func() (*agent.Agent, error) {
		return agent.New("test-agent"), nil
	}); err != nil {
		t.Fatal(err)
	}
	if !Has("test-agent") {
		t.Fatal("test-agent must be registered")
	}
	found := false
	for _, n := range List() {
		if n == "test-agent" {
			found = true
		}
	}
	if !found {
		t.Fatalf("List = %v", List())
	}

	a, err := New("test-agent")
	if err != nil {
		t.Fatal(err)
	}
	if a.Name != "test-agent" {
		t.Fatalf("name = %q", a.Name)
	}

	if err := Register(nil); !errors.Is(err, ErrNilFactory) {
		t.Fatalf("err = %v, want ErrNilFactory", err)
	}
	if err := Register(func() (*agent.Agent, error) { return nil, nil }); !errors.Is(err, ErrNilAgent) {
		t.Fatalf("err = %v, want ErrNilAgent", err)
	}
	if err := Register(func() (*agent.Agent, error) { return &agent.Agent{}, nil }); !errors.Is(err, ErrEmptyName) {
		t.Fatalf("err = %v, want ErrEmptyName", err)
	}
	if Has("") {
		t.Fatal("empty name must not register")
	}

	if _, err := New("missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestRegisterDuplicate(t *testing.T) {
	if err := Register(func() (*agent.Agent, error) {
		return agent.New("dup-agent"), nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := Register(func() (*agent.Agent, error) {
		return agent.New("dup-agent"), nil
	}); !errors.Is(err, ErrDuplicate) {
		t.Fatalf("err = %v, want ErrDuplicate", err)
	}
}
