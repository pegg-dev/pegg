package schedule

import (
	"context"
	"fmt"
	"strings"

	json "github.com/goccy/go-json"

	"github.com/peggco/pegg/internal/agent/prompt"
	"github.com/peggco/pegg/internal/agent/tool"
	"github.com/peggco/pegg/internal/agent/tools"
)

var manager *Manager

func ScheduleTools(mgr *Manager) {
	if mgr != nil {
		manager = mgr
	}
	_ = tools.Register(scheduleTool())
}

func generateScheduleToolPrompt() (string, error) {
	return scheduleToolPromptBuilder().Build(prompt.FormatMarkdown)
}

func scheduleTool() tool.Tool {
	promptText, err := generateScheduleToolPrompt()
	if err != nil {
		panic(fmt.Sprintf("failed to generate schedule tool prompt: %v", err))
	}
	return tool.NewSpec(
		"schedule",
		promptText,
		map[string]any{
			"type": "object",
			"properties": map[string]any{
				"action": map[string]any{
					"type":        "string",
					"enum":        []string{"create", "list", "get", "update", "delete", "run"},
					"description": "Operation to perform.",
				},
				"id": map[string]any{
					"type":        "string",
					"description": "Schedule id (required for get, update, delete and run).",
				},
				"name": map[string]any{
					"type":        "string",
					"description": "Human readable schedule name (create/update).",
				},
				"cron": map[string]any{
					"type":        "string",
					"description": "Cron expression, e.g. '0 9 * * MON-FRI' or '*/30 * * * *' (create/update).",
				},
				"prompt": map[string]any{
					"type":        "string",
					"description": "Prompt sent to the agent as a user message on each run (create/update).",
				},
				"workspace": map[string]any{
					"type":        "string",
					"description": "Absolute project directory the run executes in. Defaults to the current directory (create/update).",
				},
				"provider": map[string]any{
					"type":        "string",
					"description": "Optional provider override for the runs (create/update).",
				},
				"model": map[string]any{
					"type":        "string",
					"description": "Optional model override for the runs (create/update).",
				},
				"tags": map[string]any{
					"type":        "array",
					"items":       map[string]any{"type": "string"},
					"description": "Optional tags (create/update) or filter (list).",
				},
				"timeout": map[string]any{
					"type":        "integer",
					"minimum":     0,
					"description": "Optional timeout in seconds for each run; 0 disables (create/update).",
				},
				"max_parallel": map[string]any{
					"type":        "integer",
					"minimum":     1,
					"description": "Maximum concurrent runs of this schedule; default 1 (create/update).",
				},
				"enabled": map[string]any{
					"type":        "boolean",
					"description": "Enable or disable the schedule. Set false to pause, true to resume (update).",
				},
				"disabled": map[string]any{
					"type":        "boolean",
					"description": "Create the schedule paused (create).",
				},
			},
			"required": []string{"action"},
		},
		executeSchedule,
	)
}

type scheduleArgs struct {
	Action      string    `json:"action"`
	ID          string    `json:"id"`
	Name        *string   `json:"name"`
	Cron        *string   `json:"cron"`
	Prompt      *string   `json:"prompt"`
	Workspace   *string   `json:"workspace"`
	Provider    *string   `json:"provider"`
	Model       *string   `json:"model"`
	Tags        *[]string `json:"tags"`
	Timeout     *int      `json:"timeout"`
	MaxParallel *int      `json:"max_parallel"`
	Enabled     *bool     `json:"enabled"`
	Disabled    *bool     `json:"disabled"`
}

func executeSchedule(_ context.Context, args string) (string, error) {
	var p scheduleArgs
	if err := json.Unmarshal([]byte(args), &p); err != nil {
		return "", fmt.Errorf("schedule: invalid arguments: %w", err)
	}
	if manager == nil {
		return "", fmt.Errorf("schedule: scheduler is not available")
	}

	switch strings.ToLower(strings.TrimSpace(p.Action)) {
	case "create":
		return scheduleCreate(p)
	case "list":
		return scheduleList(p)
	case "get":
		return scheduleGet(p)
	case "update":
		return scheduleUpdate(p)
	case "delete":
		return scheduleDelete(p)
	case "run":
		return scheduleRun(p)
	case "":
		return "", fmt.Errorf("schedule: action is required")
	default:
		return "", fmt.Errorf("schedule: unknown action %q", p.Action)
	}
}

