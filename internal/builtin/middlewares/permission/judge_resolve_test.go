package permission

import (
	"context"
	"errors"
	"testing"

	"github.com/peggco/pegg/internal/core/cache"
	"github.com/peggco/pegg/internal/core/config"
	"github.com/peggco/pegg/internal/core/event"
	"github.com/peggco/pegg/internal/core/logger"
	"github.com/peggco/pegg/internal/llm"
)

type judgeMockProvider struct {
	name   string
	models []llm.Model
}

func (p *judgeMockProvider) Name() string { return p.name }
func (p *judgeMockProvider) Chat(context.Context, *llm.Request) (*llm.Response, error) {
	return nil, errors.New("not used")
}
func (p *judgeMockProvider) ChatStream(context.Context, *llm.Request, llm.StreamHandler) error {
	return errors.New("not used")
}
func (p *judgeMockProvider) ListModels(context.Context) ([]llm.Model, error) {
	return p.models, nil
}

func newJudgeTestManager(t *testing.T) (*llm.Manager, event.Bus) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	store, err := cache.NewJSONCache()
	if err != nil {
		t.Fatal(err)
	}
	bus := event.New()
	mgr := llm.NewManager(bus, logger.New(logger.LevelError, discardHandler{}), store)
	if err := mgr.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(mgr.Shutdown)
	return mgr, bus
}

func syncJudgeProvider(t *testing.T, mgr *llm.Manager, bus event.Bus, name string, models []llm.Model) {
	t.Helper()
	llm.RegisterProvider(name, func(cfg config.LLMConfig) (llm.Provider, error) {
		return &judgeMockProvider{name: cfg.Provider, models: models}, nil
	})
	bus.Publish(event.TopicAppMounted, &config.Config{
		Providers: []config.LLMConfig{{Provider: name}},
	})
	mgr.WaitUntilReady()
}

func TestResolveJudgeExplicitConfig(t *testing.T) {
	mgr, bus := newJudgeTestManager(t)
	syncJudgeProvider(t, mgr, bus, "judge-prov", []llm.Model{{ID: "judge-model"}})

	m := &Middleware{llm: mgr, cfg: &config.PermissionConfig{
		JudgeProvider: "judge-prov",
		JudgeModel:    "judge-model",
	}}
	prov, model, ok := m.resolveJudge()
	if !ok {
		t.Fatal("expected resolution to succeed")
	}
	if prov.Name() != "judge-prov" || model.ID != "judge-model" {
		t.Fatalf("provider = %q model = %q", prov.Name(), model.ID)
	}
}

func TestResolveJudgeFallsBackToSessionModel(t *testing.T) {
	mgr, bus := newJudgeTestManager(t)
	syncJudgeProvider(t, mgr, bus, "judge-sess", []llm.Model{{ID: "sess-model"}})
	mgr.SetSessionResolver(func() (string, string, bool) {
		return "judge-sess", "sess-model", true
	})

	m := &Middleware{llm: mgr}
	prov, model, ok := m.resolveJudge()
	if !ok {
		t.Fatal("expected resolution to succeed")
	}
	if prov.Name() != "judge-sess" || model.ID != "sess-model" {
		t.Fatalf("provider = %q model = %q, want judge-sess/sess-model", prov.Name(), model.ID)
	}
}

func TestResolveJudgeFallsBackToFirstProvider(t *testing.T) {
	mgr, bus := newJudgeTestManager(t)
	syncJudgeProvider(t, mgr, bus, "judge-first", []llm.Model{{ID: "any-model"}})

	m := &Middleware{llm: mgr}
	prov, model, ok := m.resolveJudge()
	if !ok {
		t.Fatal("expected resolution to succeed")
	}
	if prov.Name() != "judge-first" || model.ID != "any-model" {
		t.Fatalf("provider = %q model = %q, want judge-first/any-model", prov.Name(), model.ID)
	}
}

func TestResolveJudgeUnavailable(t *testing.T) {
	mgr, _ := newJudgeTestManager(t)
	m := &Middleware{llm: mgr}
	if _, _, ok := m.resolveJudge(); ok {
		t.Fatal("expected resolution to fail without providers")
	}
}

func TestResolveJudgeMissingModelFallsBackToPreferred(t *testing.T) {
	mgr, bus := newJudgeTestManager(t)
	syncJudgeProvider(t, mgr, bus, "judge-prov", []llm.Model{{ID: "other"}})

	m := &Middleware{llm: mgr, cfg: &config.PermissionConfig{
		JudgeProvider: "judge-prov",
		JudgeModel:    "missing-model",
	}}
	prov, model, ok := m.resolveJudge()
	if !ok {
		t.Fatal("expected fallback to the preferred model when the exact judge model is unavailable")
	}
	if prov.Name() != "judge-prov" || model.ID != "other" {
		t.Fatalf("provider = %q model = %q, want judge-prov/other", prov.Name(), model.ID)
	}
}

func TestResolveJudgeNoManager(t *testing.T) {
	m := &Middleware{}
	if _, _, ok := m.resolveJudge(); ok {
		t.Fatal("expected resolution to fail without a manager")
	}
}

var _ = errJudgeUnconfigured
var _ = errJudgeModelMissing
