package ask

import (
	"context"
	"fmt"
	"sync"

	json "github.com/goccy/go-json"

	"github.com/peggco/pegg/internal/agent"
	"github.com/peggco/pegg/internal/agent/prompt"
	"github.com/peggco/pegg/internal/agent/tool"
	"github.com/peggco/pegg/internal/agent/tools"
)

func generateAskToolPrompt() (string, error) {
	sys, err := askToolPromptBuilder().
		Build(prompt.FormatMarkdown)
	if err != nil {
		return "", err
	}
	return sys, nil
}

func AskTool() {
	prompt, err := generateAskToolPrompt()
	if err != nil {
		panic(fmt.Sprintf("failed to generate ask tool prompt: %v", err))
	}

	tools.Register(tool.NewSpec(
		"askuserquestion",
		prompt,
		map[string]any{
			"type": "object",
			"properties": map[string]any{
				"questions": map[string]any{
					"type":        "array",
					"description": "One or more questions to ask the user.",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"id": map[string]any{
								"type":        "string",
								"description": "Unique identifier for this question. Used to map answers back.",
							},
							"question": map[string]any{
								"type":        "string",
								"description": "The question text to show the user.",
							},
							"type": map[string]any{
								"type":        "string",
								"enum":        []string{"text", "select"},
								"description": "'text' for free-form input, 'select' for choosing from options.",
							},
							"options": map[string]any{
								"type":        "array",
								"items":       map[string]any{"type": "string"},
								"description": "Required for 'select' type. List of options to choose from.",
							},
							"required": map[string]any{
								"type":        "boolean",
								"description": "If true, the user must provide an answer.",
							},
						},
						"required": []string{"id", "question", "type"},
					},
				},
			},
			"required": []string{"questions"},
		},
		executeAsk,
	))
}

type askParams struct {
	Questions []agent.AskQuestion `json:"questions"`
}

func executeAsk(ctx context.Context, args string) (string, error) {
	var params askParams
	if err := json.Unmarshal([]byte(args), &params); err != nil {
		return "", fmt.Errorf("askuserquestion: invalid arguments: %w", err)
	}
	if len(params.Questions) == 0 {
		return "", fmt.Errorf("askuserquestion: questions array must not be empty")
	}
	for i := range params.Questions {
		q := &params.Questions[i]
		if q.ID == "" {
			q.ID = fmt.Sprintf("q%d", i+1)
		}
		if q.Question == "" {
			return "", fmt.Errorf("askuserquestion: question text is required for all entries")
		}
		if q.Type == "select" && len(q.Options) == 0 {
			return "", fmt.Errorf("askuserquestion: select questions must provide options")
		}
	}

	parent := agent.FromContext(ctx)
	if parent == nil {
		return "", fmt.Errorf("askuserquestion: no parent agent in context")
	}
	if parent.Bus == nil {
		return "", fmt.Errorf("askuserquestion: agent has no event bus")
	}

	var (
		once    sync.Once
		done    = make(chan map[string]string, 1)
		replyFn any
	)

	replyFn = func(e agent.AgentAskAnswer) {
		if e.AgentID != parent.ID {
			return
		}
		once.Do(func() {
			done <- e.Answers
		})
	}

	if err := parent.Bus.SubscribeOnce(agent.TopicAgentAskAnswer, replyFn); err != nil {
		return "", fmt.Errorf("askuserquestion: subscribe for answer: %w", err)
	}
	defer parent.Bus.Unsubscribe(agent.TopicAgentAskAnswer, replyFn)

	parent.Bus.Publish(agent.TopicAgentAsk, agent.AgentAsk{
		AgentID:   parent.ID,
		AgentName: parent.Name,
		Questions: params.Questions,
	})

	select {
	case answers := <-done:
		if len(answers) == 0 {
			return "{}", nil
		}
		b, err := json.Marshal(map[string]any{"answers": answers})
		if err != nil {
			return "", fmt.Errorf("askuserquestion: marshal answers: %w", err)
		}
		return string(b), nil
	case <-ctx.Done():
		return "", ctx.Err()
	}
}
