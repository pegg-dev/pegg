package router

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/peggco/pegg/internal/agent"
	"github.com/peggco/pegg/internal/core/config"
	"github.com/peggco/pegg/internal/core/event"
	"github.com/peggco/pegg/internal/core/logger"
	decisionapi "github.com/peggco/pegg/internal/decision"
	"github.com/peggco/pegg/internal/llm"
)

type discardHandler struct{}

func (discardHandler) Write(logger.Record) error { return nil }
func (discardHandler) Close() error              { return nil }

type mockLLMProvider struct {
	name    string
	models  []llm.Model
	output  string
	chatErr error
}

func (p *mockLLMProvider) Name() string { return p.name }
func (p *mockLLMProvider) Chat(context.Context, *llm.Request) (*llm.Response, error) {
	if p.chatErr != nil {
		return nil, p.chatErr
	}
	msg := llm.AssistantMessage(p.output)
	fr := llm.FinishReasonStop
	return &llm.Response{Choices: []llm.Choice{{Message: &msg, FinishReason: &fr}}}, nil
}
func (p *mockLLMProvider) ChatStream(context.Context, *llm.Request, llm.StreamHandler) error {
	return errors.New("not used")
}
func (p *mockLLMProvider) ListModels(context.Context) ([]llm.Model, error) {
	return p.models, nil
}

type mockDecisionProvider struct {
	name   string
	choice string
	conf   float64
	score  float64
	err    error
	gotReq *decisionapi.Request
	reqs   []*decisionapi.Request
}

func (p *mockDecisionProvider) Name() string { return p.name }
func (p *mockDecisionProvider) Decide(_ context.Context, req *decisionapi.Request) (*decisionapi.Response, error) {
	if p.err != nil {
		return nil, p.err
	}
	p.gotReq = req
	p.reqs = append(p.reqs, req)
	conf := p.conf
	answers := map[string]decisionapi.Answer{
		"best_model": {Type: decisionapi.QuestionChoice, Choice: p.choice, Confidence: &conf},
	}
	if _, ok := req.Questions["task_difficulty"]; ok {
		answers["task_difficulty"] = decisionapi.Answer{Type: decisionapi.QuestionScore, Score: &p.score}
	}
	return &decisionapi.Response{
		Answers: answers,
	}, nil
}

func visionModel(id string) llm.Model {
	return llm.Model{
		ID:     id,
		Config: &llm.ModelConfig{MaxInputTokens: 200_000, SupportsReasoning: true, ToolCall: true, Modalities: &llm.Modalities{Input: []string{"image"}}},
	}
}

func plainModel(id string) llm.Model {
	return llm.Model{
		ID:     id,
		Config: &llm.ModelConfig{MaxInputTokens: 16_000, ToolCall: true},
	}
}

func newRouterLLM(t *testing.T, providers map[string][]llm.Model) (*llm.Manager, event.Bus) {
	return newRouterLLMOut(t, providers, "")
}

func newRouterLLMOut(t *testing.T, providers map[string][]llm.Model, output string) (*llm.Manager, event.Bus) {
	t.Helper()
	bus := event.New()
	mgr := llm.NewManager(bus, logger.New(logger.LevelError, discardHandler{}), nil)
	if err := mgr.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(mgr.Shutdown)
	for name, models := range providers {
		models := models
		llm.RegisterProvider(name, func(cfg config.LLMConfig) (llm.Provider, error) {
			return &mockLLMProvider{name: cfg.Provider, models: models, output: output}, nil
		})
	}
	cfgs := make([]config.LLMConfig, 0, len(providers))
	for name := range providers {
		cfgs = append(cfgs, config.LLMConfig{Provider: name, APIKey: "sk"})
	}
	bus.Publish(event.TopicAppMounted, &config.Config{Providers: cfgs})
	mgr.WaitUntilReady()
	return mgr, bus
}

