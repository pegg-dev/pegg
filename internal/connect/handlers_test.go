package connect

import (
	"context"
	"testing"

	"github.com/peggco/pegg/internal/core/config"
)

func TestSettingsHandlersSmoke(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	c, err := New(Deps{Config: config.DefaultConfig(), Log: testLogger(), Version: "test"}, config.DefaultConfig().Connect)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Stop()

	ctx := context.Background()
	cases := []struct {
		name string
		fn   HandlerFunc
	}{
		{"status.get", c.handleStatusGet},
		{"capabilities.get", c.handleCapabilities},
		{"models.list", c.handleModelsList},
		{"providers.list", c.handleProvidersList},
		{"run.list", c.handleRunList},
		{"session.list", c.handleSessionList},
		{"settings.general.get", c.handleSettingsGeneralGet},
		{"settings.permissions.get", c.handlePermissionsGet},
		{"settings.permissions.presets.list", c.handlePermissionPresets},
		{"settings.permissions.remembered.list", c.handleRememberedList},
		{"settings.compaction.get", c.handleCompactionGet},
		{"settings.memory.get", c.handleMemoryGet},
		{"settings.memory.entries", c.handleMemoryEntries},
		{"settings.mcp.list", c.handleMCPList},
		{"settings.skills.list", c.handleSkillsList},
		{"settings.rules.list", c.handleRulesList},
		{"settings.plugins.list", c.handlePluginsList},
		{"settings.system.get", c.handleSystemGet},
	}
	for _, tc := range cases {
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("%s panicked: %v", tc.name, r)
				}
			}()
			if _, err := tc.fn(ctx, &Request{}); err != nil {
				t.Fatalf("%s returned error: %v", tc.name, err)
			}
		}()
	}
}

func TestValidationRejects(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	c, err := New(Deps{Config: config.DefaultConfig(), Log: testLogger(), Version: "test"}, config.DefaultConfig().Connect)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Stop()

	ctx := context.Background()
	checks := []struct {
		name string
		fn   HandlerFunc
		req  *Request
	}{
		{"chat.send", c.handleChatSend, &Request{Params: []byte(`{}`)}},
		{"session.get", c.handleSessionGet, &Request{Params: []byte(`{}`)}},
		{"run.cancel", c.handleRunCancel, &Request{Params: []byte(`{}`)}},
		{"settings.general.update", c.handleSettingsGeneralUpdate, &Request{Params: []byte(`{"theme":"does-not-exist"}`)}},
		{"settings.mcp.upsert", c.handleMCPUpsert, &Request{Params: []byte(`{}`)}},
		{"settings.rules.get", c.handleRuleGet, &Request{Params: []byte(`{"name":"../evil.md"}`)}},
	}
	for _, tc := range checks {
		_, err := tc.fn(ctx, tc.req)
		if err == nil {
			t.Fatalf("%s: expected error for invalid input", tc.name)
		}
		var fe *FrameError
		if !asFrameErrorIs(err, &fe) {
			t.Fatalf("%s: expected FrameError, got %T", tc.name, err)
		}
	}
}

func asFrameErrorIs(err error, target **FrameError) bool {
	fe, ok := err.(*FrameError)
	if ok {
		*target = fe
	}
	return ok
}
