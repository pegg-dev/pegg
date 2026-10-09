package permission

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/peggco/pegg/internal/agent"
	"github.com/peggco/pegg/internal/agent/prompt"
	"github.com/peggco/pegg/internal/core/config"
	decisionapi "github.com/peggco/pegg/internal/decision"
	"github.com/peggco/pegg/internal/llm"
)

var (
	errJudgeUnconfigured = errors.New("permission: judge provider/model not configured")
	errJudgeModelMissing = errors.New("permission: judge model not found")
)

var judgeVerdictSchema = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"allow":  map[string]any{"type": "boolean"},
		"reason": map[string]any{"type": "string"},
	},
	"required":             []string{"allow", "reason"},
	"additionalProperties": false,
}

func (m *Middleware) resolveJudge() (llm.Provider, llm.Model, bool) {
	if m.llm == nil {
		return nil, llm.Model{}, false
	}
	var providerName, modelID string
	if m.cfg != nil {
		providerName, modelID = m.cfg.JudgeProvider, m.cfg.JudgeModel
	}

	if providerName != "" && modelID != "" {
		if prov, err := m.llm.Provider(providerName); err == nil {
			res := m.llm.Select(llm.SelectRequest{
				Mode:     llm.SelectModeExact,
				Provider: providerName,
				Model:    modelID,
			})
			if res.Err == nil && res.Model.ID != "" {
				return prov, res.Model, true
			}
		}
	}

	res := m.llm.Select(llm.SelectRequest{Mode: llm.SelectModePreferred})
	if res.Err != nil || res.Model.ID == "" {
		return nil, llm.Model{}, false
	}
	prov, err := m.llm.Provider(res.Provider)
	if err != nil {
		return nil, llm.Model{}, false
	}
	return prov, res.Model, true
}

func (m *Middleware) resolveDecisionJudge() (decisionapi.Provider, string, bool) {
	if m.decision == nil {
		return nil, "", false
	}
	var model string
	if m.cfg != nil {
		model = m.cfg.JudgeModel
	}
	if m.cfg != nil && m.cfg.JudgeProvider != "" {
		prov, err := m.decision.Provider(m.cfg.JudgeProvider)
		if err != nil {
			return nil, "", false
		}
		return prov, model, true
	}
	prov, err := m.decision.Preferred()
	if err != nil {
		return nil, "", false
	}
	return prov, model, true
}

func generateJudgeSystemPrompt() (string, error) {
	return prompt.New().
		Paragraph("You are a permission judge for an AI coding assistant. A tool call is proposed. You decide whether it should be allowed.").
		XMLTag("task",
			prompt.Paragraph("You will be given a tool call the assistant wants to perform."),
			prompt.Paragraph("Decide whether the operation should be allowed."),
			prompt.Paragraph("Respond using the structured output schema: a JSON object with two fields."),
			prompt.KV("allow", "boolean: true if the call should be allowed, false otherwise"),
			prompt.KV("reason", "string: a short justification. REQUIRED and must be non-empty when allow is false")).
		XMLTag("rules",
			prompt.List("Read-only and low-risk operations are usually allowed",
				"Destructive, secret-exposing, or out-of-scope operations should be denied",
				"If a sandbox denial is included, the user has already been asked to approve the operation; judge whether it is acceptable anyway",
				"Always provide a reason, even for approvals")).
		Build(prompt.FormatMarkdown)
}

type judgeVerdict struct {
	Allow  bool   `json:"allow"`
	Reason string `json:"reason"`
}

func newJudgeAgent(prov llm.Provider, model llm.Model) *agent.Agent {
	sys, err := generateJudgeSystemPrompt()
	if err != nil {
		sys = "You are a permission judge for an AI coding assistant."
	}
	return agent.New("judge",
		agent.WithProvider(prov),
		agent.WithModel(model),
		agent.WithSystemPrompt(sys),
		agent.WithMaxIterations(1),
		agent.WithStructuredOutput("judge_verdict", judgeVerdictSchema),
	)
}

const judgeHistoryMax = 5

func buildJudgePrompt(call llm.ToolCall, permErr error, history []llm.Message) (string, error) {
	p := prompt.New().
		Paragraph("A tool call requires permission approval. Decide whether it should be allowed.").
		XMLTag("tool",
			prompt.KV("name", call.Function.Name),
			prompt.KV("arguments", call.Function.Arguments))
	if ctx := formatHistory(history); ctx != "" {
		p = p.XMLTag("context", prompt.Raw(ctx))
	}
	if permErr != nil {
		p = p.XMLTag("sandbox-denial", prompt.Paragraph(permErr.Error()))
	}
	return p.Build(prompt.FormatMarkdown)
}

