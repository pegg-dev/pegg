package connect

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/peggco/pegg/internal/core/config"
	"github.com/peggco/pegg/internal/skill"
)

func (c *Connector) registerSkillMethods() {
	c.dispatch.register("settings.skills.list", ScopeSettingsRead, c.handleSkillsList)
}

func (c *Connector) handleSkillsList(_ context.Context, _ *Request) (any, error) {
	skills := skill.List()
	out := make([]map[string]any, 0, len(skills))
	for _, s := range skills {
		out = append(out, map[string]any{
			"name":        s.Name,
			"description": s.Description,
			"source":      s.Source,
			"path":        s.Path,
			"when_to_use": s.WhenToUse,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i]["name"].(string) < out[j]["name"].(string) })
	return map[string]any{"skills": out}, nil
}

func (c *Connector) registerRuleMethods() {
	c.dispatch.register("settings.rules.list", ScopeSettingsRead, c.handleRulesList)
	c.dispatch.register("settings.rules.get", ScopeSettingsRead, c.handleRuleGet)
	c.dispatch.register("settings.rules.put", ScopeSettingsWrite, c.handleRulePut)
	c.dispatch.register("settings.rules.delete", ScopeSettingsWrite, c.handleRuleDelete)
}

func (c *Connector) rulesDir(scope string) (string, error) {
	switch scope {
	case "", "global":
		return config.GetConfigPath("rules")
	case "project":
		return config.GetProjectConfigPath("rules")
	default:
		return "", &FrameError{Code: ErrCodeBadRequest, Message: "scope must be global or project"}
	}
}

func validRuleName(name string) bool {
	return name != "" && filepath.Base(name) == name && strings.HasSuffix(name, ".md")
}

func (c *Connector) handleRulesList(_ context.Context, _ *Request) (any, error) {
	type ruleFile struct {
		Scope string `json:"scope"`
		Name  string `json:"name"`
		Path  string `json:"path"`
		Size  int64  `json:"size"`
	}
	var out []ruleFile
	for _, scope := range []string{"global", "project"} {
		dir, err := c.rulesDir(scope)
		if err != nil {
			continue
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
				continue
			}
			info, _ := e.Info()
			var size int64
			if info != nil {
				size = info.Size()
			}
			out = append(out, ruleFile{Scope: scope, Name: e.Name(), Path: filepath.Join(dir, e.Name()), Size: size})
		}
	}
	return map[string]any{"rules": out}, nil
}

type ruleRefParams struct {
	Scope string `json:"scope"`
	Name  string `json:"name"`
}

func (c *Connector) handleRuleGet(_ context.Context, req *Request) (any, error) {
	var p ruleRefParams
	_ = req.Decode(&p)
	if !validRuleName(p.Name) {
		return nil, &FrameError{Code: ErrCodeBadRequest, Message: "invalid rule name"}
	}
	dir, err := c.rulesDir(p.Scope)
	if err != nil {
		return nil, err
	}
	data, rerr := os.ReadFile(filepath.Join(dir, p.Name))
	if rerr != nil {
		return nil, &FrameError{Code: ErrCodeNotFound, Message: "rule not found"}
	}
	return map[string]any{"scope": p.Scope, "name": p.Name, "content": string(data)}, nil
}

type rulePutParams struct {
	Scope   string `json:"scope"`
	Name    string `json:"name"`
	Content string `json:"content"`
}

func (c *Connector) handleRulePut(_ context.Context, req *Request) (any, error) {
	var p rulePutParams
	if err := req.Decode(&p); err != nil {
		return nil, &FrameError{Code: ErrCodeBadRequest, Message: "invalid params: " + err.Error()}
	}
	if !validRuleName(p.Name) {
		return nil, &FrameError{Code: ErrCodeBadRequest, Message: "invalid rule name (must end in .md)"}
	}
	dir, err := c.rulesDir(p.Scope)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, &FrameError{Code: ErrCodeInternal, Message: "create rules dir: " + err.Error()}
	}
	if err := os.WriteFile(filepath.Join(dir, p.Name), []byte(p.Content), 0o644); err != nil {
		return nil, &FrameError{Code: ErrCodeInternal, Message: "write rule: " + err.Error()}
	}
	Audit("rules.put", map[string]any{"scope": p.Scope, "name": p.Name})
	c.emit(eventSettings, "", "", map[string]any{"group": "rules", "scope": p.Scope, "name": p.Name})
	return map[string]any{"scope": p.Scope, "name": p.Name, "saved": true}, nil
}

func (c *Connector) handleRuleDelete(_ context.Context, req *Request) (any, error) {
	var p ruleRefParams
	_ = req.Decode(&p)
	if !validRuleName(p.Name) {
		return nil, &FrameError{Code: ErrCodeBadRequest, Message: "invalid rule name"}
	}
	dir, err := c.rulesDir(p.Scope)
	if err != nil {
		return nil, err
	}
	if err := os.Remove(filepath.Join(dir, p.Name)); err != nil {
		return nil, &FrameError{Code: ErrCodeNotFound, Message: "rule not found"}
	}
	Audit("rules.delete", map[string]any{"scope": p.Scope, "name": p.Name})
	c.emit(eventSettings, "", "", map[string]any{"group": "rules", "scope": p.Scope, "deleted": p.Name})
	return map[string]any{"scope": p.Scope, "name": p.Name, "deleted": true}, nil
}
