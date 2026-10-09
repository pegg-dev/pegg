package connect

import (
	"context"
	"runtime"

	"github.com/peggco/pegg/internal/core/config"
	"github.com/peggco/pegg/internal/core/update"
)

func (c *Connector) registerSystemMethods() {
	c.dispatch.register("settings.system.get", ScopeSettingsRead, c.handleSystemGet)
	c.dispatch.register("settings.system.update.check", ScopeSettingsRead, c.handleUpdateCheck)
	c.dispatch.register("settings.system.update.apply", ScopeAdmin, c.handleUpdateApply)
}

func (c *Connector) handleSystemGet(_ context.Context, _ *Request) (any, error) {
	return map[string]any{
		"app":       config.AppName,
		"version":   c.deps.Version,
		"os":        runtime.GOOS,
		"arch":      runtime.GOARCH,
		"protocol":  ProtocolVersion,
		"workspace": c.workspace(),
		"go":        runtime.Version(),
	}, nil
}

func (c *Connector) handleUpdateCheck(ctx context.Context, _ *Request) (any, error) {
	rel, found, err := update.DetectLatest(ctx)
	if err != nil {
		return nil, &FrameError{Code: ErrCodeUnavailable, Message: "update check: " + err.Error()}
	}
	current := c.deps.Version
	if !found || rel == nil {
		return map[string]any{"found": false, "current": current, "newer": false}, nil
	}
	return map[string]any{
		"found":   true,
		"current": current,
		"latest":  rel.Version,
		"newer":   update.IsNewerThan(current, rel.Version),
	}, nil
}

func (c *Connector) handleUpdateApply(ctx context.Context, _ *Request) (any, error) {
	if err := c.requireConfirm("install a Pegg update", "system.update"); err != nil {
		return nil, err
	}
	if err := update.UpdateToLatest(ctx); err != nil {
		return nil, &FrameError{Code: ErrCodeInternal, Message: "update: " + err.Error()}
	}
	Audit("system.update.apply", nil)
	c.emit(eventSettings, "", "", map[string]any{"group": "system", "updated": true})
	return map[string]any{"updated": true, "note": "restart Pegg to use the new version"}, nil
}
