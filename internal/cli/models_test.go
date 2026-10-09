package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/peggco/pegg/internal/core/cache"
	"github.com/peggco/pegg/internal/core/config"
	"github.com/peggco/pegg/internal/core/event"
	"github.com/peggco/pegg/internal/core/logger"
	"github.com/peggco/pegg/internal/llm"
	"github.com/peggco/pegg/internal/router"
)

func TestRunModelsListsSmartRouterWhenEnabled(t *testing.T) {
	router.ModelOptionsHook.Reset()
	t.Setenv("HOME", t.TempDir())
	cfg := config.DefaultConfig()
	cfg.SmartRouter.Enabled = true
	cfg.Providers = []config.LLMConfig{{Provider: "openai", APIKey: "sk"}}
	if err := config.Save(cfg); err != nil {
		t.Fatal(err)
	}

	_ = router.New(router.Deps{Config: cfg}, nil)
	c := &CLI{cfg: cfg}

	var out bytes.Buffer
	if err := c.runModels(&out, ""); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "[smart-router]") {
		t.Fatalf("smart router section missing:\n%s", out.String())
	}
	if !strings.Contains(out.String(), "Smart Router") {
		t.Fatalf("smart router entry missing:\n%s", out.String())
	}
}

func TestRunModelsSkipsSmartRouterWhenDisabled(t *testing.T) {
	router.ModelOptionsHook.Reset()
	t.Setenv("HOME", t.TempDir())
	cfg := config.DefaultConfig()
	cfg.SmartRouter.Enabled = false
	cfg.Providers = []config.LLMConfig{{Provider: "openai", APIKey: "sk"}}
	if err := config.Save(cfg); err != nil {
		t.Fatal(err)
	}

	_ = router.New(router.Deps{Config: cfg}, nil)
	c := &CLI{cfg: cfg}

	var out bytes.Buffer
	if err := c.runModels(&out, ""); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "smart-router") {
		t.Fatalf("smart router must not be listed when disabled:\n%s", out.String())
	}
}

func TestAvailableModelsListsSmartRouterWhenEnabled(t *testing.T) {
	router.ModelOptionsHook.Reset()
	t.Setenv("HOME", t.TempDir())
	cfg := config.DefaultConfig()
	cfg.SmartRouter.Enabled = true
	cfg.Providers = nil
	if err := config.Save(cfg); err != nil {
		t.Fatal(err)
	}

	_ = router.New(router.Deps{Config: cfg}, nil)
	bus := event.New()
	c := &CLI{cfg: cfg, bus: bus}

	choices, err := c.availableModels("")
	if err != nil {
		t.Fatal(err)
	}
	if len(choices) == 0 || choices[0].provider != router.SmartRouterModel {
		t.Fatalf("expected smart router first, got %+v", choices)
	}
}

func TestSelectModelResolvesSmartRouterViaManager(t *testing.T) {
	router.ModelOptionsHook.Reset()
	t.Setenv("HOME", t.TempDir())
	cfg := config.DefaultConfig()
	cfg.SmartRouter.Enabled = true
	if err := config.Save(cfg); err != nil {
		t.Fatal(err)
	}

	bus := event.New()
	log := logger.New(logger.LevelError, discardHandler{})
	cacheStore, err := cache.NewJSONCache()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cacheStore.Close() })
	mgr := llm.NewManager(bus, log, cacheStore)
	if err := mgr.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(mgr.Shutdown)
	_ = router.New(router.Deps{Config: cfg, LLM: mgr}, nil)

	c := &CLI{cfg: cfg, bus: bus, llmMgr: mgr}
	prov, mdl, err := c.selectModel("", router.SmartRouterModel, false)
	if err != nil {
		t.Fatal(err)
	}
	if prov.Name() != router.SmartRouterModel {
		t.Fatalf("provider = %q, want smart-router placeholder", prov.Name())
	}
	if mdl.ID != router.SmartRouterModel {
		t.Fatalf("model = %q", mdl.ID)
	}
}

var _ = logger.New