func routerCfg(providers map[string][]llm.Model, agents map[string]config.RouterAgentConfig) *config.Config {
	cfg := config.DefaultConfig()
	cfg.SmartRouter.Enabled = true
	cfg.SmartRouter.Agents = agents
	for name := range providers {
		cfg.Providers = append(cfg.Providers, config.LLMConfig{Provider: name, APIKey: "sk"})
	}
	return cfg
}

func newRouterDecision(t *testing.T, name string, prov decisionapi.Provider) *decisionapi.Manager {
	t.Helper()
	decisionapi.RegisterProvider(name, func(cfg config.LLMConfig) (decisionapi.Provider, error) {
		return prov, nil
	})
	bus := event.New()
	mgr := decisionapi.NewManager(bus, logger.New(logger.LevelError, discardHandler{}))
	if err := mgr.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(mgr.Shutdown)
	bus.Publish(event.TopicAppMounted, &config.Config{
		Providers: []config.LLMConfig{{Provider: name, APIKey: "sk"}},
	})
	return mgr
}

func baseCfg() *config.Config {
	cfg := config.DefaultConfig()
	cfg.SmartRouter.Enabled = true
	return cfg
}

func TestRouterSelectUsesDecisionPreference(t *testing.T) {
	mgr, _ := newRouterLLM(t, map[string][]llm.Model{
		"prov-a": {plainModel("a-1")},
		"prov-b": {plainModel("b-2")},
	})
	dec := &mockDecisionProvider{name: "dec", choice: "prov-b/b-2", conf: 0.9}
	cfg := routerCfg(map[string][]llm.Model{
		"prov-a": {plainModel("a-1")},
		"prov-b": {plainModel("b-2")},
	}, map[string]config.RouterAgentConfig{"developer": {Default: []string{"b-2"}}})

	r := New(Deps{Config: cfg, LLM: mgr, Decision: newRouterDecision(t, "dec", dec)}, nil)
	prov, mdl, err := r.Select(context.Background(), SelectRequest{AgentName: "developer", Task: "fix a bug"})
	if err != nil {
		t.Fatal(err)
	}
	if prov.Name() != "prov-b" || mdl.ID != "b-2" {
		t.Fatalf("got %s/%s, want prov-b/b-2", prov.Name(), mdl.ID)
	}
	if dec.gotReq == nil {
		t.Fatal("no decision request")
	}
	if !strings.Contains(dec.gotReq.State, "developer") {
		t.Errorf("state missing agent: %s", dec.gotReq.State)
	}
}

func TestRouterDecisionStateHasNoModelList(t *testing.T) {
	mgr, _ := newRouterLLM(t, map[string][]llm.Model{
		"prov-a": {plainModel("a-1"), visionModel("vision-1")},
	})
	dec := &mockDecisionProvider{name: "dec", choice: "prov-a/vision-1", conf: 0.9}
	cfg := routerCfg(map[string][]llm.Model{
		"prov-a": {plainModel("a-1"), visionModel("vision-1")},
	}, nil)

	r := New(Deps{Config: cfg, LLM: mgr, Decision: newRouterDecision(t, "dec", dec)}, nil)
	if _, _, err := r.Select(context.Background(), SelectRequest{AgentName: "explorer", Task: "look around", HasImages: true}); err != nil {
		t.Fatal(err)
	}

	if strings.Contains(dec.gotReq.State, "prov-a/a-1") || strings.Contains(dec.gotReq.State, "Available models") {
		t.Fatalf("state must not list models:\n%s", dec.gotReq.State)
	}
	criteria := dec.gotReq.Questions["best_model"].Criteria.(map[string]string)
	if _, ok := criteria["prov-a/vision-1"]; !ok {
		t.Fatalf("options must carry model info: %v", criteria)
	}
	if !strings.Contains(dec.gotReq.State, "Images attached") {
		t.Fatalf("state missing image hint:\n%s", dec.gotReq.State)
	}
}

