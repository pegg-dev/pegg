package llm

import (
	"context"
	"testing"
	"time"

	json "github.com/goccy/go-json"

	"github.com/peggco/pegg/internal/core/cache"
	"github.com/peggco/pegg/internal/core/config"
	"github.com/peggco/pegg/internal/core/event"
	"github.com/peggco/pegg/internal/core/logger"
)

type discardHandler struct{}

func (discardHandler) Write(logger.Record) error { return nil }
func (discardHandler) Close() error              { return nil }

func testLogger() *logger.Logger {
	return logger.New(logger.LevelDebug, discardHandler{})
}

func newTestCache(t *testing.T) cache.Cache {
	t.Helper()
	t.Setenv("HOME", t.TempDir())

	c, err := cache.NewJSONCache()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })
	return c
}

func newTestManager(t *testing.T) (*Manager, event.Bus) {
	t.Helper()
	bus := event.New()
	mgr := NewManager(bus, testLogger(), newTestCache(t))
	if err := mgr.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(mgr.Shutdown)
	return mgr, bus
}

func newTestManagerWithCache(t *testing.T) (*Manager, event.Bus, cache.Cache) {
	t.Helper()
	bus := event.New()
	c := newTestCache(t)
	mgr := NewManager(bus, testLogger(), c)
	if err := mgr.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(mgr.Shutdown)
	return mgr, bus, c
}

func registerMockProvider(t *testing.T, name string, models []Model) {
	t.Helper()
	RegisterProvider(name, func(cfg config.LLMConfig) (Provider, error) {
		return &namedMockProvider{name: cfg.Provider, models: models}, nil
	})
}

func TestManagerSyncAndExactSelect(t *testing.T) {
	mgr, _ := newTestManager(t)
	registerMockProvider(t, "mgr-sync-a", []Model{{ID: "m1"}, {ID: "m2"}})

	mgr.Sync(context.Background(), []config.LLMConfig{{Provider: "mgr-sync-a"}})

	models, err := mgr.Models("mgr-sync-a")
	if err != nil || len(models) != 2 {
		t.Fatalf("models = %+v, err = %v", models, err)
	}

	res := mgr.Select(SelectRequest{Mode: SelectModeExact, Provider: "mgr-sync-a", Model: "m2"})
	if res.Err != nil || res.Provider != "mgr-sync-a" || res.Model.ID != "m2" {
		t.Fatalf("unexpected result: %+v", res)
	}
}

func TestManagerExactGlobal(t *testing.T) {
	mgr, _ := newTestManager(t)
	registerMockProvider(t, "mgr-glob-a", []Model{{ID: "a1"}})
	registerMockProvider(t, "mgr-glob-b", []Model{{ID: "b1"}})
	mgr.Sync(context.Background(), []config.LLMConfig{
		{Provider: "mgr-glob-a"},
		{Provider: "mgr-glob-b"},
	})

	res := mgr.Select(SelectRequest{Mode: SelectModeExact, Model: "b1"})
	if res.Err != nil || res.Provider != "mgr-glob-b" || res.Model.ID != "b1" {
		t.Fatalf("unexpected result: %+v", res)
	}
}

func TestManagerExactNotFound(t *testing.T) {
	mgr, _ := newTestManager(t)
	registerMockProvider(t, "mgr-miss-a", []Model{{ID: "m1"}})
	mgr.Sync(context.Background(), []config.LLMConfig{{Provider: "mgr-miss-a"}})

	res := mgr.Select(SelectRequest{Mode: SelectModeExact, Provider: "mgr-miss-a", Model: "nope"})
	if res.Err == nil {
		t.Fatal("expected error for missing model")
	}
}

func TestManagerPreferredFallback(t *testing.T) {
	mgr, _ := newTestManager(t)
	registerMockProvider(t, "mgr-pref-a", []Model{{ID: "first"}, {ID: "second"}})
	mgr.Sync(context.Background(), []config.LLMConfig{{Provider: "mgr-pref-a"}})

	res := mgr.Select(SelectRequest{Mode: SelectModePreferred, Provider: "mgr-pref-a"})
	if res.Err != nil || res.Model.ID != "first" {
		t.Fatalf("unexpected result: %+v", res)
	}
}

