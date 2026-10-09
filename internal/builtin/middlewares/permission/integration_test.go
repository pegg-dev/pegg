package permission

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	json "github.com/goccy/go-json"

	"github.com/peggco/pegg/internal/agent"
	"github.com/peggco/pegg/internal/agent/tool"
	"github.com/peggco/pegg/internal/core/event"
	"github.com/peggco/pegg/internal/core/logger"
	"github.com/peggco/pegg/internal/llm"
	"github.com/peggco/pegg/internal/vfs"
)

type discardHandler struct{}

func (discardHandler) Write(logger.Record) error { return nil }
func (discardHandler) Close() error              { return nil }

func readToolCtx(fs *vfs.VFS) tool.Tool {
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

type scriptedProvider struct {
	mu        int
	responses []*llm.Response
}

func (p *scriptedProvider) Name() string { return "echo" }

func (p *scriptedProvider) ListModels(context.Context) ([]llm.Model, error) {
	return []llm.Model{{ID: "mock"}}, nil
}

func (p *scriptedProvider) Chat(_ context.Context, _ *llm.Request) (*llm.Response, error) {
	if p.mu >= len(p.responses) {
		return nil, errors.New("echo: no more responses")
	}
	resp := p.responses[p.mu]
	p.mu++
	return resp, nil
}

func (p *scriptedProvider) ChatStream(context.Context, *llm.Request, llm.StreamHandler) error {
	return errors.New("not used")
}

func textResponse(s string) *llm.Response {
	msg := llm.AssistantMessage(s)
	fr := llm.FinishReasonStop
	return &llm.Response{Choices: []llm.Choice{{Message: &msg, FinishReason: &fr}}}
}

func toolCallResponse(id, name, args string) *llm.Response {
	msg := llm.AssistantMessage("")
	msg.ToolCalls = []llm.ToolCall{{ID: id, Type: "function", Function: llm.Function{Name: name, Arguments: args}}}
	fr := llm.FinishReasonToolCalls
	return &llm.Response{Choices: []llm.Choice{{Message: &msg, FinishReason: &fr}}}
}

func TestAgentRunPermissionFlow(t *testing.T) {
	fs, err := vfs.New(t.TempDir(), vfs.Options{})
	if err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(filepath.Dir(fs.Root()), "secret.txt")
	if err := os.WriteFile(outside, []byte("top secret\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	argsA := `{"filePath": "` + outside + `"}`
	outsideB := filepath.Join(filepath.Dir(fs.Root()), "other.txt")
	if err := os.WriteFile(outsideB, []byte("more secret\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	argsB := `{"filePath": "` + outsideB + `"}`

	bus := event.New()
	askCount := 0
	bus.Subscribe(agent.TopicAgentAsk, func(e agent.AgentAsk) {
		askCount++
		if len(e.Questions) == 0 {
			return
		}
		q := e.Questions[0]
		switch {
		case q.ID == "reason":
			bus.Publish(agent.TopicAgentAskAnswer, agent.AgentAskAnswer{AgentID: e.AgentID, Answers: map[string]string{"reason": "first time no"}})
		case askCount == 1:
			bus.Publish(agent.TopicAgentAskAnswer, agent.AgentAskAnswer{AgentID: e.AgentID, Answers: map[string]string{"decision": "Reject"}})
		default:
			bus.Publish(agent.TopicAgentAskAnswer, agent.AgentAskAnswer{AgentID: e.AgentID, Answers: map[string]string{"decision": "Allow"}})
		}
	})

	prov := &scriptedProvider{responses: []*llm.Response{
		toolCallResponse("c1", "read", argsA),
		toolCallResponse("c2", "read", argsA),
		toolCallResponse("c3", "read", argsB),
		textResponse("done"),
	}}

	a := agent.New("orch",
		agent.WithModel(llm.Model{ID: "mock"}),
		agent.WithProvider(prov),
		agent.WithTool(readToolCtx(fs)),
		agent.WithMiddleware(testMiddleware(t, Deps{})),
		agent.WithBus(bus),
		agent.WithLogger(logger.New(logger.LevelDebug, discardHandler{})),
	)

	res, err := a.Run(context.Background(), "read the file")
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if res.Output != "done" {
		t.Fatalf("output = %q, want done", res.Output)
	}
	if askCount != 3 {
		t.Fatalf("asks = %d, want 3 (decision + reason; repeat call auto-rejected)", askCount)
	}

	var denials, approvals int
	for _, m := range res.History {
		if m.Role != llm.RoleTool {
			continue
		}
		text, _ := m.Content.(string)
		if strings.HasPrefix(text, "Error: permission denied for tool \"read\"") {
			denials++
			continue
		}
		if text != "" {
			approvals++
		}
	}
	if denials != 2 {
		t.Fatalf("denials in history = %d, want 2 (rejected + auto-rejected repeat)", denials)
	}
	if approvals != 1 {
		t.Fatalf("approved tool results in history = %d, want 1", approvals)
	}
}
