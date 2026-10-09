package memory

import (
	"context"
	"strings"
	"testing"

	"github.com/peggco/pegg/internal/core/config"
	"github.com/peggco/pegg/internal/core/event"
	"github.com/peggco/pegg/internal/llm"
)

type gateProvider struct{}

func (p *gateProvider) Name() string { return "gate-llm" }

func (p *gateProvider) Chat(_ context.Context, req *llm.Request) (*llm.Response, error) {
	var last string
	for i := len(req.Messages) - 1; i >= 0; i-- {
		if last = llm.MessageText(req.Messages[i]); last != "" {
			break
		}
	}
	content := "1, 3"
	if !strings.Contains(last, "candidates") {
		content = "none"
	}
	msg := llm.AssistantMessage(content)
	fr := llm.FinishReasonStop
	return &llm.Response{
		Model:   "gate-llm",
		Choices: []llm.Choice{{Index: 0, Message: &msg, FinishReason: &fr}},
	}, nil
}

func (p *gateProvider) ChatStream(_ context.Context, _ *llm.Request, _ llm.StreamHandler) error {
	return nil
}

func (p *gateProvider) ListModels(_ context.Context) ([]llm.Model, error) {
	return []llm.Model{{ID: "gate-1"}}, nil
}

func TestGateByScoreKeepsTopCluster(t *testing.T) {
	hits := []SearchHit{
		{ID: "a", Title: "auth", Score: 20},
		{ID: "b", Title: "auth-ish", Score: 12},
		{ID: "c", Title: "weak", Score: 3},
	}
	kept := gateByScore(hits)
	if len(kept) != 2 || kept[0].ID != "a" || kept[1].ID != "b" {
		t.Fatalf("kept = %+v, want top cluster [a b]", kept)
	}
}

func TestGateByScoreLowFloor(t *testing.T) {
	hits := []SearchHit{{ID: "a", Title: "x", Score: 2}}
	kept := gateByScore(hits)
	if len(kept) != 1 {
		t.Fatalf("kept = %+v, want the single hit even at floor 2", kept)
	}
	hits = []SearchHit{{ID: "a", Title: "x", Score: 1}}
	if kept := gateByScore(hits); len(kept) != 0 {
		t.Fatalf("kept = %+v, want empty (score below floor)", kept)
	}
}

func TestParseGateIndices(t *testing.T) {
	cases := []struct {
		in   string
		want []int
	}{
		{"1, 3, 5", []int{0, 2, 4}},
		{"none", nil},
		{"None.", nil},
		{"```\n2\n```", []int{1}},
		{"Only 1 and 4 are relevant", []int{0, 3}},
		{"garbage", nil},
	}
	for _, c := range cases {
		got := parseGateIndices(c.in)
		if len(got) != len(c.want) {
			t.Fatalf("parseGateIndices(%q) = %v, want %v", c.in, got, c.want)
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Fatalf("parseGateIndices(%q) = %v, want %v", c.in, got, c.want)
			}
		}
	}
}

func TestGateDispatcherOffAndScore(t *testing.T) {
	bus := event.New()
	hits := []SearchHit{
		{ID: "a", Title: "auth token", Score: 20},
		{ID: "b", Title: "weak", Score: 2},
	}

	cfg := defaultCfg()
	cfg.Gate = config.GateOff
	m := newManager(Deps{Config: cfg, Bus: bus}, t.TempDir())
	if got := m.gateHits(context.Background(), "q", hits); len(got) != 2 {
		t.Fatalf("off mode changed hits: %+v", got)
	}

	cfg.Gate = config.GateScore
	if got := m.gateHits(context.Background(), "q", hits); len(got) != 1 || got[0].ID != "a" {
		t.Fatalf("score mode = %+v, want [a]", got)
	}
}

func TestGateAutoFallsBackToScore(t *testing.T) {
	bus := event.New()
	cfg := defaultCfg()
	m := newManager(Deps{Config: cfg, Bus: bus}, t.TempDir())
	hits := []SearchHit{
		{ID: "a", Title: "auth token", Score: 20},
		{ID: "b", Title: "weak", Score: 2},
	}
	got := m.gateHits(context.Background(), "q", hits)
	if len(got) != 1 || got[0].ID != "a" {
		t.Fatalf("auto fallback = %+v, want [a]", got)
	}
}

func TestGateDecisionUnavailableFailsOpen(t *testing.T) {
	bus := event.New()
	cfg := defaultCfg()
	cfg.Gate = config.GateDecision
	m := newManager(Deps{Config: cfg, Bus: bus}, t.TempDir())
	hits := []SearchHit{{ID: "a", Title: "x", Score: 10}}
	if got := m.gateHits(context.Background(), "q", hits); len(got) != 1 {
		t.Fatalf("decision-unavailable should fail open, got %+v", got)
	}
}

func TestGateValueNormalization(t *testing.T) {
	if got := (&config.MemoryConfig{}).GateValue(); got != config.GateAuto {
		t.Fatalf("empty gate = %q, want auto", got)
	}
	if got := (&config.MemoryConfig{Gate: "bogus"}).GateValue(); got != config.GateAuto {
		t.Fatalf("bogus gate = %q, want auto", got)
	}
	if got := (&config.MemoryConfig{Gate: config.GateLLM}).GateValue(); got != config.GateLLM {
		t.Fatalf("llm gate = %q", got)
	}
}

func TestGateLLMMode(t *testing.T) {
	bus := event.New()
	llmMgr := llm.NewManager(bus, testLogger(), testCache(t))
	if err := llmMgr.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(llmMgr.Shutdown)
	llm.RegisterProvider("gate-llm", func(cfg config.LLMConfig) (llm.Provider, error) {
		return &gateProvider{}, nil
	})
	llmMgr.Sync(context.Background(), []config.LLMConfig{{Provider: "gate-llm"}})

	cfg := defaultCfg()
	cfg.Gate = config.GateLLM
	m := newManager(Deps{Config: cfg, LLM: llmMgr, Bus: bus}, t.TempDir())

	hits := []SearchHit{
		{ID: "a", Title: "auth token", Score: 20},
		{ID: "b", Title: "cache eviction", Score: 10},
		{ID: "c", Title: "deploy notes", Score: 8},
	}
	got := m.gateHits(context.Background(), "auth", hits)
	if len(got) != 2 || got[0].ID != "a" || got[1].ID != "c" {
		t.Fatalf("llm gate = %+v, want [a c]", got)
	}
}
