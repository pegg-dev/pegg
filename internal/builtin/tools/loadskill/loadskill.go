package loadskill

import (
	"context"
	"fmt"
	"strings"
	"sync"

	json "github.com/goccy/go-json"
	"github.com/google/uuid"

	"github.com/peggco/pegg/internal/agent"
	"github.com/peggco/pegg/internal/agent/prompt"
	"github.com/peggco/pegg/internal/agent/tool"
	"github.com/peggco/pegg/internal/agent/tools"
	"github.com/peggco/pegg/internal/builtin/tools/subagent"
	"github.com/peggco/pegg/internal/core/event"
	"github.com/peggco/pegg/internal/session"
	"github.com/peggco/pegg/internal/skill"
)

const contextFork = "fork"

func generateLoadSkillToolPrompt() (string, error) {
	sys, err := loadSkillPromptBuilder().
		Build(prompt.FormatMarkdown)
	if err != nil {
		return "", err
	}
	return sys, nil
}

type loadSkillParams struct {
	Name      string            `json:"name"`
	Arguments map[string]string `json:"arguments,omitempty"`
}

type agentSessions struct {
	mu       sync.Mutex
	sessions map[string]string
}

var sessions = &agentSessions{sessions: make(map[string]string)}

func (s *agentSessions) subscribe(bus event.Bus) {
	if bus == nil {
		return
	}
	_ = bus.Subscribe(session.TopicSessionAttached, func(e session.SessionAttached) {
		s.mu.Lock()
		defer s.mu.Unlock()
		s.sessions[e.AgentID] = e.SessionID
	})
}

func (s *agentSessions) get(agentID string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.sessions[agentID]
}

func LoadSkillTool(sess *session.Manager) {
	prompt, err := generateLoadSkillToolPrompt()
	if err != nil {
		panic(fmt.Sprintf("failed to generate loadskill tool prompt: %v", err))
	}
	if sess != nil {
		sessions.subscribe(sess.Bus())
	}

	tools.Register(tool.NewSpec(
		"loadskill",
		prompt,
		map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name": map[string]any{
					"type":        "string",
					"description": "The name of the skill to load (e.g. 'batch', 'review', 'init').",
				},
				"arguments": map[string]any{
					"type":        "object",
					"description": "Optional arguments to substitute into the skill content. Keys correspond to $arg_name placeholders in the skill instructions.",
					"additionalProperties": map[string]any{
						"type": "string",
					},
				},
			},
			"required": []string{"name"},
		},
		func(ctx context.Context, args string) (string, error) {
			var params loadSkillParams
			if err := json.Unmarshal([]byte(args), &params); err != nil {
				return "", fmt.Errorf("loadskill: invalid arguments: %w", err)
			}
			if strings.TrimSpace(params.Name) == "" {
				return "", fmt.Errorf("loadskill: name is required")
			}

			s, ok := skill.Get(params.Name)
			if !ok {
				return "", fmt.Errorf("loadskill: skill %q not found", params.Name)
			}

			content := skill.ExpandWithArgs(s, params.Arguments)
			if s.Context == contextFork {
				return executeFork(ctx, sess, s, content)
			}

			var b strings.Builder
			fmt.Fprintf(&b, "<skill:%s>\n", s.Name)
			if s.Description != "" {
				fmt.Fprintf(&b, "Description: %s\n", s.Description)
			}
			fmt.Fprintf(&b, "Path: %s\n", s.Path)
			if s.HasScripts {
				fmt.Fprintf(&b, "Scripts: %s\n", s.ScriptsPath)
			}
			if len(s.AllowedTools) > 0 {
				fmt.Fprintf(&b, "Allowed tools: %s\n", strings.Join(s.AllowedTools, ", "))
			}
			if s.WhenToUse != "" {
				fmt.Fprintf(&b, "When to use: %s\n", s.WhenToUse)
			}
			if s.ArgumentHint != "" {
				fmt.Fprintf(&b, "Arguments: %s\n", s.ArgumentHint)
			}
			if len(s.Arguments) > 0 {
				fmt.Fprintf(&b, "Parameters: %s\n", strings.Join(s.Arguments, ", "))
			}
			b.WriteString("\n")
			b.WriteString(strings.TrimSpace(content))
			fmt.Fprintf(&b, "\n</skill:%s>\n", s.Name)

			return b.String(), nil
		},
	))
}

func executeFork(ctx context.Context, sess *session.Manager, s *skill.Skill, content string) (string, error) {
	parent := agent.FromContext(ctx)
	if parent == nil {
		return "", fmt.Errorf("loadskill: no parent agent in context")
	}

	var sessionID string
	if sess != nil {
		if id := sessions.get(parent.ID); id != "" {
			msgs, err := sess.Messages(id)
			if err == nil && len(msgs) > 0 {
				forked, err := sess.Fork(id, msgs[len(msgs)-1].ID)
				if err != nil {
					return "", fmt.Errorf("loadskill: fork session: %w", err)
				}
				sessionID = forked.ID
			}
		}
	}

	name := fmt.Sprintf("%s-%s", s.Name, uuid.NewString()[:8])
	return subagent.SpawnForked(parent, name, sessionID, content)
}
