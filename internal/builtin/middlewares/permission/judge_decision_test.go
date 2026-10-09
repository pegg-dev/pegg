package permission

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/peggco/pegg/internal/core/config"
	"github.com/peggco/pegg/internal/core/event"
	"github.com/peggco/pegg/internal/core/logger"
	decisionapi "github.com/peggco/pegg/internal/decision"
	"github.com/peggco/pegg/internal/llm"
)

type judgeDecisionProvider struct {
	name   string
	decide func(ctx context.Context, req *decisionapi.Request) (*decisionapi.Response, error)
}

func (p *judgeDecisionProvider) Name() string { return p.name }
func (p *judgeDecisionProvider) Decide(ctx context.Context, req *decisionapi.Request) (*decisionapi.Response, error) {
	if p.decide != nil {
		return p.decide(ctx, req)
	}
	return &decisionapi.Response{Answers: map[string]decisionapi.Answer{}}, nil
}

func noulResponse(p float64) *decisionapi.Response {
	n := p
	return &decisionapi.Response{
		Answers: map[string]decisionapi.Answer{
			"safe_to_run": {Type: decisionapi.QuestionNoul, Noul: &n},
		},
	}
}

func newJudgeDecisionManager(t *testing.T, name string, prov decisionapi.Provider) *decisionapi.Manager {
	return newJudgeDecisionManagerKeyed(t, name, "", prov)
}

func newJudgeDecisionManagerKeyed(t *testing.T, name, apiKey string, prov decisionapi.Provider) *decisionapi.Manager {
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
		Providers: []config.LLMConfig{{Provider: name, APIKey: apiKey}},
	})
	return mgr
}