func TestRouterSelectFiltersByImage(t *testing.T) {
	mgr, _ := newRouterLLM(t, map[string][]llm.Model{
		"prov-a": {visionModel("vision-1"), plainModel("text-2")},
	})
	dec := &mockDecisionProvider{name: "dec", choice: "prov-a/vision-1", conf: 0.95}
	cfg := routerCfg(map[string][]llm.Model{
		"prov-a": {visionModel("vision-1"), plainModel("text-2")},
	}, nil)

	r := New(Deps{Config: cfg, LLM: mgr, Decision: newRouterDecision(t, "dec", dec)}, nil)
	prov, mdl, err := r.Select(context.Background(), SelectRequest{AgentName: "explorer", Task: "describe this image", HasImages: true})
	if err != nil {
		t.Fatal(err)
	}
	if prov.Name() != "prov-a" || mdl.ID != "vision-1" {
		t.Fatalf("got %s/%s", prov.Name(), mdl.ID)
	}
	criteria := dec.gotReq.Questions["best_model"].Criteria.(map[string]string)
	if _, ok := criteria["prov-a/text-2"]; ok {
		t.Error("non-vision model must be excluded when images are attached")
	}
	if !strings.Contains(criteria["prov-a/vision-1"], "vision=yes") {
		t.Errorf("vision model description missing vision flag: %s", criteria["prov-a/vision-1"])
	}
}

func TestRouterSelectAcceptsChoiceRegardlessOfConfidence(t *testing.T) {
	providers := map[string][]llm.Model{
		"prov-a": {plainModel("a-1"), plainModel("a-2")},
	}
	mgr, _ := newRouterLLM(t, providers)
	dec := &mockDecisionProvider{name: "dec", choice: "prov-a/a-2", conf: 0.2}
	cfg := routerCfg(providers, nil)

	r := New(Deps{Config: cfg, LLM: mgr, Decision: newRouterDecision(t, "dec", dec)}, nil)
	prov, mdl, err := r.Select(context.Background(), SelectRequest{AgentName: "planner", Task: "plan"})
	if err != nil {
		t.Fatal(err)
	}
	if prov.Name() != "prov-a" || mdl.ID != "a-2" {
		t.Fatalf("got %s/%s, want decision-picked prov-a/a-2", prov.Name(), mdl.ID)
	}
}

func TestRouterSelectUnknownChoiceFallsBackToPreferred(t *testing.T) {
	mgr, _ := newRouterLLM(t, map[string][]llm.Model{
		"prov-a": {plainModel("a-1")},
	})
	dec := &mockDecisionProvider{name: "dec", choice: "prov-x/nope", conf: 0.9}
	cfg := routerCfg(map[string][]llm.Model{
		"prov-a": {plainModel("a-1")},
	}, nil)

	r := New(Deps{Config: cfg, LLM: mgr, Decision: newRouterDecision(t, "dec", dec)}, nil)
	prov, mdl, err := r.Select(context.Background(), SelectRequest{AgentName: "dev", Task: "t"})
	if err != nil {
		t.Fatal(err)
	}
	if prov.Name() != "prov-a" || mdl.ID != "a-1" {
		t.Fatalf("got %s/%s", prov.Name(), mdl.ID)
	}
}

func TestRouterSelectLLMFallback(t *testing.T) {
	providers := map[string][]llm.Model{
		"prov-a": {plainModel("a-1"), plainModel("a-2")},
	}
	mgr, _ := newRouterLLM(t, providers)
	cfg := routerCfg(providers, nil)

	r := New(Deps{Config: cfg, LLM: mgr}, nil)
	prov, mdl, err := r.Select(context.Background(), SelectRequest{AgentName: "orchestrator", Task: "run"})
	if err != nil {
		t.Fatal(err)
	}
	if prov.Name() != "prov-a" || mdl.ID != "a-1" {
		t.Fatalf("got %s/%s, want preferred prov-a/a-1", prov.Name(), mdl.ID)
	}
}