func TestManagerPreferredFreeModel(t *testing.T) {
	mgr, _ := newTestManager(t)
	registerMockProvider(t, "mgr-free-a", []Model{{ID: "paid-model"}, {ID: "free-model"}})
	mgr.Sync(context.Background(), []config.LLMConfig{{Provider: "mgr-free-a"}})

	res := mgr.Select(SelectRequest{Mode: SelectModePreferred})
	if res.Err != nil || res.Model.ID != "free-model" {
		t.Fatalf("unexpected result: %+v", res)
	}
}

func TestManagerPreferredFreeModelAcrossProviders(t *testing.T) {
	mgr, _ := newTestManager(t)
	registerMockProvider(t, "mgr-fp-a", []Model{{ID: "a1"}})
	registerMockProvider(t, "mgr-fp-b", []Model{{ID: "b1"}, {ID: "b2", Name: "Some Free Tier"}})
	mgr.Sync(context.Background(), []config.LLMConfig{
		{Provider: "mgr-fp-a"},
		{Provider: "mgr-fp-b"},
	})

	res := mgr.Select(SelectRequest{Mode: SelectModePreferred})
	if res.Err != nil || res.Provider != "mgr-fp-b" || res.Model.ID != "b2" {
		t.Fatalf("unexpected result: %+v", res)
	}
}

func TestManagerPreferredSessionResolver(t *testing.T) {
	mgr, _ := newTestManager(t)
	registerMockProvider(t, "mgr-res-a", []Model{{ID: "a1"}, {ID: "a2"}})
	registerMockProvider(t, "mgr-res-b", []Model{{ID: "b1"}})
	mgr.Sync(context.Background(), []config.LLMConfig{
		{Provider: "mgr-res-a"},
		{Provider: "mgr-res-b"},
	})

	mgr.SetSessionResolver(func() (string, string, bool) {
		return "mgr-res-a", "a2", true
	})

	res := mgr.Select(SelectRequest{Mode: SelectModePreferred})
	if res.Err != nil || res.Provider != "mgr-res-a" || res.Model.ID != "a2" {
		t.Fatalf("unexpected result: %+v", res)
	}
}

func TestManagerPreferredResolverFallbackOnMiss(t *testing.T) {
	mgr, _ := newTestManager(t)
	registerMockProvider(t, "mgr-miss-a", []Model{{ID: "m1"}})
	mgr.Sync(context.Background(), []config.LLMConfig{{Provider: "mgr-miss-a"}})

	mgr.SetSessionResolver(func() (string, string, bool) {
		return "gone-provider", "gone-model", true
	})

	res := mgr.Select(SelectRequest{Mode: SelectModePreferred})
	if res.Err != nil || res.Provider != "mgr-miss-a" || res.Model.ID != "m1" {
		t.Fatalf("unexpected result: %+v", res)
	}
}

func TestManagerPreferredEmptyCache(t *testing.T) {
	mgr, _ := newTestManager(t)
	mgr.SetSessionResolver(func() (string, string, bool) { return "", "", false })
	res := mgr.Select(SelectRequest{Mode: SelectModePreferred})
	if res.Err == nil {
		t.Fatal("expected error for empty cache")
	}
}

func TestManagerSkipsModelCache(t *testing.T) {
	mgr, _, _ := newTestManagerWithCache(t)
	p := &skipCacheProvider{name: "mgr-skip", models: []Model{{ID: "old"}}}
	RegisterProvider("mgr-skip", func(config.LLMConfig) (Provider, error) { return p, nil })

	mgr.Sync(context.Background(), []config.LLMConfig{{Provider: "mgr-skip"}})
	if models, _ := mgr.Models("mgr-skip"); len(models) != 1 || models[0].ID != "old" {
		t.Fatalf("models = %+v", models)
	}

	p.models = []Model{{ID: "new1"}, {ID: "new2"}}
	mgr.Sync(context.Background(), []config.LLMConfig{{Provider: "mgr-skip"}})
	models, _ := mgr.Models("mgr-skip")
	if len(models) != 2 || models[0].ID != "new1" {
		t.Fatalf("models after change = %+v, want the fresh list", models)
	}
}

