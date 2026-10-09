package permission

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/peggco/pegg/internal/agent"
	"github.com/peggco/pegg/internal/llm"
)

const (
	decisionAllow  = "allow"
	decisionReject = "reject"
)

type decision struct {
	Allow   bool
	Persist bool
	Reason  string
}

func (m *Middleware) ask(ctx context.Context, call llm.ToolCall, mode Mode, permErr error) (*decision, error) {
	if mode == ModeJudge || mode == ModeSemiJudge {
		return m.askJudge(ctx, call, permErr)
	}
	return m.askUser(ctx, call, permErr)
}

func buildQuestion(call llm.ToolCall, permErr error) agent.AskQuestion {
	var b strings.Builder
	fmt.Fprintf(&b, "Tool %q requested permission.\n", call.Function.Name)
	if permErr != nil {
		fmt.Fprintf(&b, "The sandbox denied the operation: %s\n", permErr.Error())
	}
	b.WriteString("Arguments:\n")
	b.WriteString(call.Function.Arguments)
	return agent.AskQuestion{
		ID:       "decision",
		Question: b.String(),
		Type:     "select",
		Options:  []string{"Allow", "Allow All", "Reject"},
		Required: true,
	}
}

func (m *Middleware) askUser(ctx context.Context, call llm.ToolCall, permErr error) (*decision, error) {
	a := agent.FromContext(ctx)
	if a == nil || a.Bus == nil {
		return nil, errors.New("no agent bus available")
	}
	if !a.Bus.HasCallback(agent.TopicAgentAsk) {
		return nil, errors.New("no interactive prompt available")
	}

	answers, err := m.prompt(ctx, a, buildQuestion(call, permErr))
	if err != nil {
		return nil, err
	}
	switch answers["decision"] {
	case "Allow":
		return &decision{Allow: true}, nil
	case "Allow All":
		return &decision{Allow: true, Persist: true}, nil
	case "Reject":
		reason := ""
		reasonAnswers, err := m.prompt(ctx, a, agent.AskQuestion{
			ID:       "reason",
			Question: fmt.Sprintf("Reason for rejecting the %q tool call (optional):", call.Function.Name),
			Type:     "text",
		})
		if err == nil {
			reason = strings.TrimSpace(reasonAnswers["reason"])
		}
		if reason == "" {
			reason = "user rejected the tool call"
		}
		return &decision{Allow: false, Reason: reason}, nil
	default:
		return &decision{Allow: false, Reason: "prompt dismissed by user"}, nil
	}
}

func (m *Middleware) prompt(ctx context.Context, a *agent.Agent, q agent.AskQuestion) (map[string]string, error) {
	var (
		once    sync.Once
		done    = make(chan map[string]string, 1)
		replyFn any
	)
	replyFn = func(e agent.AgentAskAnswer) {
		if e.AgentID != a.ID {
			return
		}
		once.Do(func() {
			done <- e.Answers
		})
	}
	if err := a.Bus.SubscribeOnce(agent.TopicAgentAskAnswer, replyFn); err != nil {
		return nil, fmt.Errorf("permission: subscribe for answer: %w", err)
	}
	defer a.Bus.Unsubscribe(agent.TopicAgentAskAnswer, replyFn)

	a.Bus.Publish(agent.TopicAgentAsk, agent.AgentAsk{
		AgentID:   a.ID,
		AgentName: a.Name,
		Questions: []agent.AskQuestion{q},
	})

	select {
	case answers := <-done:
		if answers == nil {
			answers = map[string]string{}
		}
		return answers, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