func TestRouterSelectLLMPicksModel(t *testing.T) {
	providers := map[string][]llm.Model{
		"prov-a": {plainModel("a-1"), plainModel("a-2")},
	}
	mgr, _ := newRouterLLMOut(t, providers, `{"model": "prov-a/a-2"}`)
	cfg := routerCfg(providers, nil)

	r := New(Deps{Config: cfg, LLM: mgr}, nil)
	prov, mdl, err := r.Select(context.Background(), SelectRequest{AgentName: "orchestrator", Task: "run"})
	if err != nil {
		t.Fatal(err)
	}
	if prov.Name() != "prov-a" || mdl.ID != "a-2" {
		t.Fatalf("got %s/%s, want llm-picked prov-a/a-2", prov.Name(), mdl.ID)
	}
}

func TestRouterDisabled(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.SmartRouter.Enabled = false
	r := New(Deps{Config: cfg}, nil)
	if r.Enabled() {
		t.Fatal("router must be disabled")
	}
	r2 := New(Deps{}, nil)
	if r2.Enabled() {
		t.Fatal("router must be disabled without config")
	}
}

func TestRouterDifficultyBuckets(t *testing.T) {
	providers := map[string][]llm.Model{
		"prov-a": {plainModel("easy-1"), plainModel("hard-1")},
	}
	mgr, _ := newRouterLLM(t, providers)
	dec := &mockDecisionProvider{name: "dec", choice: "prov-a/hard-1", conf: 0.9, score: 2.3}
	cfg := routerCfg(providers, map[string]config.RouterAgentConfig{
		"developer": {
			Default: []string{"easy-1"},
			Difficulty: map[string][]string{
				"trivial": {"easy-1"},
				"complex": {"hard-1"},
			},
		},
	})

	r := New(Deps{Config: cfg, LLM: mgr, Decision: newRouterDecision(t, "dec", dec)}, nil)
	prov, mdl, err := r.Select(context.Background(), SelectRequest{AgentName: "developer", Task: "rewrite the auth service"})
	if err != nil {
		t.Fatal(err)
	}
	if prov.Name() != "prov-a" || mdl.ID != "hard-1" {
		t.Fatalf("got %s/%s, want complex-bucket hard-1", prov.Name(), mdl.ID)
	}
	if len(dec.reqs) != 2 {
		t.Fatalf("expected 2 decision calls (difficulty + choice), got %d", len(dec.reqs))
	}
	if _, ok := dec.reqs[0].Questions["task_difficulty"]; !ok {
		t.Fatal("difficulty question missing")
	}
	if _, ok := dec.reqs[1].Questions["best_model"]; !ok {
		t.Fatal("model choice question missing")
	}
}

func TestRouterDifficultyTrivialBucket(t *testing.T) {
	providers := map[string][]llm.Model{
		"prov-a": {plainModel("easy-1"), plainModel("hard-1")},
	}
	mgr, _ := newRouterLLM(t, providers)
	dec := &mockDecisionProvider{name: "dec", choice: "prov-a/easy-1", conf: 0.9, score: 0.1}
	cfg := routerCfg(providers, map[string]config.RouterAgentConfig{
		"developer": {
			Default: []string{"easy-1"},
			Difficulty: map[string][]string{
				"trivial": {"easy-1"},
				"complex": {"hard-1"},
			},
		},
	})

	r := New(Deps{Config: cfg, LLM: mgr, Decision: newRouterDecision(t, "dec", dec)}, nil)
	prov, mdl, err := r.Select(context.Background(), SelectRequest{AgentName: "developer", Task: "fix the typo"})
	if err != nil {
		t.Fatal(err)
	}
	if prov.Name() != "prov-a" || mdl.ID != "easy-1" {
		t.Fatalf("got %s/%s, want trivial-bucket easy-1", prov.Name(), mdl.ID)
	}
}

