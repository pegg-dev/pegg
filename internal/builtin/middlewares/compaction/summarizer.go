package compaction

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/peggco/pegg/internal/agent"
	"github.com/peggco/pegg/internal/agent/prompt"
	"github.com/peggco/pegg/internal/core/config"
	"github.com/peggco/pegg/internal/llm"
)

func generateSummarizerPrompt() (string, error) {
	sys, err := summarizerPromptBuilder().
		Build(prompt.FormatMarkdown)
	if err != nil {
		return "", fmt.Errorf("compaction: summarizer prompt: %w", err)
	}
	return sys, nil
}

type summarizer struct {
	once    sync.Once
	agent   *agent.Agent
	prompt  string
	prov    llm.Provider
	model   llm.Model
	initErr error
}

func newSummarizer() *summarizer {
	return &summarizer{}
}

func (s *summarizer) resolve(cfg *config.CompactionConfig, llmMgr *llm.Manager, parent *agent.Agent) (llm.Provider, llm.Model) {
	if llmMgr == nil {
		return nil, llm.Model{}
	}

	if cfg != nil && cfg.SummarizerProvider != "" && cfg.SummarizerModel != "" {
		prov, err := llmMgr.Provider(cfg.SummarizerProvider)
		if err != nil {
			return nil, llm.Model{}
		}
		res := llmMgr.Select(llm.SelectRequest{
			Mode:     llm.SelectModeExact,
			Provider: cfg.SummarizerProvider,
			Model:    cfg.SummarizerModel,
		})
		if res.Err != nil || res.Model.ID == "" {
			return nil, llm.Model{}
		}
		return prov, res.Model
	}

	if parent != nil && parent.Provider != nil && parent.Model.ID != "" {
		return parent.Provider, parent.Model
	}

	res := llmMgr.Select(llm.SelectRequest{Mode: llm.SelectModePreferred})
	if res.Err != nil || res.Model.ID == "" {
		return nil, llm.Model{}
	}
	prov, err := llmMgr.Provider(res.Provider)
	if err != nil {
		return nil, llm.Model{}
	}
	return prov, res.Model
}

func (s *summarizer) get(cfg *config.CompactionConfig, llmMgr *llm.Manager, parent *agent.Agent) *agent.Agent {
	s.once.Do(func() {
		s.prompt, s.initErr = generateSummarizerPrompt()
		if s.initErr != nil {
			return
		}

		s.prov, s.model = s.resolve(cfg, llmMgr, parent)
		if s.prov == nil {
			s.initErr = fmt.Errorf("compaction: summarizer provider unavailable")
			return
		}

		s.agent = agent.New("compaction-summarizer",
			agent.WithProvider(s.prov),
			agent.WithModel(s.model),
			agent.WithSystemPrompt(s.prompt),
			agent.WithMaxIterations(1),
		)
	})
	if s.initErr != nil {
		return nil
	}
	return s.agent
}

func (s *summarizer) run(history []llm.Message) (string, error) {
	if s.agent == nil {
		return "", fmt.Errorf("compaction: summarizer not initialized")
	}
	input := formatHistoryForSummary(history)
	res, err := s.agent.Run(context.Background(), input)
	if err != nil {
		return "", fmt.Errorf("compaction: summarizer run: %w", err)
	}
	return strings.TrimSpace(res.Output), nil
}

func formatHistoryForSummary(history []llm.Message) string {
	var b strings.Builder
	for _, msg := range history {
		text := llm.MessageText(msg)
		if text == "" && len(msg.ToolCalls) == 0 {
			continue
		}
		switch msg.Role {
		case llm.RoleSystem:
			continue
		case llm.RoleUser:
			b.WriteString("user: ")
			b.WriteString(text)
			b.WriteString("\n")
		case llm.RoleAssistant:
			if len(msg.ToolCalls) > 0 {
				var names []string
				for _, tc := range msg.ToolCalls {
					names = append(names, tc.Function.Name)
				}
				b.WriteString("assistant: tool call: ")
				b.WriteString(strings.Join(names, ", "))
				b.WriteString("\n")
			}
			if text != "" {
				b.WriteString("assistant: ")
				b.WriteString(text)
				b.WriteString("\n")
			}
		case llm.RoleTool:
			if len(text) > 500 {
				text = text[:500] + "..."
			}
			b.WriteString("tool: ")
			b.WriteString(text)
			b.WriteString("\n")
		}
	}
	return b.String()
}
