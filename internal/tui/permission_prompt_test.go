package tui

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	json "github.com/goccy/go-json"

	"github.com/peggco/pegg/internal/agent"
	"github.com/peggco/pegg/internal/agent/tool"
	"github.com/peggco/pegg/internal/builtin/middlewares/permission"
	"github.com/peggco/pegg/internal/llm"
	"github.com/peggco/pegg/internal/vfs"
)

func dbgReadTool(fs *vfs.VFS) tool.Tool {
	return tool.NewSpec("read", "read", nil, func(ctx context.Context, args string) (string, error) {
		var p struct {
			FilePath string `json:"filePath"`
		}
		if err := json.Unmarshal([]byte(args), &p); err != nil {
			return "", err
		}
		return fs.ReadCtx(ctx, p.FilePath)
	}).SetPermissionError(func(err error) bool {
		return err != nil && strings.Contains(err.Error(), "escapes the workspace")
	})
}

type dbgProvider struct {
	mu        int
	responses []*llm.Response
}

func (p *dbgProvider) Name() string { return "dbg" }
func (p *dbgProvider) ListModels(context.Context) ([]llm.Model, error) {
	return []llm.Model{{ID: "m"}}, nil
}
func (p *dbgProvider) Chat(_ context.Context, _ *llm.Request) (*llm.Response, error) {
	if p.mu >= len(p.responses) {
		return nil, errors.New("no more")
	}
	r := p.responses[p.mu]
	p.mu++
	return r, nil
}
func (p *dbgProvider) ChatStream(_ context.Context, _ *llm.Request, handler llm.StreamHandler) error {
	if p.mu >= len(p.responses) {
		return errors.New("no more")
	}
	r := p.responses[p.mu]
	p.mu++
	for _, c := range r.Choices {
		if c.Message == nil {
			continue
		}
		for _, tc := range c.Message.ToolCalls {
			if err := handler(llm.StreamChunk{ToolCalls: []llm.ToolCall{tc}}); err != nil {
				return err
			}
		}
		if text, ok := c.Message.Content.(string); ok && text != "" {
			if err := handler(llm.StreamChunk{Content: text}); err != nil {
				return err
			}
		}
	}
	return handler(llm.StreamChunk{FinishReason: llm.FinishReasonStop, IsDone: true})
}

func dbgToolCall(id, name, args string) *llm.Response {
	msg := llm.AssistantMessage("")
	msg.ToolCalls = []llm.ToolCall{{ID: id, Type: "function", Function: llm.Function{Name: name, Arguments: args}}}
	fr := llm.FinishReasonToolCalls
	return &llm.Response{Choices: []llm.Choice{{Message: &msg, FinishReason: &fr}}}
}

func dbgText(s string) *llm.Response {
	msg := llm.AssistantMessage(s)
	fr := llm.FinishReasonStop
	return &llm.Response{Choices: []llm.Choice{{Message: &msg, FinishReason: &fr}}}
}

func newDbgApp(t *testing.T, fill func(*dbgProvider, *vfs.VFS)) (*App, *vfs.VFS, *dbgProvider) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())

	fs, err := vfs.New(t.TempDir(), vfs.Options{})
	if err != nil {
		t.Fatal(err)
	}
	prov := &dbgProvider{}
	mw := permission.New(permission.Deps{})
	fs.OnAccessCheck(mw.AccessChecker)
	orch := agent.New("orch",
		agent.WithModel(llm.Model{ID: "m"}),
		agent.WithProvider(prov),
		agent.WithTool(dbgReadTool(fs)),
		agent.WithMiddleware(mw),
	)
	app, _ := newChatApp(t, orch)
	if fill != nil {
		fill(prov, fs)
	}
	go app.loop()
	return app, fs, prov
}

func outsideArgs(t *testing.T, fs *vfs.VFS, name string) string {
	t.Helper()
	outside := filepath.Join(filepath.Dir(fs.Root()), name)
	if err := os.WriteFile(outside, []byte("secret\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return `{"filePath": "` + outside + `"}`
}

func waitAskOpen(t *testing.T, app *App, want bool) {
	t.Helper()
	waitFor(t, 5*time.Second, func() bool {
		app.chatMu.Lock()
		defer app.chatMu.Unlock()
		return app.home.AskOpen() == want
	})
}

func collectItems(app *App) []string {
	app.chatMu.Lock()
	defer app.chatMu.Unlock()
	var items []string
	for _, it := range app.main.items {
		items = append(items, it.Text)
		if it.ToolErr != "" {
			items = append(items, "TOOLERR: "+it.ToolErr)
		}
	}
	return items
}

func TestPermissionPromptAllowApproves(t *testing.T) {
	app, _, _ := newDbgApp(t, func(prov *dbgProvider, fs *vfs.VFS) {
		prov.responses = []*llm.Response{
			dbgToolCall("c1", "read", outsideArgs(t, fs, "secret.txt")),
			dbgText("approved!"),
		}
	})
	app.submitMessage("read it")
	waitAskOpen(t, app, true)

	_ = app.screen.PostEvent(tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone))
	waitAskOpen(t, app, false)

	waitFor(t, 5*time.Second, func() bool {
		app.chatMu.Lock()
		defer app.chatMu.Unlock()
		return !app.running
	})
	items := collectItems(app)
	t.Logf("items: %v", items)
	if !strings.Contains(strings.Join(items, " "), "approved!") {
		t.Fatalf("expected approval flow: %v", items)
	}
	for _, it := range items {
		if strings.Contains(it, "permission denied") {
			t.Fatalf("unexpected denial: %v", items)
		}
	}
}

func TestPermissionPromptRejectCollectsReason(t *testing.T) {
	app, _, _ := newDbgApp(t, func(prov *dbgProvider, fs *vfs.VFS) {
		prov.responses = []*llm.Response{
			dbgToolCall("c1", "read", outsideArgs(t, fs, "secret.txt")),
			dbgText("done!"),
		}
	})
	app.submitMessage("read it")
	waitAskOpen(t, app, true)

	asks := make(chan int, 4)
	bus := app.bus
	bus.Subscribe(agent.TopicAgentAsk, func(e agent.AgentAsk) {
		asks <- len(e.Questions)
	})

	_ = app.screen.PostEvent(tcell.NewEventKey(tcell.KeyDown, 0, tcell.ModNone))
	_ = app.screen.PostEvent(tcell.NewEventKey(tcell.KeyDown, 0, tcell.ModNone))
	_ = app.screen.PostEvent(tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone))

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) && len(asks) < 1 {
		time.Sleep(10 * time.Millisecond)
	}
	waitAskOpen(t, app, true)

	for _, r := range "not today" {
		_ = app.screen.PostEvent(tcell.NewEventKey(tcell.KeyRune, r, tcell.ModNone))
	}
	time.Sleep(200 * time.Millisecond)
	_ = app.screen.PostEvent(tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone))

	waitFor(t, 5*time.Second, func() bool {
		app.chatMu.Lock()
		defer app.chatMu.Unlock()
		return !app.running
	})

	items := collectItems(app)
	t.Logf("items: %v", items)
	found := false
	for _, txt := range items {
		if strings.Contains(txt, "permission denied") && strings.Contains(txt, "not today") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected denial with typed reason: %v", items)
	}
}