func TestAskJudgeUsesDecisionModel(t *testing.T) {
	var gotReq *decisionapi.Request
	prov := &judgeDecisionProvider{
		name: "dec-prov",
		decide: func(_ context.Context, req *decisionapi.Request) (*decisionapi.Response, error) {
			gotReq = req
			return noulResponse(0.9), nil
		},
	}
	m := &Middleware{
		decision: newJudgeDecisionManager(t, "dec-prov", prov),
		cfg:      &config.PermissionConfig{JudgeProvider: "dec-prov"},
	}

	call := llm.ToolCall{Function: llm.Function{Name: "bash", Arguments: `{"command":"rm -rf /"}`}}
	dec, err := m.askJudge(context.Background(), call, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !dec.Allow {
		t.Fatal("expected decision judge to allow")
	}
	if gotReq == nil {
		t.Fatal("no decision request captured")
	}
	if gotReq.Model != "" {
		t.Errorf("model = %q, want empty (provider default)", gotReq.Model)
	}
	q, ok := gotReq.Questions["safe_to_run"]
	if !ok {
		t.Fatal("expected safe_to_run question")
	}
	if q.Type != decisionapi.QuestionNoul {
		t.Errorf("question type = %q", q.Type)
	}
	if !strings.Contains(gotReq.State, "bash") || !strings.Contains(gotReq.State, "rm -rf /") {
		t.Errorf("state missing tool context:\n%s", gotReq.State)
	}
}

func TestAskJudgeDecisionDeniesBelowThreshold(t *testing.T) {
	prov := &judgeDecisionProvider{
		name: "dec-prov",
		decide: func(_ context.Context, _ *decisionapi.Request) (*decisionapi.Response, error) {
			return noulResponse(0.5), nil
		},
	}
	m := &Middleware{
		decision: newJudgeDecisionManager(t, "dec-prov", prov),
		cfg:      &config.PermissionConfig{JudgeProvider: "dec-prov"},
	}

	dec, err := m.askJudge(context.Background(), llm.ToolCall{Function: llm.Function{Name: "bash", Arguments: "{}"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if dec.Allow {
		t.Fatal("expected decision judge to deny below default threshold")
	}
	if !strings.Contains(dec.Reason, "0.50") {
		t.Errorf("reason = %q", dec.Reason)
	}
}

func TestAskJudgeDecisionCustomThreshold(t *testing.T) {
	var p float64 = 0.6
	prov := &judgeDecisionProvider{
		name: "dec-prov",
		decide: func(_ context.Context, _ *decisionapi.Request) (*decisionapi.Response, error) {
			return noulResponse(p), nil
		},
	}
	threshold := 0.5
	m := &Middleware{
		decision: newJudgeDecisionManager(t, "dec-prov", prov),
		cfg: &config.PermissionConfig{
			JudgeProvider:  "dec-prov",
			JudgeThreshold: &threshold,
		},
	}

	dec, err := m.askJudge(context.Background(), llm.ToolCall{Function: llm.Function{Name: "bash", Arguments: "{}"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !dec.Allow {
		t.Fatalf("expected allow at 0.6 >= threshold 0.5, got %+v", dec)
	}

	p = 0.4
	dec, err = m.askJudge(context.Background(), llm.ToolCall{Function: llm.Function{Name: "bash", Arguments: "{}"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if dec.Allow {
		t.Fatalf("expected deny at 0.4 < threshold 0.5, got %+v", dec)
	}
}

func TestAskJudgeDecisionModelFromConfig(t *testing.T) {
	var gotModel string
	prov := &judgeDecisionProvider{
		name: "dec-prov",
		decide: func(_ context.Context, req *decisionapi.Request) (*decisionapi.Response, error) {
			gotModel = req.Model
			return noulResponse(0.9), nil
		},
	}
	m := &Middleware{
		decision: newJudgeDecisionManager(t, "dec-prov", prov),
		cfg:      &config.PermissionConfig{JudgeProvider: "dec-prov", JudgeModel: "typesafe/jev-1.13"},
	}

	if _, err := m.askJudge(context.Background(), llm.ToolCall{Function: llm.Function{Name: "bash", Arguments: "{}"}}, nil); err != nil {
		t.Fatal(err)
	}
	if gotModel != "typesafe/jev-1.13" {
		t.Errorf("model = %q", gotModel)
	}
}

type judgeLLMContentProvider struct {
	name   string
	models []llm.Model
	output string
}

func (p *judgeLLMContentProvider) Name() string { return p.name }
func (p *judgeLLMContentProvider) Chat(context.Context, *llm.Request) (*llm.Response, error) {
	msg := llm.AssistantMessage(p.output)
	return &llm.Response{Choices: []llm.Choice{{Message: &msg}}}, nil
}
func (p *judgeLLMContentProvider) ChatStream(context.Context, *llm.Request, llm.StreamHandler) error {
	return errors.New("not used")
}
func (p *judgeLLMContentProvider) ListModels(context.Context) ([]llm.Model, error) {
	return p.models, nil
}

func newJudgeLLMManager(t *testing.T, name, output string) (*llm.Manager, event.Bus) {
	t.Helper()
	mgr, bus := newJudgeTestManager(t)
	llm.RegisterProvider(name, func(cfg config.LLMConfig) (llm.Provider, error) {
		return &judgeLLMContentProvider{name: cfg.Provider, models: []llm.Model{{ID: cfg.Provider + "-model"}}, output: output}, nil
	})
	bus.Publish(event.TopicAppMounted, &config.Config{
		Providers: []config.LLMConfig{{Provider: name}},
	})
	mgr.WaitUntilReady()
	return mgr, bus
}

func TestAskJudgeDecisionInvalidAnswerFallsBackToLLM(t *testing.T) {
	prov := &judgeDecisionProvider{
		name: "dec-prov",
		decide: func(_ context.Context, _ *decisionapi.Request) (*decisionapi.Response, error) {
			return &decisionapi.Response{Answers: map[string]decisionapi.Answer{
				"safe_to_run": {Type: decisionapi.QuestionScore},
			}}, nil
		},
	}
	mgr, _ := newJudgeLLMManager(t, "judge-fb", `{"allow": true, "reason": "llm said ok"}`)

	m := &Middleware{
		llm:      mgr,
		decision: newJudgeDecisionManager(t, "dec-prov", prov),
		cfg:      &config.PermissionConfig{JudgeProvider: "dec-prov"},
	}

	dec, err := m.askJudge(context.Background(), llm.ToolCall{Function: llm.Function{Name: "bash", Arguments: "{}"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !dec.Allow {
		t.Fatalf("expected LLM fallback to allow, got %+v", dec)
	}
}

func TestAskJudgeDecisionErrorFallsBackToLLM(t *testing.T) {
	prov := &judgeDecisionProvider{
		name: "dec-prov",
		decide: func(_ context.Context, _ *decisionapi.Request) (*decisionapi.Response, error) {
			return nil, errors.New("network down")
		},
	}
	mgr, _ := newJudgeLLMManager(t, "judge-fb2", `{"allow": true, "reason": "llm said ok"}`)

	m := &Middleware{
		llm:      mgr,
		decision: newJudgeDecisionManager(t, "dec-prov", prov),
		cfg:      &config.PermissionConfig{JudgeProvider: "dec-prov"},
	}

	dec, err := m.askJudge(context.Background(), llm.ToolCall{Function: llm.Function{Name: "bash", Arguments: "{}"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !dec.Allow {
		t.Fatalf("expected LLM fallback to allow, got %+v", dec)
	}
}

func TestResolveDecisionJudgeUnavailable(t *testing.T) {
	m := &Middleware{}
	if _, _, ok := m.resolveDecisionJudge(); ok {
		t.Fatal("expected no decision judge without a manager")
	}

	m = &Middleware{
		decision: newJudgeDecisionManagerKeyed(t, "dec-prov", "sk-1", &judgeDecisionProvider{name: "dec-prov"}),
		cfg:      &config.PermissionConfig{JudgeProvider: "dec-other"},
	}
	if _, _, ok := m.resolveDecisionJudge(); ok {
		t.Fatal("expected no decision judge for an unloaded provider")
	}
}

func TestResolveDecisionJudgeNoAPIKey(t *testing.T) {
	m := &Middleware{decision: newJudgeDecisionManager(t, "dec-prov", &judgeDecisionProvider{name: "dec-prov"})}
	if _, _, ok := m.resolveDecisionJudge(); ok {
		t.Fatal("expected no default decision judge without an api key")
	}
}

func TestResolveDecisionJudgeDefaultPreferred(t *testing.T) {
	prov := &judgeDecisionProvider{name: "dec-prov"}
	m := &Middleware{decision: newJudgeDecisionManagerKeyed(t, "dec-prov", "sk-1", prov)}
	got, model, ok := m.resolveDecisionJudge()
	if !ok || got != prov || model != "" {
		t.Fatalf("ok=%v prov=%v model=%q", ok, got, model)
	}

	m = &Middleware{
		decision: newJudgeDecisionManagerKeyed(t, "dec-prov", "sk-1", prov),
		cfg:      &config.PermissionConfig{JudgeModel: "typesafe/jev-1.13"},
	}
	got, model, ok = m.resolveDecisionJudge()
	if !ok || model != "typesafe/jev-1.13" {
		t.Fatalf("ok=%v model=%q", ok, model)
	}
}

func TestAskJudgeUsesDefaultDecisionProvider(t *testing.T) {
	prov := &judgeDecisionProvider{
		name: "dec-prov",
		decide: func(_ context.Context, _ *decisionapi.Request) (*decisionapi.Response, error) {
			return noulResponse(0.95), nil
		},
	}
	m := &Middleware{
		decision: newJudgeDecisionManagerKeyed(t, "dec-prov", "sk-1", prov),
	}

	dec, err := m.askJudge(context.Background(), llm.ToolCall{Function: llm.Function{Name: "bash", Arguments: "{}"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !dec.Allow {
		t.Fatal("expected default decision judge to allow")
	}
}

func TestAskJudgeFallsBackToLLMWithoutDecisionManager(t *testing.T) {
	mgr, _ := newJudgeLLMManager(t, "judge-fb3", `{"allow": true, "reason": "llm said ok"}`)

	m := &Middleware{
		llm: mgr,
		cfg: &config.PermissionConfig{JudgeProvider: "dec-prov"},
	}

	dec, err := m.askJudge(context.Background(), llm.ToolCall{Function: llm.Function{Name: "bash", Arguments: "{}"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !dec.Allow {
		t.Fatalf("expected LLM fallback to allow, got %+v", dec)
	}
}

func TestJudgeThresholdDefault(t *testing.T) {
	if got := (&config.PermissionConfig{}).JudgeThresholdValue(); got != config.DefaultJudgeThreshold {
		t.Fatalf("default threshold = %v, want %v", got, config.DefaultJudgeThreshold)
	}
	if got := (*config.PermissionConfig)(nil).JudgeThresholdValue(); got != config.DefaultJudgeThreshold {
		t.Fatalf("nil threshold = %v, want %v", got, config.DefaultJudgeThreshold)
	}
	thr := 0.5
	if got := (&config.PermissionConfig{JudgeThreshold: &thr}).JudgeThresholdValue(); got != 0.5 {
		t.Fatalf("custom threshold = %v, want 0.5", got)
	}
}
