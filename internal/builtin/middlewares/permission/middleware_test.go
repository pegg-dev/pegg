package permission

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	json "github.com/goccy/go-json"

	"github.com/peggco/pegg/internal/agent"
	"github.com/peggco/pegg/internal/agent/tool"
	"github.com/peggco/pegg/internal/core/config"
	"github.com/peggco/pegg/internal/core/event"
	"github.com/peggco/pegg/internal/llm"
	"github.com/peggco/pegg/internal/vfs"
)

func testAgent(t *testing.T, tools ...tool.Tool) (*agent.Agent, event.Bus) {
	t.Helper()
	bus := event.New()
	a := agent.New("test-agent", agent.WithBus(bus))
	for _, tl := range tools {
		_ = a.Tools.Register(tl)
	}
	return a, bus
}

func testMiddleware(t *testing.T, deps Deps) *Middleware {
	t.Helper()
	if deps.Store == nil {
		deps.Store = &Store{
			path: filepath.Join(t.TempDir(), "permissions.json"),
			data: &storeData{Allowed: map[string]*allowedEntry{}, Rejected: map[string]*rejectedEntry{}},
		}
	}
	return New(deps)
}

func readSpec(fs *vfs.VFS) tool.Tool {
	return tool.NewSpec("read", "read a file", nil, func(ctx context.Context, args string) (string, error) {
		var p struct {
			FilePath string `json:"filePath"`
		}
		if err := json.Unmarshal([]byte(args), &p); err != nil {
			return "", err
		}
		return fs.ReadCtx(ctx, p.FilePath)
	}).SetPermissionError(func(err error) bool {
		return errors.Is(err, vfs.ErrOutOfBounds)
	})
}

func respondToAsks(bus event.Bus, agentID string, decision, reason map[string]string, count *int) {
	bus.Subscribe(agent.TopicAgentAsk, func(e agent.AgentAsk) {
		if e.AgentID != agentID {
			return
		}
		if count != nil {
			*count++
		}
		if len(e.Questions) > 0 && e.Questions[0].ID == "reason" {
			bus.Publish(agent.TopicAgentAskAnswer, agent.AgentAskAnswer{AgentID: e.AgentID, Answers: reason})
			return
		}
		bus.Publish(agent.TopicAgentAskAnswer, agent.AgentAskAnswer{AgentID: e.AgentID, Answers: decision})
	})
}

func TestModeResolution(t *testing.T) {
	m := testMiddleware(t, Deps{Config: &config.PermissionConfig{
		Default: "semi-ask",
		Rules:   config.DefaultConfig().Permission.Rules,
	}})
	if got := m.modeFor("read"); got != ModeSemiAsk {
		t.Errorf("read = %v, want semi-ask", got)
	}
	if got := m.modeFor("bash"); got != ModeSemiJudge {
		t.Errorf("bash = %v, want semi-judge", got)
	}
	if got := m.modeFor("todo"); got != ModeAllow {
		t.Errorf("todo = %v, want allow", got)
	}
	if got := m.modeFor("askuserquestion"); got != ModeAllow {
		t.Errorf("ask = %v, want allow", got)
	}
	if got := m.modeFor("enterplanmode"); got != ModeAsk {
		t.Errorf("enterplanmode = %v, want ask", got)
	}
	if got := m.modeFor("exitplanmode"); got != ModeAsk {
		t.Errorf("exitplanmode = %v, want ask", got)
	}
	if got := m.modeFor("unknown-tool"); got != ModeSemiAsk {
		t.Errorf("unknown tool = %v, want semi-ask (default)", got)
	}

	mc := New(Deps{Config: &config.PermissionConfig{
		Default: "allow",
		Rules:   map[string]string{"read": "judge", "weird": "banana"},
	}})
	if got := mc.modeFor("read"); got != ModeJudge {
		t.Errorf("config read = %v, want judge", got)
	}
	if got := mc.modeFor("bash"); got != ModeAllow {
		t.Errorf("config bash (unlisted) = %v, want allow", got)
	}
	if got := mc.modeFor("weird"); got != ModeAsk {
		t.Errorf("invalid mode = %v, want ask", got)
	}
}