func scheduleCreate(p scheduleArgs) (string, error) {
	if p.Name == nil || strings.TrimSpace(*p.Name) == "" {
		return "", fmt.Errorf("schedule: name is required for create")
	}
	if p.Cron == nil || strings.TrimSpace(*p.Cron) == "" {
		return "", fmt.Errorf("schedule: cron is required for create")
	}
	if p.Prompt == nil || strings.TrimSpace(*p.Prompt) == "" {
		return "", fmt.Errorf("schedule: prompt is required for create")
	}
	opts := CreateOptions{
		Name:   *p.Name,
		Cron:   *p.Cron,
		Prompt: *p.Prompt,
	}
	if p.Workspace != nil {
		opts.Workspace = *p.Workspace
	}
	if p.Provider != nil {
		opts.Provider = *p.Provider
	}
	if p.Model != nil {
		opts.Model = *p.Model
	}
	if p.Tags != nil {
		opts.Tags = *p.Tags
	}
	if p.Timeout != nil {
		opts.TimeoutSecs = *p.Timeout
	}
	if p.MaxParallel != nil {
		opts.MaxParallel = *p.MaxParallel
	}
	if p.Disabled != nil {
		opts.Disabled = *p.Disabled
	}
	s, err := manager.Create(opts)
	if err != nil {
		return "", err
	}
	return marshalSchedule(s), nil
}

func scheduleList(p scheduleArgs) (string, error) {
	var filter []string
	if p.Tags != nil {
		filter = *p.Tags
	}
	list := manager.List()
	out := make([]Schedule, 0, len(list))
	for _, s := range list {
		if len(filter) > 0 && !hasAnyTag(s.Tags, filter) {
			continue
		}
		out = append(out, s)
	}
	data, err := json.Marshal(out)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func scheduleGet(p scheduleArgs) (string, error) {
	if err := requireID(p.ID, "get"); err != nil {
		return "", err
	}
	s, err := manager.Get(p.ID)
	if err != nil {
		return "", err
	}
	return marshalSchedule(s), nil
}

func scheduleUpdate(p scheduleArgs) (string, error) {
	if err := requireID(p.ID, "update"); err != nil {
		return "", err
	}
	s, err := manager.Update(p.ID, UpdateOptions{
		Name:        p.Name,
		Cron:        p.Cron,
		Prompt:      p.Prompt,
		Workspace:   p.Workspace,
		Provider:    p.Provider,
		Model:       p.Model,
		Tags:        p.Tags,
		TimeoutSecs: p.Timeout,
		MaxParallel: p.MaxParallel,
		Enabled:     p.Enabled,
	})
	if err != nil {
		return "", err
	}
	return marshalSchedule(s), nil
}

func scheduleDelete(p scheduleArgs) (string, error) {
	if err := requireID(p.ID, "delete"); err != nil {
		return "", err
	}
	if err := manager.Delete(p.ID); err != nil {
		return "", err
	}
	return marshalStatus("deleted", p.ID), nil
}

func scheduleRun(p scheduleArgs) (string, error) {
	if err := requireID(p.ID, "run"); err != nil {
		return "", err
	}
	s, err := manager.Get(p.ID)
	if err != nil {
		return "", err
	}
	if err := manager.Trigger(p.ID); err != nil {
		return "", err
	}
	return marshalStatus("triggered", s.ID), nil
}

func requireID(id, action string) error {
	if strings.TrimSpace(id) == "" {
		return fmt.Errorf("schedule: id is required for %s", action)
	}
	return nil
}

func marshalSchedule(s Schedule) string {
	data, err := json.Marshal(s)
	if err != nil {
		return ""
	}
	return string(data)
}

func marshalStatus(status, id string) string {
	data, err := json.Marshal(map[string]string{"status": status, "id": id})
	if err != nil {
		return ""
	}
	return string(data)
}

func hasAnyTag(have, want []string) bool {
	for _, w := range want {
		for _, h := range have {
			if strings.EqualFold(strings.TrimSpace(h), strings.TrimSpace(w)) {
				return true
			}
		}
	}
	return false
}
