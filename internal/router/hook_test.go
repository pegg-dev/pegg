package router

import (
	"testing"

	"github.com/peggco/pegg/internal/core/config"
)

func TestModelOptionsHookAddsSmartRouterWhenEnabled(t *testing.T) {
	ModelOptionsHook.Reset()
	cfg := config.DefaultConfig()
	cfg.SmartRouter.Enabled = true
	New(Deps{Config: cfg}, nil)

	opts := ModelOptionsHook.Apply(nil)
	if len(opts) != 1 {
		t.Fatalf("opts = %v, want exactly the smart router entry", opts)
	}
	if opts[0].Provider != SmartRouterModel || opts[0].Label != "Smart Router" {
		t.Fatalf("opt = %+v", opts[0])
	}
}

func TestModelOptionsHookSkipsWhenDisabled(t *testing.T) {
	ModelOptionsHook.Reset()
	cfg := config.DefaultConfig()
	cfg.SmartRouter.Enabled = false
	New(Deps{Config: cfg}, nil)

	if opts := ModelOptionsHook.Apply(nil); len(opts) != 0 {
		t.Fatalf("opts = %v, want none when disabled", opts)
	}
}

func TestModelOptionsHookDeduplicates(t *testing.T) {
	ModelOptionsHook.Reset()
	cfg := config.DefaultConfig()
	cfg.SmartRouter.Enabled = true
	New(Deps{Config: cfg}, nil)
	New(Deps{Config: cfg}, nil)

	opts := ModelOptionsHook.Apply(nil)
	if len(opts) != 1 {
		t.Fatalf("opts = %v, want deduplicated single entry", opts)
	}
}

func TestModelOptionsHookPreservesExisting(t *testing.T) {
	ModelOptionsHook.Reset()
	cfg := config.DefaultConfig()
	cfg.SmartRouter.Enabled = true
	New(Deps{Config: cfg}, nil)

	existing := []ModelOption{{Provider: "openai", ModelID: "gpt-4o", Label: "GPT-4o"}}
	opts := ModelOptionsHook.Apply(existing)
	if len(opts) != 2 {
		t.Fatalf("opts = %v, want smart router + existing", opts)
	}
	if opts[0].Provider != SmartRouterModel || opts[1].Provider != "openai" {
		t.Fatalf("order wrong: %+v", opts)
	}
}