func TestAllowModeRunsUnrestricted(t *testing.T) {
	fs := newFS(t)
	a, _ := testAgent(t, readSpec(fs))
	ctx := agent.WithAgent(context.Background(), a)

	outside := filepath.Join(filepath.Dir(fs.Root()), "secret.txt")
	_ = os.WriteFile(outside, []byte("secret\n"), 0o644)

	m := testMiddleware(t, Deps{Config: &config.PermissionConfig{Default: "allow"}})
	fs.OnAccessCheck(m.AccessChecker)
	var runs int
	output, err := m.InvokeTool(ctx, llm.ToolCall{Function: llm.Function{Name: "read", Arguments: `{"filePath": "` + outside + `"}`}}, func(ctx context.Context, call llm.ToolCall) (string, error) {
		runs++
		return fs.ReadCtx(ctx, outside)
	})
	if err != nil {
		t.Fatalf("allow mode should not fail: %v", err)
	}
	if runs != 1 || output == "" {
		t.Fatalf("runs = %d output = %q", runs, output)
	}
}

func TestAskModeUserAllow(t *testing.T) {
	fs := newFS(t)
	a, bus := testAgent(t, readSpec(fs))
	ctx := agent.WithAgent(context.Background(), a)
	respondToAsks(bus, a.ID, map[string]string{"decision": "Allow"}, nil, nil)

	m := testMiddleware(t, Deps{Config: &config.PermissionConfig{Default: "ask"}})
	var runs int
	_, err := m.InvokeTool(ctx, llm.ToolCall{Function: llm.Function{Name: "read", Arguments: `{"filePath": "a.txt"}`}}, func(ctx context.Context, call llm.ToolCall) (string, error) {
		runs++
		return "content", nil
	})
	if err != nil {
		t.Fatalf("approved call should run: %v", err)
	}
	if runs != 1 {
		t.Fatalf("runs = %d, want 1", runs)
	}
}

func TestPlanModeToolsAskByDefault(t *testing.T) {
	fs := newFS(t)
	a, bus := testAgent(t, readSpec(fs))
	ctx := agent.WithAgent(context.Background(), a)
	respondToAsks(bus, a.ID, map[string]string{"decision": "Allow"}, nil, nil)

	m := testMiddleware(t, Deps{Config: &config.PermissionConfig{
		Default: "semi-ask",
		Rules:   config.DefaultConfig().Permission.Rules,
	}})
	var runs int
	_, err := m.InvokeTool(ctx, llm.ToolCall{Function: llm.Function{Name: "enterplanmode", Arguments: "{}"}}, func(ctx context.Context, call llm.ToolCall) (string, error) {
		runs++
		return "entered", nil
	})
	if err != nil {
		t.Fatalf("approved enterplanmode should run: %v", err)
	}
	if runs != 1 {
		t.Fatalf("runs = %d, want 1", runs)
	}
}

func TestPlanModeToolsDeniedWithoutApproval(t *testing.T) {
	a, _ := testAgent(t)
	ctx := agent.WithAgent(context.Background(), a)

	m := testMiddleware(t, Deps{Config: &config.PermissionConfig{
		Default: "semi-ask",
		Rules:   config.DefaultConfig().Permission.Rules,
	}})
	var runs int
	_, err := m.InvokeTool(ctx, llm.ToolCall{Function: llm.Function{Name: "exitplanmode", Arguments: "{}"}}, func(ctx context.Context, call llm.ToolCall) (string, error) {
		runs++
		return "exited", nil
	})
	var denied *DeniedError
	if !errors.As(err, &denied) {
		t.Fatalf("expected DeniedError, got %v", err)
	}
	if runs != 0 {
		t.Fatalf("unapproved plan tool must not run, runs = %d", runs)
	}
}