type skipCacheProvider struct {
	name   string
	models []Model
}

func (m *skipCacheProvider) Name() string { return m.name }
func (m *skipCacheProvider) Chat(context.Context, *Request) (*Response, error) {
	return &Response{}, nil
}
func (m *skipCacheProvider) ChatStream(context.Context, *Request, StreamHandler) error {
	return nil
}
func (m *skipCacheProvider) ListModels(context.Context) ([]Model, error) { return m.models, nil }
func (m *skipCacheProvider) SkipModelCache() bool                        { return true }

func TestManagerProviderRemoved(t *testing.T) {
	mgr, _ := newTestManager(t)
	registerMockProvider(t, "mgr-rm-a", []Model{{ID: "m1"}})
	mgr.Sync(context.Background(), []config.LLMConfig{{Provider: "mgr-rm-a"}})

	mgr.handleProviderRemoved("mgr-rm-a")
	if _, err := mgr.Models("mgr-rm-a"); err == nil {
		t.Fatal("expected error after removal")
	}
}

func TestManagerAppMountedSync(t *testing.T) {
	mgr, bus := newTestManager(t)
	registerMockProvider(t, "mgr-mount-a", []Model{{ID: "m1"}})

	bus.Publish(event.TopicAppMounted, &config.Config{
		Providers: []config.LLMConfig{{Provider: "mgr-mount-a"}},
	})

	models, err := mgr.Models("mgr-mount-a")
	if err != nil || len(models) != 1 || models[0].ID != "m1" {
		t.Fatalf("models = %+v, err = %v", models, err)
	}
}

