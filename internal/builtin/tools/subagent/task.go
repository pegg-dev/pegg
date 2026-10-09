package subagent

import (
	"context"
	"fmt"
	"strings"

	json "github.com/goccy/go-json"

	"github.com/peggco/pegg/internal/agent"
	"github.com/peggco/pegg/internal/agent/agents"
	"github.com/peggco/pegg/internal/agent/prompt"
	"github.com/peggco/pegg/internal/agent/reminder"
	"github.com/peggco/pegg/internal/agent/tool"
	"github.com/peggco/pegg/internal/llm"
	"github.com/peggco/pegg/internal/session"
)

func backgroundReminder(name string) reminder.Reminder {
	content := fmt.Sprintf("Subagent %q is working in the background. You may continue your own work or finish your turn; the system will wake you up when the subagent finishes. Do NOT wait for it.", name)
	return reminder.New("background_subagent", content, "agent", name)
}

func agentOutput(res *agent.RunResult, runErr error) string {
	if res != nil && strings.TrimSpace(res.Output) != "" {
		return res.Output
	}
	if res != nil {
		var b strings.Builder
		for _, m := range res.History {
			if m.Role != llm.RoleAssistant {
				continue
			}
			if c, ok := m.Content.(string); ok && strings.TrimSpace(c) != "" {
				b.WriteString(c)
				b.WriteString("\n")
			}
		}
		if s := strings.TrimSpace(b.String()); s != "" {
			return s
		}
	}
	if runErr != nil {
		return runErr.Error()
	}
	return ""
}

func generateTaskToolPrompt(agents []string) (string, error) {
	sys, err := taskToolPromptBuilder(agents).
		Build(prompt.FormatMarkdown)
	if err != nil {
		return "", err
	}
	return sys, nil
}