func TestRouterNoDifficultyConfigSkipsEvaluation(t *testing.T) {
	providers := map[string][]llm.Model{
		"prov-a": {plainModel("a-1")},
	}
	mgr, _ := newRouterLLM(t, providers)
	dec := &mockDecisionProvider{name: "dec", choice: "prov-a/a-1", conf: 0.9}
	cfg := routerCfg(providers, nil)

	r := New(Deps{Config: cfg, LLM: mgr, Decision: newRouterDecision(t, "dec", dec)}, nil)
	if _, _, err := r.Select(context.Background(), SelectRequest{AgentName: "developer", Task: "do a thing"}); err != nil {
		t.Fatal(err)
	}
	if _, ok := dec.gotReq.Questions["task_difficulty"]; ok {
		t.Fatal("difficulty must not be evaluated without difficulty config")
	}
	if len(dec.reqs) != 1 {
		t.Fatalf("expected a single decision call, got %d", len(dec.reqs))
	}
}

func TestRouterImageListPreference(t *testing.T) {
	providers := map[string][]llm.Model{
		"prov-a": {visionModel("vision-x"), plainModel("text-1"), visionModel("vision-y")},
	}
	mgr, _ := newRouterLLM(t, providers)
	dec := &mockDecisionProvider{name: "dec", choice: "prov-a/vision-y", conf: 0.9}
	cfg := routerCfg(providers, map[string]config.RouterAgentConfig{
		"orchestrator": {
			Default: []string{"text-1"},
			Images:  []string{"vision-y"},
		},
	})

	r := New(Deps{Config: cfg, LLM: mgr, Decision: newRouterDecision(t, "dec", dec)}, nil)
	prov, mdl, err := r.Select(context.Background(), SelectRequest{AgentName: "orchestrator", Task: "look at this", HasImages: true})
	if err != nil {
		t.Fatal(err)
	}
	if prov.Name() != "prov-a" || mdl.ID != "vision-y" {
		t.Fatalf("got %s/%s, want image-list vision-y", prov.Name(), mdl.ID)
	}
}

func TestRouterImageSkipsDifficulty(t *testing.T) {
	providers := map[string][]llm.Model{
		"prov-a": {visionModel("vision-1")},
	}
	mgr, _ := newRouterLLM(t, providers)
	dec := &mockDecisionProvider{name: "dec", choice: "prov-a/vision-1", conf: 0.9}
	cfg := routerCfg(providers, map[string]config.RouterAgentConfig{
		"orchestrator": {
			Difficulty: map[string][]string{"complex": {"vision-1"}},
		},
	})

	r := New(Deps{Config: cfg, LLM: mgr, Decision: newRouterDecision(t, "dec", dec)}, nil)
	if _, _, err := r.Select(context.Background(), SelectRequest{AgentName: "orchestrator", Task: "read the image", HasImages: true}); err != nil {
		t.Fatal(err)
	}
	if _, ok := dec.gotReq.Questions["task_difficulty"]; ok {
		t.Fatal("difficulty must not be evaluated when images are attached")
	}
	if len(dec.reqs) != 1 {
		t.Fatalf("expected a single decision call, got %d", len(dec.reqs))
	}
}

func TestDifficultyFromScore(t *testing.T) {
	cases := []struct {
		score float64
		want  string
	}{
		{0, DifficultyTrivial},
		{0.4, DifficultyTrivial},
		{0.5, DifficultyModerate},
		{1.4, DifficultyModerate},
		{1.5, DifficultyComplex},
		{2.9, DifficultyComplex},
	}
	for _, c := range cases {
		if got := difficultyFromScore(c.score); got != c.want {
			t.Errorf("difficultyFromScore(%v) = %q, want %q", c.score, got, c.want)
		}
	}
}