func TestAskModeUserReject(t *testing.T) {
	fs := newFS(t)
	a, bus := testAgent(t, readSpec(fs))
	ctx := agent.WithAgent(context.Background(), a)
	respondToAsks(bus, a.ID, map[string]string{"decision": "Reject"}, map[string]string{"reason": "not now"}, nil)

	m := testMiddleware(t, Deps{Config: &config.PermissionConfig{Default: "ask"}})
	var runs int
	_, err := m.InvokeTool(ctx, llm.ToolCall{Function: llm.Function{Name: "read", Arguments: `{"filePath": "a.txt"}`}}, func(ctx context.Context, call llm.ToolCall) (string, error) {
		runs++
		return "content", nil
	})
	var denied *DeniedError
	if !errors.As(err, &denied) {
		t.Fatalf("expected DeniedError, got %v", err)
	}
	if denied.Reason != "not now" {
		t.Fatalf("reason = %q, want %q", denied.Reason, "not now")
	}
	if runs != 0 {
		t.Fatalf("rejected call must not run, runs = %d", runs)
	}
}

func TestAllowAllPersistsAndAutoApproves(t *testing.T) {
	fs := newFS(t)
	a, bus := testAgent(t, readSpec(fs))
	ctx := agent.WithAgent(context.Background(), a)

	asks := 0
	respondToAsks(bus, a.ID, map[string]string{"decision": "Allow All"}, nil, &asks)

	m := testMiddleware(t, Deps{Config: &config.PermissionConfig{Default: "ask"}})
	call := llm.ToolCall{Function: llm.Function{Name: "read", Arguments: `{"filePath": "a.txt"}`}}
	next := func(ctx context.Context, call llm.ToolCall) (string, error) {
		return "content", nil
	}

	if _, err := m.InvokeTool(ctx, call, next); err != nil {
		t.Fatalf("first call: %v", err)
	}
	if _, err := m.InvokeTool(ctx, call, next); err != nil {
		t.Fatalf("second call should auto-approve: %v", err)
	}
	if asks != 1 {
		t.Fatalf("asks = %d, want 1 (second call auto-approved)", asks)
	}
}

func TestRejectedStoredAndAutoRejected(t *testing.T) {
	fs := newFS(t)
	a, bus := testAgent(t, readSpec(fs))
	ctx := agent.WithAgent(context.Background(), a)

	asks := 0
	respondToAsks(bus, a.ID, map[string]string{"decision": "Reject"}, map[string]string{"reason": "no"}, &asks)

	m := testMiddleware(t, Deps{Config: &config.PermissionConfig{Default: "ask"}})
	call := llm.ToolCall{Function: llm.Function{Name: "read", Arguments: `{"filePath": "a.txt"}`}}
	next := func(ctx context.Context, call llm.ToolCall) (string, error) {
		return "content", nil
	}

	if _, err := m.InvokeTool(ctx, call, next); err == nil {
		t.Fatal("expected rejection")
	}
	_, err := m.InvokeTool(ctx, call, next)
	var denied *DeniedError
	if !errors.As(err, &denied) {
		t.Fatalf("expected DeniedError, got %v", err)
	}
	if denied.Reason != "previously rejected: no" {
		t.Fatalf("stored reason = %q, want %q", denied.Reason, "previously rejected: no")
	}
	if asks != 2 {
		t.Fatalf("asks = %d, want 2 (decision + reason, second call auto-rejected)", asks)
	}
}

