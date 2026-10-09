package ask

import (
	"context"
	"fmt"
	"strings"
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
								"enum":        []string{"text", "select", "multiselect", "boolean"},
								"description": "'text' free-form input; 'select' pick one option; 'multiselect' pick one or more options; 'boolean' yes/no.",
							},
							"options": map[string]any{
								"type":        "array",
								"items":       map[string]any{"type": "string"},
								"description": "Required for 'select' and 'multiselect'. Ignored for other types.",
							},
							"required": map[string]any{
								"type":        "boolean",
								"description": "If true, the user must provide an answer.",
							},
						},
						"required": []string{"question", "type"},
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

const askExpectedJSON = `{"questions":[{"id":"q1","question":"...","type":"text","required":true},{"id":"q2","question":"...","type":"select","options":["a","b"]},{"id":"q3","question":"...","type":"multiselect","options":["a","b"]},{"id":"q4","question":"...","type":"boolean"}]}`

func askError(reason string) error {
	return fmt.Errorf("askuserquestion: %s. Expected JSON: %s", reason, askExpectedJSON)
}

func parseAskArgs(args string) ([]agent.AskQuestion, error) {
	var params askParams
	if err := json.Unmarshal([]byte(args), &params); err != nil {
		return nil, askError(fmt.Sprintf("invalid arguments: %v", err))
	}
	return validateQuestions(params.Questions)
}

func validateQuestions(questions []agent.AskQuestion) ([]agent.AskQuestion, error) {
	if len(questions) == 0 {
		return nil, askError("questions array must not be empty")
	}
	for i := range questions {
		q := &questions[i]
		if strings.TrimSpace(q.ID) == "" {
			q.ID = fmt.Sprintf("q%d", i+1)
		}
		if strings.TrimSpace(q.Question) == "" {
			return nil, askError(fmt.Sprintf("questions[%d].question is required", i))
		}
		switch typ := strings.ToLower(strings.TrimSpace(q.Type)); typ {
		case "text", "boolean":
			q.Type = typ
		case "select", "multiselect":
			if len(q.Options) == 0 {
				return nil, askError(fmt.Sprintf("questions[%d] has type %q but no options", i, typ))
			}
			q.Type = typ
		default:
			return nil, askError(fmt.Sprintf("questions[%d].type must be one of \"text\", \"select\", \"multiselect\", \"boolean\", got %q", i, q.Type))
		}
	}
	return questions, nil
}

func executeAsk(ctx context.Context, args string) (string, error) {
	questions, err := parseAskArgs(args)
	if err != nil {
		return "", err
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
		Questions: questions,
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
