package subagent

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	json "github.com/goccy/go-json"

	"github.com/peggco/pegg/internal/agent"
	"github.com/peggco/pegg/internal/agent/agents"
	"github.com/peggco/pegg/internal/agent/tool"
	"github.com/peggco/pegg/internal/agent/tools"
	builtinmw "github.com/peggco/pegg/internal/builtin/middlewares"
	"github.com/peggco/pegg/internal/core/config"
	"github.com/peggco/pegg/internal/core/event"
	"github.com/peggco/pegg/internal/core/logger"
	"github.com/peggco/pegg/internal/llm"
	"github.com/peggco/pegg/internal/session"
)

func TestMain(m *testing.M) {
	tmp, err := os.MkdirTemp("", "subagent-test-")
	if err != nil {
		fmt.Fprintln(os.Stderr, "subagent: mkdir temp:", err)
		os.Exit(1)
	}
	if err := os.Chdir(tmp); err != nil {
		fmt.Fprintln(os.Stderr, "subagent: chdir temp:", err)
		os.Exit(1)
	}
	store, err := session.NewJSONStoreAt(".")
	if err != nil {
		fmt.Fprintln(os.Stderr, "subagent: session store:", err)
		os.Exit(1)
	}
	testMgr = session.NewManager(store, testBus, logger.New(logger.LevelDebug, discardHandler{}))
	rec := session.NewRecorder(testMgr, logger.New(logger.LevelDebug, discardHandler{}))
	if err := rec.Start(testBus); err != nil {
		fmt.Fprintln(os.Stderr, "subagent: recorder:", err)
		os.Exit(1)
	}
	agents.Register(func() (*agent.Agent, error) {
		return agent.New("sub-agent",
			agent.WithToolNames("now"),
			agent.WithMiddlewareNames("loop-detector", "redaction"),
		), nil
	})
	SubAgentTools(testMgr)
	builtinmw.Create(nil, builtinmw.Deps{})
	tools.Register(tool.NewSpec("now", "stub", map[string]any{}, func(ctx context.Context, args string) (string, error) {
		return "now", nil
	}))
	os.Exit(m.Run())
}

var testBus = event.New()
var testMgr *session.Manager

type discardHandler struct{}

func (discardHandler) Write(logger.Record) error { return nil }
func (discardHandler) Close() error              { return nil }

type stubProvider struct{}

func (stubProvider) Name() string { return "stub" }
func (stubProvider) Chat(context.Context, *llm.Request) (*llm.Response, error) {
	return &llm.Response{Choices: []llm.Choice{{
		Index: 0,
		Message: func() *llm.Message {
			m := llm.AssistantMessage("sub answer")
			return &m
		}(),
		FinishReason: func() *llm.FinishReason { fr := llm.FinishReasonStop; return &fr }(),
	}}}, nil
}
func (stubProvider) ChatStream(context.Context, *llm.Request, llm.StreamHandler) error { return nil }
func (stubProvider) ListModels(context.Context) ([]llm.Model, error)                   { return nil, nil }

func parentCtx(t *testing.T, names ...string) context.Context {
	t.Helper()
	parent := agent.New("parent")
	parent.Provider = stubProvider{}
	parent.Model = llm.Model{ID: "stub-model"}
	return agent.WithAgent(context.Background(), parent)
}

func spec(name, agent, task string, extra ...map[string]any) map[string]any {
	s := map[string]any{"name": name, "subagent_type": agent, "prompt": task}
	for _, e := range extra {
		for k, v := range e {
			s[k] = v
		}
	}
	return s
}

func singleArgs(t *testing.T, s map[string]any) string {
	t.Helper()
	return mustJSON(t, s)
}

func batchArgs(t *testing.T, bg bool, specs ...map[string]any) string {
	t.Helper()
	if len(specs) == 0 {
		specs = []map[string]any{}
	}
	if len(specs) == 1 {
		specs[0]["background"] = bg
		return mustJSON(t, specs[0])
	}
	specs[0]["background"] = bg
	return mustJSON(t, specs[0])
}

func TestSubAgentToolsRegistered(t *testing.T) {
	for _, name := range []string{"task", "taskstatus"} {
		if _, ok := tools.Get(name); !ok {
			t.Errorf("tool %q not registered", name)
		}
	}
}