func subAgentTool() tool.Tool {
	agentsList := agents.List()

	prompt, err := generateTaskToolPrompt(agentsList)
	if err != nil {
		panic(fmt.Sprintf("failed to generate task tool prompt: %v", err))
	}

	return tool.NewSpec(
		"task",
		prompt,
		map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name": map[string]any{
					"type":        "string",
					"description": "Unique name for this subagent run. Must describe the task. If a subagent with this name already exists and has finished, it will be resumed with its existing context and conversation history.",
				},
				"subagent_type": map[string]any{
					"type":        "string",
					"enum":        agentsList,
					"description": "The type of specialized agent to use for this task.",
				},
				"prompt": map[string]any{
					"type":        "string",
					"description": "The task for the agent to perform.",
				},
				"task_id": map[string]any{
					"type":        "array",
					"items":       map[string]any{"type": "string"},
					"description": "Optional todo ID(s) this subagent is working on (e.g. ['todo-1', 'todo-3']). Used to track which tasks the subagent is handling, enabling the user to see task progress and agent assignments.",
				},
				"background": map[string]any{
					"type":        "boolean",
					"description": "Run the agent in the background. When it completes, a system reminder will be appended to your context automatically. You do NOT need to wait or poll for results. You can continue your own work and will be notified when the subagent finishes. (default false)",
				},
			},
			"required": []string{"name", "subagent_type", "prompt"},
		},
		func(ctx context.Context, args string) (string, error) {
			var params struct {
				Name         string   `json:"name"`
				SubagentType string   `json:"subagent_type"`
				Prompt       string   `json:"prompt"`
				TaskIDs      []string `json:"task_id"`
				Background   bool     `json:"background"`
			}
			if err := json.Unmarshal([]byte(args), &params); err != nil {
				return "", fmt.Errorf("subagent: invalid arguments: %w", err)
			}

			parent := agent.FromContext(ctx)
			if parent == nil {
				return "", fmt.Errorf("subagent: no parent agent in context")
			}
			if parent.Provider == nil {
				return "", fmt.Errorf("subagent: parent agent has no provider configured")
			}

			if strings.TrimSpace(params.Name) == "" {
				return "", fmt.Errorf("subagent: name is required")
			}
			if strings.TrimSpace(params.Prompt) == "" {
				return "", fmt.Errorf("subagent: task is required")
			}
			if !agents.Has(params.SubagentType) {
				return "", fmt.Errorf("subagent: unknown agent %q (available: %s)", params.SubagentType, strings.Join(agentsList, ", "))
			}

			if existing, ok := store.get(params.Name, parent); ok {
				if existing.Status == StatusRunning || existing.Status == StatusPending {
					return "", fmt.Errorf("subagent: %q is still running", params.Name)
				}
				sa, err := store.resume(params.Name, parent)
				if err != nil {
					return "", err
				}
				if parent.Bus != nil {
					store.subscribe(parent.Bus)
				}

				runAgent := func(runCtx context.Context) {
					sub, err := agents.New(params.SubagentType)
					if err != nil {
						store.finish(sa, "", err)
						return
					}
					sub.SetModelProvider(parent.Model, parent.Provider)
					sub.RouterManaged = parent.RouterManaged
					sub.ParentAgentID = parent.ID
					sub.DisplayName = sa.Name

					var history []llm.Message
					if sa.SessionID != "" && sessionReader != nil {
						sessMsgs, readErr := sessionReader.Messages(sa.SessionID)
						if readErr == nil && len(sessMsgs) > 0 {
							if sub.SystemPrompt != "" {
								history = append(history, llm.SystemMessage(sub.SystemPrompt))
							}
							history = append(history, session.MessagesToLLM(sessMsgs)...)
						}
					}

					if parent.Bus != nil {
						sub.Bus = parent.Bus
						store.setAgentID(sa.Name, sub.ID)
						parent.Bus.Publish(session.TopicSessionResume, session.SessionResume{
							AgentID:   sub.ID,
							SessionID: sa.SessionID,
						})
						res, runErr := sub.Resume(runCtx, params.Prompt, history)
						store.finish(sa, agentOutput(res, runErr), runErr)
						return
					}
					store.start(sa)
					res, runErr := sub.Resume(runCtx, params.Prompt, history)
					store.finish(sa, agentOutput(res, runErr), runErr)
				}

				if params.Background {
					parent.AttachReminder(backgroundReminder(params.Name))
					go runAgent(context.Background())
					return fmt.Sprintf("Resumed subagent %q in background with existing context.\n", params.Name), nil
				}

				go runAgent(ctx)

				select {
				case <-sa.done:
				case <-ctx.Done():
					return "", fmt.Errorf("subagent: %w", ctx.Err())
				}

				done, _ := store.get(sa.Name, parent)
				if done.Status == StatusFailed {
					return fmt.Sprintf("Subagent %q failed: %s\n", done.Name, done.Err), nil
				}
				return fmt.Sprintf("Subagent %q finished.\nResponse:\n%s\n", done.Name, done.Output), nil
			}

			if parent.Bus != nil {
				store.subscribe(parent.Bus)
			}

			sa, err := store.spawn(params.Name, params.SubagentType, params.TaskIDs, params.Background, parent)
			if err != nil {
				return "", err
			}

			runAgent := func(runCtx context.Context) {
				sub, err := agents.New(params.SubagentType)
				if err != nil {
					store.finish(sa, "", err)
					return
				}
				sub.SetModelProvider(parent.Model, parent.Provider)
				sub.RouterManaged = parent.RouterManaged
				sub.ParentAgentID = parent.ID
				sub.DisplayName = sa.Name
				if parent.Bus != nil {
					sub.Bus = parent.Bus
					store.setAgentID(sa.Name, sub.ID)
					res, runErr := sub.Run(runCtx, params.Prompt)
					store.finish(sa, agentOutput(res, runErr), runErr)
					return
				}
				store.start(sa)
				res, runErr := sub.Run(runCtx, params.Prompt)
				store.finish(sa, agentOutput(res, runErr), runErr)
			}

			if params.Background {
				parent.AttachReminder(backgroundReminder(params.Name))
				go runAgent(context.Background())
				return fmt.Sprintf("Started subagent in background: %s\n", params.Name), nil
			}

			go runAgent(ctx)

			select {
			case <-sa.done:
			case <-ctx.Done():
				return "", fmt.Errorf("subagent: %w", ctx.Err())
			}

			done, _ := store.get(sa.Name, parent)
			if done.Status == StatusFailed {
				return fmt.Sprintf("Subagent %q failed: %s\n", done.Name, done.Err), nil
			}

			return fmt.Sprintf("Subagent %q finished.\nResponse:\n%s\n", done.Name, done.Output), nil
		},
	)
}