func TestSemiAskPermissionErrorAsksAndReruns(t *testing.T) {
	fs := newFS(t)
	outside := filepath.Join(filepath.Dir(fs.Root()), "outside.txt")
	_ = os.WriteFile(outside, []byte("top secret\n"), 0o644)

	a, bus := testAgent(t, readSpec(fs))
	ctx := agent.WithAgent(context.Background(), a)
	respondToAsks(bus, a.ID, map[string]string{"decision": "Allow"}, nil, nil)

	m := testMiddleware(t, Deps{Config: &config.PermissionConfig{
		Default: "semi-ask",
		Rules:   config.DefaultConfig().Permission.Rules,
	}})
	fs.OnAccessCheck(m.AccessChecker)
	var runs int
	output, err := m.InvokeTool(ctx, llm.ToolCall{Function: llm.Function{Name: "read", Arguments: `{"filePath": "` + outside + `"}`}}, func(ctx context.Context, call llm.ToolCall) (string, error) {
		runs++
		return fs.ReadCtx(ctx, outside)
	})
	if err != nil {
		t.Fatalf("approved semi-ask should succeed: %v", err)
	}
	if runs != 2 {
		t.Fatalf("runs = %d, want 2 (denied then re-run)", runs)
	}
	if output == "" {
		t.Fatal("expected re-run output")
	}
}

func TestSemiAskNonPermissionErrorPassesThrough(t *testing.T) {
	fs := newFS(t)
	a, bus := testAgent(t, readSpec(fs))
	ctx := agent.WithAgent(context.Background(), a)

	asks := 0
	respondToAsks(bus, a.ID, map[string]string{"decision": "Allow"}, nil, &asks)

	m := testMiddleware(t, Deps{Config: &config.PermissionConfig{
		Default: "semi-ask",
		Rules:   config.DefaultConfig().Permission.Rules,
	}})
	_, err := m.InvokeTool(ctx, llm.ToolCall{Function: llm.Function{Name: "read", Arguments: `{"filePath": "missing.txt"}`}}, func(ctx context.Context, call llm.ToolCall) (string, error) {
		return "", vfs.ErrNotFound
	})
	if !errors.Is(err, vfs.ErrNotFound) {
		t.Fatalf("non-permission errors must pass through, got %v", err)
	}
	if asks != 0 {
		t.Fatalf("asks = %d, want 0", asks)
	}
}

func TestNoInteractiveHandlerRejects(t *testing.T) {
	fs := newFS(t)
	a, _ := testAgent(t, readSpec(fs))
	ctx := agent.WithAgent(context.Background(), a)

	m := testMiddleware(t, Deps{Config: &config.PermissionConfig{Default: "ask"}})
	_, err := m.InvokeTool(ctx, llm.ToolCall{Function: llm.Function{Name: "read", Arguments: `{"filePath": "a.txt"}`}}, func(ctx context.Context, call llm.ToolCall) (string, error) {
		return "content", nil
	})
	var denied *DeniedError
	if !errors.As(err, &denied) {
		t.Fatalf("expected DeniedError, got %v", err)
	}
	if denied.Reason != "no interactive prompt available" {
		t.Fatalf("reason = %q", denied.Reason)
	}
}

func TestBashSemiModeWhitelist(t *testing.T) {
	a, bus := testAgent(t)
	ctx := agent.WithAgent(context.Background(), a)

	asks := 0
	respondToAsks(bus, a.ID, map[string]string{"decision": "Allow"}, nil, &asks)

	m := testMiddleware(t, Deps{Config: &config.PermissionConfig{Default: "semi-ask"}})

	runs := 0
	next := func(ctx context.Context, call llm.ToolCall) (string, error) {
		runs++
		return "ok", nil
	}

	if _, err := m.InvokeTool(ctx, llm.ToolCall{Function: llm.Function{Name: "bash", Arguments: `{"command": "ls -la"}`}}, next); err != nil {
		t.Fatalf("whitelisted command should run: %v", err)
	}
	if asks != 0 || runs != 1 {
		t.Fatalf("whitelisted: asks=%d runs=%d", asks, runs)
	}

	if _, err := m.InvokeTool(ctx, llm.ToolCall{Function: llm.Function{Name: "bash", Arguments: `{"command": "cat /etc/passwd"}`}}, next); err != nil {
		t.Fatalf("asked command should run after approval: %v", err)
	}
	if asks != 1 || runs != 2 {
		t.Fatalf("asked: asks=%d runs=%d", asks, runs)
	}
}