func TestSubAgentTool_Schema_AgentEnum(t *testing.T) {
	tt, _ := tools.Get("task")
	var schema map[string]any
	if err := json.Unmarshal([]byte(mustJSON(t, tt.Parameters())), &schema); err != nil {
		t.Fatalf("unmarshal schema: %v", err)
	}
	props := schema["properties"].(map[string]any)
	subagentTypeProp := props["subagent_type"].(map[string]any)
	enum, ok := subagentTypeProp["enum"].([]any)
	if !ok {
		t.Fatalf("agent enum missing: %v", subagentTypeProp)
	}
	registered := agents.List()
	if len(enum) != len(registered) {
		t.Errorf("enum %v does not match registered agents %v", enum, registered)
	}
	for _, r := range registered {
		found := false
		for _, e := range enum {
			if e == r {
				found = true
			}
		}
		if !found {
			t.Errorf("registered agent %q missing from enum %v", r, enum)
		}
	}
	required := schema["required"].([]any)
	if len(required) != 3 {
		t.Errorf("required = %v, want [name, subagent_type, prompt]", required)
	}
	if _, ok := props["background"]; !ok {
		t.Error("background property missing")
	}
	if _, ok := props["task_id"]; !ok {
		t.Error("task_id property missing")
	}
}

func TestSubAgentTool_Validation(t *testing.T) {
	tt, _ := tools.Get("task")
	ctx := parentCtx(t)

	cases := []struct {
		name string
		args string
	}{
		{"empty object", `{}`},
		{"missing name", `{"subagent_type": "sub-agent", "prompt": "do x"}`},
		{"missing prompt", `{"name": "a-1", "subagent_type": "sub-agent"}`},
		{"unknown agent", `{"name": "a-1", "subagent_type": "nope", "prompt": "do x"}`},
		{"empty name", `{"name": "", "subagent_type": "sub-agent", "prompt": "do x"}`},
		{"empty prompt", `{"name": "a-1", "subagent_type": "sub-agent", "prompt": ""}`},
	}
	for _, c := range cases {
		if _, err := tt.Execute(ctx, c.args); err == nil {
			t.Errorf("%s: expected error", c.name)
		}
	}
}

func TestSubAgentTool_NoParent(t *testing.T) {
	tt, _ := tools.Get("task")
	if _, err := tt.Execute(context.Background(), `{"name": "a-1", "subagent_type": "sub-agent", "prompt": "do x"}`); err == nil {
		t.Error("expected error without parent agent in context")
	}
}

func TestSubAgentTool_Foreground(t *testing.T) {
	tt, _ := tools.Get("task")
	ctx := parentCtx(t)
	args := singleArgs(t, spec("fg-1", "sub-agent", "do x", map[string]any{"task_id": []string{"todo-1"}}))
	out, err := tt.Execute(ctx, args)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(out, "sub answer") {
		t.Errorf("output = %q, want sub answer", out)
	}
	sa, ok := store.get("fg-1", nil)
	if !ok {
		t.Fatal("subagent not recorded")
	}
	if sa.Status != StatusCompleted {
		t.Errorf("status = %q, want completed", sa.Status)
	}
	if len(sa.TaskIDs) != 1 || sa.TaskIDs[0] != "todo-1" {
		t.Errorf("task ids = %v, want [todo-1]", sa.TaskIDs)
	}
}

