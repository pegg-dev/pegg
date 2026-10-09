package acp

import (
	"testing"

	"github.com/peggco/pegg/internal/core/cache"
	"github.com/peggco/pegg/internal/core/config"
	"github.com/peggco/pegg/internal/core/event"
	"github.com/peggco/pegg/internal/core/logger"
	"github.com/peggco/pegg/internal/llm"
	"github.com/peggco/pegg/internal/router"
	"github.com/peggco/pegg/internal/session"
)

type acpDiscardHandler struct{}

func (acpDiscardHandler) Write(logger.Record) error { return nil }
func (acpDiscardHandler) Close() error              { return nil }

func TestResolveModelSmartRouterThroughManager(t *testing.T) {
	bus := event.New()
	log := logger.New(logger.LevelError, acpDiscardHandler{})
	store, err := cache.NewJSONCache()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })

	mgr := llm.NewManager(bus, log, store)
	if err := mgr.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(mgr.Shutdown)

	cfg := config.DefaultConfig()
	cfg.SmartRouter.Enabled = true
	bus.Publish(event.TopicAppMounted, cfg)
	mgr.WaitUntilReady()
	_ = router.New(router.Deps{Config: cfg, LLM: mgr}, log)

	sessMgr := session.NewManager(newMockStore(), bus, log)
	s := New(cfg, bus, log, nil, sessMgr, mgr)
	s.newAgentFn = mockAgentFn()

	prov, mdl, err := s.resolveModel("", router.SmartRouterModel)
	if err != nil {
		t.Fatal(err)
	}
	if prov.Name() != router.SmartRouterModel || mdl.ID != router.SmartRouterModel {
		t.Fatalf("got %q/%q, want smart-router marker", prov.Name(), mdl.ID)
	}
}

func TestResolveModelUnavailableManager(t *testing.T) {
	s := newTestServer(t)
	if _, _, err := s.resolveModel("", "any"); err == nil {
		t.Fatal("expected ErrModelUnavailable without an llm manager")
	}
}
