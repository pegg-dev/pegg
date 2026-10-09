package connect

import (
	"context"

	"github.com/peggco/pegg/internal/agent/middlewares"
	"github.com/peggco/pegg/internal/builtin/middlewares/permission"
	"github.com/peggco/pegg/internal/core/config"
)

var permissionModes = []string{"allow", "semi-ask", "ask", "semi-judge", "judge"}

var permissionPresets = map[string]map[string]string{
	"auto": {
		"read": "allow", "write": "allow", "edit": "allow", "delete": "allow",
		"list": "allow", "glob": "allow", "grep": "allow", "bash": "allow",
		"todo": "allow", "todoread": "allow", "todowrite": "allow",
		"webfetch": "allow", "websearch": "allow", "task": "allow", "taskstatus": "allow",
		"askuserquestion": "allow", "enterplanmode": "allow", "exitplanmode": "allow",
	},
	"manual": {
		"read": "semi-ask", "write": "semi-ask", "edit": "semi-ask", "delete": "semi-ask",
		"list": "semi-ask", "glob": "semi-ask", "grep": "semi-ask", "bash": "semi-ask",
		"todo": "semi-ask", "todoread": "semi-ask", "todowrite": "semi-ask",
		"webfetch": "semi-ask", "websearch": "semi-ask", "task": "semi-ask", "taskstatus": "semi-ask",
		"askuserquestion": "semi-ask", "enterplanmode": "semi-ask", "exitplanmode": "semi-ask",
	},
	"judge": {
		"read": "semi-judge", "write": "semi-judge", "edit": "semi-judge", "delete": "semi-judge",
		"list": "semi-judge", "glob": "semi-judge", "grep": "semi-judge", "bash": "semi-judge",
		"todo": "semi-judge", "todoread": "semi-judge", "todowrite": "semi-judge",
		"webfetch": "semi-judge", "websearch": "semi-judge", "task": "semi-judge", "taskstatus": "semi-judge",
		"askuserquestion": "semi-judge", "enterplanmode": "semi-judge", "exitplanmode": "semi-judge",
	},
}

func (c *Connector) registerPermissionSettings() {
	c.dispatch.register("settings.permissions.get", ScopeSettingsRead, c.handlePermissionsGet)
	c.dispatch.register("settings.permissions.update", ScopeSettingsWrite, c.handlePermissionsUpdate)
	c.dispatch.register("settings.permissions.presets.list", ScopeSettingsRead, c.handlePermissionPresets)
	c.dispatch.register("settings.permissions.remembered.list", ScopeSettingsRead, c.handleRememberedList)
	c.dispatch.register("settings.permissions.remembered.revoke", ScopeSettingsWrite, c.handleRememberedRevoke)
}

func (c *Connector) handlePermissionsGet(_ context.Context, _ *Request) (any, error) {
	perm := (*config.PermissionConfig)(nil)
	if c.deps.Config != nil {
		perm = c.deps.Config.Permission
	}
	def := "semi-ask"
	rules := map[string]string{}
	judgeProvider, judgeModel := "", ""
	threshold := config.DefaultJudgeThreshold
	if perm != nil {
		if perm.Default != "" {
			def = perm.Default
		}
		if perm.Rules != nil {
			rules = perm.Rules
		}
		judgeProvider = perm.JudgeProvider
		judgeModel = perm.JudgeModel
		threshold = perm.JudgeThresholdValue()
	}
	return map[string]any{
		"default":         def,
		"rules":           rules,
		"judge_provider":  judgeProvider,
		"judge_model":     judgeModel,
		"judge_threshold": threshold,
		"modes":           permissionModes,
		"presets":         permissionPresets,
	}, nil
}

type permissionsUpdateParams struct {
	Default        string            `json:"default"`
	Rules          map[string]string `json:"rules"`
	JudgeProvider  string            `json:"judge_provider"`
	JudgeModel     string            `json:"judge_model"`
	JudgeThreshold *float64          `json:"judge_threshold"`
}

func (c *Connector) handlePermissionsUpdate(_ context.Context, req *Request) (any, error) {
	var p permissionsUpdateParams
	if err := req.Decode(&p); err != nil {
		return nil, &FrameError{Code: ErrCodeBadRequest, Message: "invalid params: " + err.Error()}
	}
	if err := c.requireConfirm("change permission policy", "permissions"); err != nil {
		return nil, err
	}
	perm := &config.PermissionConfig{
		Default:        p.Default,
		Rules:          p.Rules,
		JudgeProvider:  p.JudgeProvider,
		JudgeModel:     p.JudgeModel,
		JudgeThreshold: p.JudgeThreshold,
	}
	if perm.Default == "" {
		perm.Default = "semi-ask"
	}
	if err := config.UpsertPermission(perm); err != nil {
		return nil, &FrameError{Code: ErrCodeInternal, Message: "save permissions: " + err.Error()}
	}
	applyPermissionConfig(perm)
	if err := c.reloadConfig(); err != nil {
		return nil, &FrameError{Code: ErrCodeInternal, Message: err.Error()}
	}
	c.emit(eventSettings, "", "", map[string]any{"group": "permissions"})
	return map[string]any{"saved": true}, nil
}

func (c *Connector) handlePermissionPresets(_ context.Context, _ *Request) (any, error) {
	return map[string]any{
		"modes":   permissionModes,
		"presets": permissionPresets,
	}, nil
}

func applyPermissionConfig(perm *config.PermissionConfig) {
	if m, ok := middlewares.Get("permission"); ok {
		if p, ok := m.(*permission.Middleware); ok {
			p.UpdateConfig(perm)
		}
	}
}