func TestPreferencesResolution(t *testing.T) {
	cfg := routerCfg(nil, map[string]config.RouterAgentConfig{
		"agent": {
			Default:    []string{"d1"},
			Difficulty: map[string][]string{"complex": {"c1"}},
			Images:     []string{"i1"},
		},
	})
	r := New(Deps{Config: cfg}, nil)

	if got := r.preferences(SelectRequest{AgentName: "agent"}); len(got) != 1 || got[0] != "d1" {
		t.Fatalf("default = %v", got)
	}
	if got := r.preferences(SelectRequest{AgentName: "agent", Difficulty: "complex"}); len(got) != 1 || got[0] != "c1" {
		t.Fatalf("complex = %v", got)
	}
	if got := r.preferences(SelectRequest{AgentName: "agent", HasImages: true}); len(got) != 1 || got[0] != "i1" {
		t.Fatalf("images = %v", got)
	}
	if got := r.preferences(SelectRequest{AgentName: "other"}); got != nil {
		t.Fatalf("unknown agent = %v", got)
	}
}

func TestRouterResolveHookRoutesMarkerAgent(t *testing.T) {
	agent.ModelResolveHook.Reset()
	ModelOptionsHook.Reset()
	providers := map[string][]llm.Model{"prov-a": {plainModel("a-1")}}
	mgr, _ := newRouterLLM(t, providers)
	dec := &mockDecisionProvider{name: "dec", choice: "prov-a/a-1", conf: 0.9}
	cfg := routerCfg(providers, nil)
	New(Deps{Config: cfg, LLM: mgr, Decision: newRouterDecision(t, "dec", dec)}, nil)

	a := agent.New("orchestrator", agent.WithMaxIterations(1))
	a.Model = llm.Model{ID: SmartRouterModel}
	if _, err := a.Run(context.Background(), "do the thing"); err != nil {
		t.Fatal(err)
	}
	if a.Provider == nil || a.Provider.Name() != "prov-a" {
		t.Fatalf("provider = %v, want routed prov-a", a.Provider)
	}
	if a.Model.ID != "a-1" {
		t.Fatalf("model = %q, want routed a-1", a.Model.ID)
	}
	if !a.RouterManaged {
		t.Fatal("want RouterManaged set after routing")
	}
}

func TestRouterResolveHookRoutesSubagent(t *testing.T) {
	agent.ModelResolveHook.Reset()
	ModelOptionsHook.Reset()
	providers := map[string][]llm.Model{"prov-a": {plainModel("a-1")}}
	mgr, _ := newRouterLLM(t, providers)
	dec := &mockDecisionProvider{name: "dec", choice: "prov-a/a-1", conf: 0.9}
	cfg := routerCfg(providers, nil)
	New(Deps{Config: cfg, LLM: mgr, Decision: newRouterDecision(t, "dec", dec)}, nil)

	a := agent.New("developer", agent.WithMaxIterations(1))
	a.ParentAgentID = "parent-1"
	a.RouterManaged = true
	a.Model = llm.Model{ID: "inherited"}
	if _, err := a.Run(context.Background(), "write tests"); err != nil {
		t.Fatal(err)
	}
	if a.Provider == nil || a.Provider.Name() != "prov-a" || a.Model.ID != "a-1" {
		t.Fatalf("got %v/%q, want routed prov-a/a-1", a.Provider, a.Model.ID)
	}
}

func TestRouterResolveHookSkipsUnmanagedSubagent(t *testing.T) {
	agent.ModelResolveHook.Reset()
	ModelOptionsHook.Reset()
	providers := map[string][]llm.Model{"prov-a": {plainModel("a-1")}}
	mgr, _ := newRouterLLM(t, providers)
	dec := &mockDecisionProvider{name: "dec", choice: "prov-a/a-1", conf: 0.9}
	cfg := routerCfg(providers, nil)
	New(Deps{Config: cfg, LLM: mgr, Decision: newRouterDecision(t, "dec", dec)}, nil)

	a := agent.New("developer", agent.WithMaxIterations(1))
	a.ParentAgentID = "parent-1"
	a.Model = llm.Model{ID: "inherited"}
	a.Provider = &mockLLMProvider{name: "prov-a", models: providers["prov-a"]}
	if _, err := a.Run(context.Background(), "write tests"); err != nil {
		t.Fatal(err)
	}
	if a.Model.ID != "inherited" {
		t.Fatalf("model = %q, want unchanged inherited", a.Model.ID)
	}
	if len(dec.reqs) != 0 {
		t.Fatalf("decision provider called %d times, want 0", len(dec.reqs))
	}
}