type judgeProvider struct {
	verdict string
	calls   int
}

func (p *judgeProvider) Name() string { return "judge" }
func (p *judgeProvider) Chat(_ context.Context, _ *llm.Request) (*llm.Response, error) {
	p.calls++
	msg := llm.AssistantMessage(p.verdict)
	fr := llm.FinishReasonStop
	return &llm.Response{Choices: []llm.Choice{{Message: &msg, FinishReason: &fr}}}, nil
}
func (p *judgeProvider) ChatStream(context.Context, *llm.Request, llm.StreamHandler) error {
	return errors.New("not used")
}
func (p *judgeProvider) ListModels(context.Context) ([]llm.Model, error) { return nil, nil }

func TestJudgeMode(t *testing.T) {
	prov := &judgeProvider{verdict: `{"allow": false, "reason": "too risky"}`}
	m := testMiddleware(t, Deps{Config: &config.PermissionConfig{Default: "judge"}})
	m.judgeAgent = newJudgeAgent(prov, llm.Model{ID: "j"})

	a, _ := testAgent(t)
	ctx := agent.WithAgent(context.Background(), a)

	_, err := m.InvokeTool(ctx, llm.ToolCall{Function: llm.Function{Name: "read", Arguments: `{"filePath": "a.txt"}`}}, func(ctx context.Context, call llm.ToolCall) (string, error) {
		return "content", nil
	})
	var denied *DeniedError
	if !errors.As(err, &denied) {
		t.Fatalf("expected DeniedError, got %v", err)
	}
	if denied.Reason != "too risky" {
		t.Fatalf("reason = %q, want %q", denied.Reason, "too risky")
	}
	if prov.calls != 1 {
		t.Fatalf("judge calls = %d, want 1", prov.calls)
	}
}

func TestJudgeApproves(t *testing.T) {
	prov := &judgeProvider{verdict: `{"allow": true, "reason": "fine"}`}
	m := testMiddleware(t, Deps{Config: &config.PermissionConfig{Default: "judge"}})
	m.judgeAgent = newJudgeAgent(prov, llm.Model{ID: "j"})

	a, _ := testAgent(t)
	ctx := agent.WithAgent(context.Background(), a)

	var runs int
	output, err := m.InvokeTool(ctx, llm.ToolCall{Function: llm.Function{Name: "read", Arguments: `{"filePath": "a.txt"}`}}, func(ctx context.Context, call llm.ToolCall) (string, error) {
		runs++
		return "content", nil
	})
	if err != nil {
		t.Fatalf("judge-approved call should run: %v", err)
	}
	if runs != 1 || output != "content" {
		t.Fatalf("runs = %d output = %q", runs, output)
	}
}

func TestJudgeUnavailableRejects(t *testing.T) {
	m := testMiddleware(t, Deps{Config: &config.PermissionConfig{Default: "judge"}})
	a, _ := testAgent(t)
	ctx := agent.WithAgent(context.Background(), a)

	var runs int
	_, err := m.InvokeTool(ctx, llm.ToolCall{Function: llm.Function{Name: "read", Arguments: `{"filePath": "a.txt"}`}}, func(ctx context.Context, call llm.ToolCall) (string, error) {
		runs++
		return "content", nil
	})
	var denied *DeniedError
	if !errors.As(err, &denied) {
		t.Fatalf("expected DeniedError, got %v", err)
	}
	if denied.Reason != "judge provider unavailable" {
		t.Fatalf("reason = %q", denied.Reason)
	}
	if runs != 0 {
		t.Fatalf("runs = %d, want 0", runs)
	}
}

func newFS(t *testing.T) *vfs.VFS {
	t.Helper()
	fs, err := vfs.New(t.TempDir(), vfs.Options{})
	if err != nil {
		t.Fatalf("mount vfs: %v", err)
	}
	return fs
}
