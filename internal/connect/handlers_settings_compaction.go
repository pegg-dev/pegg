package connect

import (
	"context"

	"github.com/peggco/pegg/internal/core/config"
)

func (c *Connector) registerCompactionMemorySettings() {
	c.registerCompactionMethods()
	c.registerMemoryMethods()
}

func (c *Connector) registerCompactionMethods() {
	c.dispatch.register("settings.compaction.get", ScopeSettingsRead, c.handleCompactionGet)
	c.dispatch.register("settings.compaction.update", ScopeSettingsWrite, c.handleCompactionUpdate)
}

func (c *Connector) handleCompactionGet(_ context.Context, _ *Request) (any, error) {
	if c.deps.Config != nil && c.deps.Config.Compaction != nil {
		return c.deps.Config.Compaction, nil
	}
	return &config.CompactionConfig{
		Enabled:            true,
		Strategy:           []string{"tool-clearing", "sliding-window"},
		Threshold:          80,
		MaxMessages:        50,
		MaxToolOutputChars: 4000,
	}, nil
}

type compactionUpdateParams struct {
	Enabled            *bool    `json:"enabled"`
	Strategy           []string `json:"strategy"`
	Threshold          float64  `json:"threshold"`
	MaxMessages        int      `json:"max_messages"`
	MaxToolOutputChars int      `json:"max_tool_output_chars"`
	SummarizerProvider string   `json:"summarizer_provider"`
	SummarizerModel    string   `json:"summarizer_model"`
}

func (c *Connector) handleCompactionUpdate(_ context.Context, req *Request) (any, error) {
	var p compactionUpdateParams
	if err := req.Decode(&p); err != nil {
		return nil, &FrameError{Code: ErrCodeBadRequest, Message: "invalid params: " + err.Error()}
	}
	if p.Threshold < 0 || p.Threshold > 100 {
		return nil, &FrameError{Code: ErrCodeBadRequest, Message: "threshold must be between 0 and 100"}
	}
	if p.MaxMessages < 0 || p.MaxToolOutputChars < 0 {
		return nil, &FrameError{Code: ErrCodeBadRequest, Message: "max_messages and max_tool_output_chars must be >= 0"}
	}
	enabled := true
	if p.Enabled != nil {
		enabled = *p.Enabled
	}
	cfg := &config.CompactionConfig{
		Enabled:            enabled,
		Strategy:           p.Strategy,
		Threshold:          p.Threshold,
		MaxMessages:        p.MaxMessages,
		MaxToolOutputChars: p.MaxToolOutputChars,
		SummarizerProvider: p.SummarizerProvider,
		SummarizerModel:    p.SummarizerModel,
	}
	if cfg.Threshold == 0 {
		cfg.Threshold = 80
	}
	if cfg.MaxMessages == 0 {
		cfg.MaxMessages = 50
	}
	if cfg.MaxToolOutputChars == 0 {
		cfg.MaxToolOutputChars = 4000
	}
	if err := config.UpsertCompaction(cfg); err != nil {
		return nil, &FrameError{Code: ErrCodeInternal, Message: "save compaction: " + err.Error()}
	}
	if err := c.reloadConfig(); err != nil {
		return nil, &FrameError{Code: ErrCodeInternal, Message: err.Error()}
	}
	c.emit(eventSettings, "", "", map[string]any{"group": "compaction"})
	return cfg, nil
}
