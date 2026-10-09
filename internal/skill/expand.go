package skill

import (
	"encoding/json"
	"regexp"
	"strings"

	"github.com/google/uuid"

	"github.com/peggco/pegg/internal/agent"
	"github.com/peggco/pegg/internal/llm"
)

var (
	skillRefRe = regexp.MustCompile(`(^|[^A-Za-z0-9_/:.-])(/[a-z0-9]+(?:-[a-z0-9]+)*)`)
	argRefRe   = regexp.MustCompile(`\$([a-zA-Z][a-zA-Z0-9_]*)`)
)

func ExpandMessage(in agent.MessageInput) agent.MessageInput {
	if !strings.Contains(in.Text, "/") {
		return in
	}
	in.Text = skillRefRe.ReplaceAllStringFunc(in.Text, func(m string) string {
		parts := skillRefRe.FindStringSubmatch(m)
		if len(parts) != 3 {
			return m
		}
		prefix, token := parts[1], parts[2]
		s, ok := Get(strings.TrimPrefix(token, "/"))
		if !ok {
			return m
		}
		args, err := json.Marshal(map[string]string{"name": s.Name})
		if err != nil {
			return m
		}
		in.Calls = append(in.Calls, llm.ToolCall{
			ID:   uuid.NewString(),
			Type: "function",
			Function: llm.Function{
				Name:      "loadskill",
				Arguments: string(args),
			},
		})
		return prefix
	})
	return in
}

func ExpandWithArgs(s *Skill, args map[string]string) string {
	if s == nil {
		return ""
	}
	content := s.Instructions
	if len(args) > 0 {
		content = argRefRe.ReplaceAllStringFunc(content, func(m string) string {
			parts := argRefRe.FindStringSubmatch(m)
			if len(parts) != 2 {
				return m
			}
			if val, ok := args[parts[1]]; ok {
				return val
			}
			return m
		})
	}
	return content
}
