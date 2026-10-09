package tools

import (
	"context"
	"errors"
	"testing"

	"github.com/peggco/pegg/internal/agent/tool"
)

func TestGlobalRegistry(t *testing.T) {
	if _, ok := Get("test-tool"); ok {
		Unregister("test-tool")
	}
	err := Register(tool.NewSpec("test-tool", "test", nil, func(context.Context, string) (string, error) {
		return "x", nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	defer Unregister("test-tool")

	names := Names()
	found := false
	for _, n := range names {
		if n == "test-tool" {
			found = true
		}
	}
	if !found {
		t.Fatalf("names = %v, want test-tool", names)
	}

	if _, ok := Get("test-tool"); !ok {
		t.Fatal("Get must return registered tool")
	}

	llmtools := LLMTools()
	found = false
	for _, lt := range llmtools {
		if lt.Function.Name == "test-tool" {
			found = true
		}
	}
	if !found {
		t.Fatalf("test-tool missing from LLMTools: %+v", llmtools)
	}
}

func TestRegisterDuplicate(t *testing.T) {
	err := Register(tool.NewSpec("test-tool", "test", nil, func(context.Context, string) (string, error) {
		return "x", nil
	}))
	if err != nil && !errors.Is(err, tool.ErrDuplicate) {
		t.Fatalf("register duplicate: %v", err)
	}
	Unregister("test-tool")
}