func TestRouterResolveHookSkipsPlainAgent(t *testing.T) {
	agent.ModelResolveHook.Reset()
	ModelOptionsHook.Reset()
	providers := map[string][]llm.Model{"prov-a": {plainModel("a-1")}}
	mgr, _ := newRouterLLM(t, providers)
	dec := &mockDecisionProvider{name: "dec", choice: "prov-a/a-1", conf: 0.9}
	cfg := routerCfg(providers, nil)
	New(Deps{Config: cfg, LLM: mgr, Decision: newRouterDecision(t, "dec", dec)}, nil)

	a := agent.New("orchestrator", agent.WithMaxIterations(1))
	a.Model = llm.Model{ID: "gpt-4o"}
	a.Provider = &mockLLMProvider{name: "prov-a", models: providers["prov-a"]}
	if _, err := a.Run(context.Background(), "hi"); err != nil {
		t.Fatal(err)
	}
	if a.Model.ID != "gpt-4o" {
		t.Fatalf("model = %q, want unchanged gpt-4o", a.Model.ID)
	}
}

func TestRouterResolveHookDisabledLeavesPlaceholder(t *testing.T) {
	agent.ModelResolveHook.Reset()
	ModelOptionsHook.Reset()
	cfg := config.DefaultConfig()
	cfg.SmartRouter.Enabled = false
	New(Deps{Config: cfg}, nil)

	a := agent.New("orchestrator", agent.WithMaxIterations(1))
	a.Model = llm.Model{ID: SmartRouterModel}
	a.Provider = SmartRouterProvider
	_, err := a.Run(context.Background(), "hi")
	if err == nil || !strings.Contains(err.Error(), "smart router: no model resolved") {
		t.Fatalf("err = %v, want smart router placeholder error", err)
	}
	if a.Model.ID != SmartRouterModel {
		t.Fatalf("model = %q, want unchanged marker", a.Model.ID)
	}
}

func TestSmartRouterProviderName(t *testing.T) {
	if got := SmartRouterProvider.Name(); got != SmartRouterModel {
		t.Fatalf("placeholder name = %q", got)
	}
}

func TestRouterCandidateOrderPrefersConfigured(t *testing.T) {
	mgr, _ := newRouterLLM(t, map[string][]llm.Model{
		"prov-a": {plainModel("a-1")},
		"prov-b": {plainModel("b-2")},
	})
	dec := &mockDecisionProvider{name: "dec", conf: 0.5}
	cfg := routerCfg(map[string][]llm.Model{
		"prov-a": {plainModel("a-1")},
		"prov-b": {plainModel("b-2")},
	}, map[string]config.RouterAgentConfig{"planner": {Default: []string{"b-2"}}})

	r := New(Deps{Config: cfg, LLM: mgr, Decision: newRouterDecision(t, "dec", dec)}, nil)
	_, _, err := r.Select(context.Background(), SelectRequest{AgentName: "planner", Task: "plan"})
	if err != nil {
		t.Fatal(err)
	}
	criteria := dec.gotReq.Questions["best_model"].Criteria.(map[string]string)
	keys := make([]string, 0, len(criteria))
	for k := range criteria {
		keys = append(keys, k)
	}
	if len(keys) != 2 {
		t.Fatalf("expected both candidates, got %v", keys)
	}
}