func TestManagerModelSelectEvent(t *testing.T) {
	mgr, bus := newTestManager(t)
	registerMockProvider(t, "mgr-ev-a", []Model{{ID: "m1"}, {ID: "m2"}})
	mgr.Sync(context.Background(), []config.LLMConfig{{Provider: "mgr-ev-a"}})

	reply := make(chan SelectResult, 1)
	if err := bus.SubscribeOnce("reply.test", func(res SelectResult) { reply <- res }); err != nil {
		t.Fatal(err)
	}

	bus.Publish(event.TopicModelSelect, SelectRequest{
		Provider:   "mgr-ev-a",
		Model:      "m1",
		Mode:       SelectModeExact,
		ReplyTopic: "reply.test",
	})

	select {
	case res := <-reply:
		if res.Err != nil || res.Provider != "mgr-ev-a" || res.Model.ID != "m1" {
			t.Fatalf("unexpected result: %+v", res)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for model select reply")
	}
}

func TestManagerProviderAddedEvent(t *testing.T) {
	mgr, bus := newTestManager(t)
	registerMockProvider(t, "mgr-add-a", []Model{{ID: "m1"}})

	loaded := make(chan ModelsLoaded, 1)
	if err := bus.Subscribe(event.TopicModelsLoaded, func(ml ModelsLoaded) { loaded <- ml }); err != nil {
		t.Fatal(err)
	}

	bus.Publish(event.TopicProviderAdded, config.LLMConfig{Provider: "mgr-add-a"})

	select {
	case ml := <-loaded:
		if ml.Provider != "mgr-add-a" || ml.Count != 1 || ml.Err != nil {
			t.Fatalf("unexpected models loaded: %+v", ml)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for models loaded")
	}

	models, err := mgr.Models("mgr-add-a")
	if err != nil || len(models) != 1 {
		t.Fatalf("models = %+v, err = %v", models, err)
	}
}

func TestManagerLoadError(t *testing.T) {
	mgr, _ := newTestManager(t)
	RegisterProvider("mgr-err-a", func(config.LLMConfig) (Provider, error) {
		return &errMockProvider{}, nil
	})
	mgr.Sync(context.Background(), []config.LLMConfig{{Provider: "mgr-err-a"}})

	if _, err := mgr.Models("mgr-err-a"); err != nil {
		t.Fatalf("entry should exist despite load error: %v", err)
	}

	res := mgr.Select(SelectRequest{Mode: SelectModeExact, Provider: "mgr-err-a", Model: "x"})
	if res.Err == nil {
		t.Fatal("expected error for empty cache")
	}
}

func TestManagerAccessorsMissing(t *testing.T) {
	mgr, _ := newTestManager(t)
	if _, err := mgr.Provider("missing"); err == nil {
		t.Fatal("expected error for missing provider")
	}
	if _, err := mgr.Models("missing"); err == nil {
		t.Fatal("expected error for missing provider")
	}
}

func TestManagerProviderAccessor(t *testing.T) {
	mgr, _ := newTestManager(t)
	registerMockProvider(t, "mgr-prov-a", []Model{{ID: "m1"}})
	mgr.Sync(context.Background(), []config.LLMConfig{{Provider: "mgr-prov-a"}})

	p, err := mgr.Provider("mgr-prov-a")
	if err != nil {
		t.Fatal(err)
	}
	if p.Name() != "mgr-prov-a" {
		t.Fatalf("Name = %q, want mgr-prov-a", p.Name())
	}
}

type errMockProvider struct{}

func (e *errMockProvider) Name() string { return "err" }
func (e *errMockProvider) Chat(context.Context, *Request) (*Response, error) {
	return &Response{}, nil
}
func (e *errMockProvider) ChatStream(context.Context, *Request, StreamHandler) error {
	return nil
}
func (e *errMockProvider) ListModels(context.Context) ([]Model, error) {
	return nil, errModuleTest
}

type recordingProvider struct {
	name      string
	models    []Model
	callCount int
}

func (r *recordingProvider) Name() string { return r.name }
func (r *recordingProvider) Chat(context.Context, *Request) (*Response, error) {
	return &Response{}, nil
}
func (r *recordingProvider) ChatStream(context.Context, *Request, StreamHandler) error {
	return nil
}
func (r *recordingProvider) ListModels(context.Context) ([]Model, error) {
	r.callCount++
	return r.models, nil
}

func TestManagerSavesModelsToCache(t *testing.T) {
	mgr, _, c := newTestManagerWithCache(t)
	registerMockProvider(t, "cache-save-a", []Model{{ID: "m1"}, {ID: "m2"}})

	mgr.Sync(context.Background(), []config.LLMConfig{{Provider: "cache-save-a"}})

	raw, err := c.Get("cache-save-a")
	if err != nil {
		t.Fatalf("cache miss for provider: %v", err)
	}
	var models []Model
	if err := json.Unmarshal(raw, &models); err != nil {
		t.Fatal(err)
	}
	if len(models) != 2 {
		t.Fatalf("cached models = %+v, want 2", models)
	}
}

func TestManagerCacheByProvider(t *testing.T) {
	mgr, _, c := newTestManagerWithCache(t)
	registerMockProvider(t, "cache-pa", []Model{{ID: "shared"}})
	registerMockProvider(t, "cache-pb", []Model{{ID: "shared"}})

	mgr.Sync(context.Background(), []config.LLMConfig{
		{Provider: "cache-pa"},
		{Provider: "cache-pb"},
	})

	for _, p := range []string{"cache-pa", "cache-pb"} {
		if _, err := c.Get(p); err != nil {
			t.Fatalf("provider %q not cached: %v", p, err)
		}
	}
}

func TestManagerLoadFromCacheSkipsRequest(t *testing.T) {
	mgr, _, c := newTestManagerWithCache(t)

	data, _ := json.Marshal([]Model{{ID: "cached-1"}})
	if err := c.Set("cached-prov", data); err != nil {
		t.Fatal(err)
	}

	rec := &recordingProvider{name: "cached-prov", models: []Model{{ID: "fresh-1"}}}
	RegisterProvider("cached-prov", func(config.LLMConfig) (Provider, error) {
		return rec, nil
	})

	mgr.Sync(context.Background(), []config.LLMConfig{{Provider: "cached-prov"}})

	if rec.callCount != 0 {
		t.Fatalf("ListModels called %d times, want 0 (cache hit)", rec.callCount)
	}
	models, err := mgr.Models("cached-prov")
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 1 || models[0].ID != "cached-1" {
		t.Fatalf("models = %+v, want cached-1", models)
	}
}

func TestManagerProviderAddedUsesCacheAfterFirstLoad(t *testing.T) {
	mgr, _, c := newTestManagerWithCache(t)

	rec := &recordingProvider{name: "added-cache-a", models: []Model{{ID: "m1"}}}
	RegisterProvider("added-cache-a", func(config.LLMConfig) (Provider, error) {
		return rec, nil
	})

	trigger := func() {
		loaded := make(chan ModelsLoaded, 1)
		if err := mgr.bus.SubscribeOnce(event.TopicModelsLoaded, func(ml ModelsLoaded) { loaded <- ml }); err != nil {
			t.Fatal(err)
		}
		mgr.bus.Publish(event.TopicProviderAdded, config.LLMConfig{Provider: "added-cache-a"})
		select {
		case ml := <-loaded:
			if ml.Err != nil {
				t.Fatal(ml.Err)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("timed out waiting for models loaded")
		}
	}

	trigger()
	if rec.callCount != 1 {
		t.Fatalf("ListModels calls after first add = %d, want 1", rec.callCount)
	}

	trigger()
	if rec.callCount != 1 {
		t.Fatalf("ListModels calls after second add = %d, want still 1 (cache hit)", rec.callCount)
	}
	if _, err := c.Get("added-cache-a"); err != nil {
		t.Fatalf("models not cached: %v", err)
	}
}

func TestManagerProviderAddedError(t *testing.T) {
	mgr, _, _ := newTestManagerWithCache(t)
	RegisterProvider("added-err-a", func(config.LLMConfig) (Provider, error) {
		return &errMockProvider{}, nil
	})

	loaded := make(chan ModelsLoaded, 1)
	if err := mgr.bus.SubscribeOnce(event.TopicModelsLoaded, func(ml ModelsLoaded) { loaded <- ml }); err != nil {
		t.Fatal(err)
	}

	mgr.bus.Publish(event.TopicProviderAdded, config.LLMConfig{Provider: "added-err-a"})

	select {
	case ml := <-loaded:
		if ml.Err == nil {
			t.Fatal("expected models loaded error")
		}
		if ml.Err != errModuleTest {
			t.Fatalf("got %v, want errModuleTest", ml.Err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for models loaded")
	}
}

func TestManagerProviderRemovedClearsCache(t *testing.T) {
	mgr, _, c := newTestManagerWithCache(t)
	registerMockProvider(t, "cache-rm-a", []Model{{ID: "m1"}})
	mgr.Sync(context.Background(), []config.LLMConfig{{Provider: "cache-rm-a"}})

	mgr.handleProviderRemoved("cache-rm-a")

	if _, err := c.Get("cache-rm-a"); err != cache.ErrNotFound {
		t.Fatalf("cache not cleared: %v", err)
	}
}

func TestManagerShutdownUnsubscribes(t *testing.T) {
	bus := event.New()
	mgr := NewManager(bus, testLogger(), newTestCache(t))
	if err := mgr.Start(); err != nil {
		t.Fatal(err)
	}
	mgr.Shutdown()

	registerMockProvider(t, "shutdown-a", []Model{{ID: "m1"}})
	bus.Publish(event.TopicAppMounted, &config.Config{
		Providers: []config.LLMConfig{{Provider: "shutdown-a"}},
	})

	if _, err := mgr.Models("shutdown-a"); err == nil {
		t.Fatal("handler still subscribed after shutdown")
	}
}