func formatHistory(msgs []llm.Message) string {
	var b strings.Builder
	for _, m := range msgs {
		if m.Role != llm.RoleUser && m.Role != llm.RoleAssistant && m.Role != llm.RoleTool {
			continue
		}
		if emptyBlock(m) {
			continue
		}
		text := renderContextMessage(m)
		if text == "" {
			continue
		}
		if len(text) > 1000 {
			text = text[:1000] + "…"
		}
		b.WriteString(text)
		b.WriteString("\n")
	}
	return strings.TrimSuffix(b.String(), "\n")
}

func emptyBlock(m llm.Message) bool {
	return m.Role == llm.RoleAssistant &&
		len(m.ToolCalls) == 0 &&
		llm.MessageText(m) == "" &&
		reasoningText(m) == ""
}

func renderContextMessage(m llm.Message) string {
	switch m.Role {
	case llm.RoleUser:
		return "user: " + llm.MessageText(m)
	case llm.RoleAssistant:
		var lines []string
		if len(m.ToolCalls) > 0 {
			var names []string
			for _, tc := range m.ToolCalls {
				names = append(names, tc.Function.Name)
			}
			lines = append(lines, "assistant: tool call: "+strings.Join(names, ", "))
		} else if text := llm.MessageText(m); text != "" {
			lines = append(lines, "assistant: "+text)
		}
		if r := reasoningText(m); r != "" {
			lines = append(lines, "assistant thinking: "+r)
		}
		return strings.Join(lines, "\n")
	case llm.RoleTool:
		text := llm.MessageText(m)
		if !strings.HasPrefix(text, "Error:") {
			return ""
		}
		return "tool error: " + strings.TrimPrefix(text, "Error: ")
	}
	return ""
}

func reasoningText(m llm.Message) string {
	if r, ok := m.Reasoning.(string); ok {
		return r
	}
	return ""
}

func (m *Middleware) askJudge(ctx context.Context, call llm.ToolCall, permErr error) (*decision, error) {
	if prov, model, ok := m.resolveDecisionJudge(); ok {
		verdict, err := m.askDecisionJudge(ctx, prov, model, call, permErr)
		if err == nil {
			return verdict, nil
		}
	}

	judge := m.judge()
	if judge == nil {
		return nil, fmt.Errorf("judge provider unavailable")
	}
	input, err := buildJudgePrompt(call, permErr, agent.HistoryFrom(ctx))
	if err != nil {
		return nil, fmt.Errorf("judge: build prompt: %w", err)
	}
	res, err := judge.Run(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("judge request failed: %w", err)
	}
	verdict, err := parseVerdict(res.Output)
	if err != nil {
		return nil, fmt.Errorf("judge returned an invalid response")
	}
	if verdict.Allow {
		return &decision{Allow: true}, nil
	}
	reason := strings.TrimSpace(verdict.Reason)
	if reason == "" {
		reason = "judge denied the tool call"
	}
	return &decision{Allow: false, Reason: reason}, nil
}

func (m *Middleware) askDecisionJudge(ctx context.Context, prov decisionapi.Provider, model string, call llm.ToolCall, permErr error) (*decision, error) {
	state, err := buildJudgePrompt(call, permErr, agent.HistoryFrom(ctx))
	if err != nil {
		return nil, fmt.Errorf("judge: build prompt: %w", err)
	}

	res, err := prov.Decide(ctx, &decisionapi.Request{
		Model: model,
		State: state,
		Questions: map[string]decisionapi.Question{
			"safe_to_run": decisionapi.BoolQuestion(
				"Is this action safe to run without a human approving it first?",
				map[string]string{
					"true":  "Reversible or low-impact, and clearly within the stated task.",
					"false": "Destructive, irreversible, or broader than the task requires.",
				},
			),
		},
	})
	if err != nil {
		return nil, fmt.Errorf("judge decision request failed: %w", err)
	}

	answer, ok := res.Answers["safe_to_run"]
	if !ok {
		return nil, fmt.Errorf("judge decision returned no answer for %q", "safe_to_run")
	}
	p, ok := answer.NoulValue()
	if !ok {
		return nil, fmt.Errorf("judge decision returned an invalid response")
	}

	threshold := config.DefaultJudgeThreshold
	if m.cfg != nil {
		threshold = m.cfg.JudgeThresholdValue()
	}
	if p >= threshold {
		return &decision{Allow: true}, nil
	}
	reason := fmt.Sprintf("judge denied the tool call (p=%.2f)", p)
	return &decision{Allow: false, Reason: reason}, nil
}

func parseVerdict(output string) (judgeVerdict, error) {
	var verdict judgeVerdict
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &verdict); err == nil {
		return verdict, nil
	}
	start := strings.Index(output, "{")
	end := strings.LastIndex(output, "}")
	if start < 0 || end <= start {
		return verdict, fmt.Errorf("no JSON object found")
	}
	if err := json.Unmarshal([]byte(output[start:end+1]), &verdict); err != nil {
		return verdict, err
	}
	return verdict, nil
}