func TestSubAgentTool_DuplicateName(t *testing.T) {
	tt, _ := tools.Get("task")
	ctx := parentCtx(t)
	if _, err := tt.Execute(ctx, singleArgs(t, spec("dup-1", "sub-agent", "first"))); err != nil {
		t.Fatalf("first run: %v", err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		sa, _ := store.get("dup-1", nil)
		if sa.Status == StatusCompleted || sa.Status == StatusFailed {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("first subagent not completed")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if _, err := tt.Execute(ctx, singleArgs(t, spec("dup-1", "sub-agent", "second"))); err != nil {
		t.Fatalf("second run (resume): %v", err)
	}
}

func TestSubAgentTool_MultipleForeground(t *testing.T) {
	tt, _ := tools.Get("task")
	ctx := parentCtx(t)

	for i := 1; i <= 3; i++ {
		name := fmt.Sprintf("seq-fg-%d", i)
		args := singleArgs(t, spec(name, "sub-agent", fmt.Sprintf("task %d", i)))
		out, err := tt.Execute(ctx, args)
		if err != nil {
			t.Fatalf("execute %s: %v", name, err)
		}
		if !strings.Contains(out, "sub answer") {
			t.Errorf("output = %q, want sub answer", out)
		}
		sa, ok := store.get(name, nil)
		if !ok {
			t.Fatalf("%s not recorded", name)
		}
		if sa.Status != StatusCompleted {
			t.Errorf("%s status = %q, want completed", name, sa.Status)
		}
	}
}

func TestSubAgentTool_BackgroundWithNotification(t *testing.T) {
	tt, _ := tools.Get("task")
	ctx := parentCtx(t)

	parent := agent.New("notify-parent", agent.WithBus(testBus))
	parent.Provider = stubProvider{}
	parent.Model = llm.Model{ID: "stub-model"}
	ctx = agent.WithAgent(context.Background(), parent)

	args := singleArgs(t, spec("bg-notify-1", "sub-agent", "do x", map[string]any{"background": true}))
	out, err := tt.Execute(ctx, args)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(out, "background") {
		t.Errorf("output = %q, want background confirmation", out)
	}

	deadline := time.Now().Add(5 * time.Second)
	for {
		sa, _ := store.get("bg-notify-1", nil)
		if sa.Status == StatusCompleted {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("background subagent not completed, status = %q", sa.Status)
		}
		time.Sleep(10 * time.Millisecond)
	}

	sa, ok := store.get("bg-notify-1", nil)
	if !ok {
		t.Fatal("subagent not recorded")
	}
	if sa.Status != StatusCompleted {
		t.Errorf("status = %q, want completed", sa.Status)
	}
}

func TestSubAgentTool_PublishesNotificationEvent(t *testing.T) {
	tt, _ := tools.Get("task")

	parent := agent.New("notify-event-parent", agent.WithBus(testBus))
	parent.Provider = stubProvider{}
	parent.Model = llm.Model{ID: "stub-model"}
	ctx := agent.WithAgent(context.Background(), parent)

	var notif *agent.SubAgentNotification
	_ = testBus.Subscribe(agent.TopicSubAgentNotification, func(e agent.SubAgentNotification) { notif = &e })

	args := singleArgs(t, spec("bg-notify-event", "sub-agent", "do x", map[string]any{"background": true}))
	if _, err := tt.Execute(ctx, args); err != nil {
		t.Fatalf("execute: %v", err)
	}

	deadline := time.Now().Add(5 * time.Second)
	for {
		sa, _ := store.get("bg-notify-event", nil)
		if sa.Status == StatusCompleted {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("background subagent not completed, status = %q", sa.Status)
		}
		time.Sleep(10 * time.Millisecond)
	}

	if notif == nil {
		t.Fatal("subagent notification event not published")
	}
	if notif.ParentAgentID != parent.ID || notif.SubAgentName != "bg-notify-event" {
		t.Errorf("notification = %+v, want parent %q subagent %q", notif, parent.ID, "bg-notify-event")
	}
	if !strings.Contains(notif.Output, "sub answer") {
		t.Errorf("notification output = %q, want sub answer", notif.Output)
	}
}

func TestSubAgentTool_BackgroundAndWait(t *testing.T) {
	tt, _ := tools.Get("task")
	ctx := parentCtx(t)

	args := singleArgs(t, spec("bg-1", "sub-agent", "do x", map[string]any{"background": true}))
	out, err := tt.Execute(ctx, args)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(out, "background") {
		t.Errorf("output = %q, want started confirmation", out)
	}

	deadline := time.Now().Add(5 * time.Second)
	for {
		sa, _ := store.get("bg-1", nil)
		if sa.Status == StatusCompleted {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("background subagent not completed, status = %q", sa.Status)
		}
		time.Sleep(10 * time.Millisecond)
	}

	sa, ok := store.get("bg-1", nil)
	if !ok {
		t.Fatal("subagent not recorded")
	}
	if sa.Status != StatusCompleted {
		t.Errorf("status = %q, want completed", sa.Status)
	}
	if !strings.Contains(sa.Output, "sub answer") {
		t.Errorf("output = %q, want sub answer", sa.Output)
	}
}

func TestSubagentsSessionIsolation(t *testing.T) {
	r := newRegistry()
	pa := agent.New("parent-a")
	pb := agent.New("parent-b")

	r.onSessionAttached(pa.ID, "sess-a")
	r.onSessionAttached(pb.ID, "sess-b")

	sa, err := r.spawn("iso-1", "sub-agent", nil, false, pa)
	if err != nil {
		t.Fatalf("spawn a: %v", err)
	}
	if _, err := r.spawn("iso-2", "sub-agent", nil, false, pb); err != nil {
		t.Fatalf("spawn b: %v", err)
	}

	if _, ok := r.get("iso-1", pa); !ok {
		t.Fatal("subagent must be visible in its own session")
	}
	if _, ok := r.get("iso-2", pb); !ok {
		t.Fatal("subagent must be visible in its own session")
	}
	if _, ok := r.get("iso-1", pb); ok {
		t.Fatal("subagent must NOT be visible in another session")
	}
	if _, ok := r.get("iso-2", pa); ok {
		t.Fatal("subagent must NOT be visible in another session")
	}

	sa.Status = StatusCompleted
	sa.finished = true
	r.saveFor(sa)

	loaded := newRegistry()
	loaded.onSessionAttached(pa.ID, "sess-a")
	loaded.onSessionAttached(pb.ID, "sess-b")
	got, ok := loaded.get("iso-1", pa)
	if !ok {
		t.Fatal("persisted subagent not loaded in its session")
	}
	if got.Name != "iso-1" {
		t.Fatalf("loaded = %q", got.Name)
	}
	if _, ok := loaded.get("iso-1", pb); ok {
		t.Fatal("persisted subagent leaked into another session")
	}
}

func TestSubagentSessionFollowsResume(t *testing.T) {
	r := newRegistry()
	bus := event.New()
	r.subscribe(bus)
	parent := agent.New("resume-parent", agent.WithBus(bus))

	bus.Publish(session.TopicSessionResume, session.SessionResume{AgentID: parent.ID, SessionID: "sess-r1"})
	if _, err := r.spawn("resume-1", "sub-agent", nil, false, parent); err != nil {
		t.Fatalf("spawn: %v", err)
	}
	if _, ok := r.get("resume-1", parent); !ok {
		t.Fatal("subagent not found in its session")
	}

	bus.Publish(session.TopicSessionResume, session.SessionResume{AgentID: parent.ID, SessionID: "sess-r2"})
	if _, err := r.spawn("resume-2", "sub-agent", nil, false, parent); err != nil {
		t.Fatalf("spawn: %v", err)
	}
	if _, ok := r.get("resume-1", parent); ok {
		t.Fatal("subagent from previous session leaked into the new session")
	}
	if _, ok := r.get("resume-2", parent); !ok {
		t.Fatal("subagent not found in switched session")
	}
}

func TestSubAgentTool_BackgroundAttachesStandingReminder(t *testing.T) {
	tt, _ := tools.Get("task")

	parent := agent.New("reminder-parent", agent.WithBus(testBus))
	parent.Provider = stubProvider{}
	parent.Model = llm.Model{ID: "stub-model"}
	ctx := agent.WithAgent(context.Background(), parent)

	args := singleArgs(t, spec("bg-rem-1", "sub-agent", "do x", map[string]any{"background": true}))
	if _, err := tt.Execute(ctx, args); err != nil {
		t.Fatalf("execute: %v", err)
	}

	r := parent.StandingReminder()
	if r == nil {
		t.Fatal("background task must attach a standing reminder")
	}
	if r.Tag != "background_subagent" {
		t.Fatalf("tag = %q, want background_subagent", r.Tag)
	}
	if !strings.Contains(r.Content, "bg-rem-1") {
		t.Fatalf("reminder content missing subagent name: %q", r.Content)
	}

	deadline := time.Now().Add(5 * time.Second)
	for {
		sa, _ := store.get("bg-rem-1", nil)
		if sa.Status == StatusCompleted {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("background subagent not completed")
		}
		time.Sleep(10 * time.Millisecond)
	}

	if r := parent.StandingReminder(); r != nil {
		t.Fatalf("standing reminder must be detached when the background subagent finishes, got %+v", r)
	}
	if !parent.HasPendingNotifications() {
		t.Fatal("finished notification must be queued for the parent")
	}
}

func TestSubAgentTool_ForegroundDoesNotAttachReminder(t *testing.T) {
	tt, _ := tools.Get("task")

	parent := agent.New("fg-reminder-parent", agent.WithBus(testBus))
	parent.Provider = stubProvider{}
	parent.Model = llm.Model{ID: "stub-model"}
	ctx := agent.WithAgent(context.Background(), parent)

	if _, err := tt.Execute(ctx, singleArgs(t, spec("fg-rem-1", "sub-agent", "do x"))); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if r := parent.StandingReminder(); r != nil {
		t.Fatalf("foreground task must not attach a standing reminder, got %+v", r)
	}
}

func TestIsSubagentSession(t *testing.T) {
	sa, err := store.spawn("sess-test-global-1", "sub-agent", nil, false, nil)
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	store.setAgentID(sa.Name, "sess-agent-1")
	store.onSessionAttached("sess-agent-1", "subagent-sess-123")

	if !IsSubagentSession("subagent-sess-123") {
		t.Fatal("recorded subagent session must be recognized")
	}
	if IsSubagentSession("main-sess-999") {
		t.Fatal("main session must not be reported as subagent session")
	}
	if IsSubagentSession("") {
		t.Fatal("empty session must not be reported as subagent session")
	}
}

func TestSubAgentsStatus(t *testing.T) {
	tt, _ := tools.Get("task")
	status, _ := tools.Get("taskstatus")
	ctx := parentCtx(t)

	if _, err := tt.Execute(ctx, singleArgs(t, spec("stat-1", "sub-agent", "do x"))); err != nil {
		t.Fatalf("spawn: %v", err)
	}

	out, err := status.Execute(ctx, `{}`)
	if err != nil {
		t.Fatalf("status all: %v", err)
	}
	if !strings.Contains(out, "stat-1") {
		t.Errorf("status output missing stat-1: %q", out)
	}

	if _, err := status.Execute(ctx, `{"agent_names": ["ghost"]}`); err == nil {
		t.Error("expected error for unknown name filter")
	}
	out, err = status.Execute(ctx, `{"agent_names": ["stat-1"]}`)
	if err != nil {
		t.Fatalf("status filter: %v", err)
	}
	if !strings.Contains(out, "stat-1") {
		t.Errorf("status output missing stat-1: %q", out)
	}
}

func TestSubAgentTool_ConcurrentParents(t *testing.T) {
	tt, _ := tools.Get("task")
	status, _ := tools.Get("taskstatus")

	const parents = 8
	var wg sync.WaitGroup
	for i := 0; i < parents; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			ctx := parentCtx(t)
			name := fmt.Sprintf("conc-%d", i)
			args := singleArgs(t, spec(name, "sub-agent", "do x"))
			_, err := tt.Execute(ctx, args)
			if err != nil {
				t.Errorf("parent %d spawn: %v", i, err)
			}
		}(i)
	}
	wg.Wait()

	deadline := time.Now().Add(5 * time.Second)
	for {
		allDone := true
		for i := 0; i < parents; i++ {
			name := fmt.Sprintf("conc-%d", i)
			sa, _ := store.get(name, nil)
			if sa.Status != StatusCompleted {
				allDone = false
				break
			}
		}
		if allDone {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("concurrent subagents not completed")
		}
		time.Sleep(10 * time.Millisecond)
	}

	statusOut, err := status.Execute(parentCtx(t), `{}`)
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	for i := 0; i < parents; i++ {
		if !strings.Contains(statusOut, fmt.Sprintf("conc-%d", i)) {
			t.Errorf("status output missing conc-%d", i)
		}
	}
}

func TestRegistry_PersistAndLoad(t *testing.T) {
	r := newRegistry()
	sa, err := r.spawn("persist-1", "sub-agent", []string{"todo-9"}, false, nil)
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	r.start(sa)
	r.finish(sa, "persisted output", nil)

	file, err := config.GetProjectConfigPath("subagents", "default.json")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(file); err != nil {
		t.Fatalf("subagents default.json not written: %v", err)
	}

	loaded := newRegistry()
	if err := loaded.init(); err != nil {
		t.Fatalf("load: %v", err)
	}
	got, ok := loaded.get("persist-1", nil)
	if !ok {
		t.Fatal("persisted subagent not loaded")
	}
	if got.Status != StatusCompleted || got.Output != "persisted output" {
		t.Errorf("loaded = %+v, want completed with output", got)
	}
	if len(got.TaskIDs) != 1 || got.TaskIDs[0] != "todo-9" {
		t.Errorf("task ids = %v, want [todo-9]", got.TaskIDs)
	}

	if got.Status != StatusCompleted && got.Status != StatusFailed {
		t.Errorf("loaded subagent status = %q, want completed or failed", got.Status)
	}
}

func TestRegistry_LoadInterruptsRunning(t *testing.T) {
	r := newRegistry()
	sa, err := r.spawn("interrupt-1", "sub-agent", nil, true, nil)
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	r.start(sa)

	loaded := newRegistry()
	if err := loaded.init(); err != nil {
		t.Fatalf("load: %v", err)
	}
	got, ok := loaded.get("interrupt-1", nil)
	if !ok {
		t.Fatal("running subagent not loaded")
	}
	if got.Status != StatusInterrupted {
		t.Errorf("status = %q, want interrupted", got.Status)
	}
	if got.Err == "" {
		t.Error("interrupted subagent should have an error message")
	}
}

func TestAgentOutput(t *testing.T) {
	if got := agentOutput(&agent.RunResult{Output: "final answer"}, nil); got != "final answer" {
		t.Errorf("expected final output, got %q", got)
	}
	res := &agent.RunResult{
		Output: "",
		History: []llm.Message{
			llm.SystemMessage("sys"),
			llm.UserMessage("do it"),
			llm.AssistantMessage("step one"),
			llm.AssistantMessage("step two"),
		},
	}
	if got := agentOutput(res, nil); got != "step one\nstep two" {
		t.Errorf("history fallback = %q, want 'step one\\nstep two'", got)
	}
	if got := agentOutput(nil, errSentinel); got != "boom" {
		t.Errorf("error fallback = %q, want boom", got)
	}
	if got := agentOutput(nil, nil); got != "" {
		t.Errorf("empty result should yield empty output, got %q", got)
	}
}

var errSentinel = errors.New("boom")

func TestSubAgentTool_EventDrivenForeground(t *testing.T) {
	tt, _ := tools.Get("task")

	parent := agent.New("event-parent", agent.WithBus(testBus))
	parent.Provider = stubProvider{}
	parent.Model = llm.Model{ID: "stub-model"}
	ctx := agent.WithAgent(context.Background(), parent)

	out, err := tt.Execute(ctx, batchArgs(t, false, spec("evt-fg", "sub-agent", "do x")))
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(out, "sub answer") {
		t.Errorf("output = %q, want sub answer", out)
	}
	sa, ok := store.get("evt-fg", nil)
	if !ok {
		t.Fatal("subagent not recorded")
	}
	if sa.Status != StatusCompleted {
		t.Errorf("status = %q, want completed (event-driven)", sa.Status)
	}
	if sa.Output != "sub answer" {
		t.Errorf("output = %q, want sub answer (from agent.finished event)", sa.Output)
	}
}

func TestSubAgentTool_EventDrivenBackground(t *testing.T) {
	tt, _ := tools.Get("task")

	parent := agent.New("event-parent-2", agent.WithBus(testBus))
	parent.Provider = stubProvider{}
	parent.Model = llm.Model{ID: "stub-model"}
	ctx := agent.WithAgent(context.Background(), parent)

	args := singleArgs(t, spec("evt-bg", "sub-agent", "do x"))
	if _, err := tt.Execute(ctx, args); err != nil {
		t.Fatalf("execute: %v", err)
	}

	deadline := time.Now().Add(5 * time.Second)
	for {
		sa, _ := store.get("evt-bg", nil)
		if sa.Status == StatusCompleted {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("background subagent not completed, status = %q", sa.Status)
		}
		time.Sleep(10 * time.Millisecond)
	}

	sa, ok := store.get("evt-bg", nil)
	if !ok {
		t.Fatal("subagent not recorded")
	}
	if !strings.Contains(sa.Output, "sub answer") {
		t.Errorf("output = %q, want sub answer", sa.Output)
	}
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return string(b)
}
