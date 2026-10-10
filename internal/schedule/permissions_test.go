package schedule

import (
	"testing"

	"github.com/peggco/pegg/internal/core/config"
)

func TestApplyScheduledPermissions(t *testing.T) {
	p := &config.PermissionConfig{
		Default: "semi-ask",
		Rules: map[string]string{
			"read":  "ask",
			"bash":  "semi-judge",
			"write": "semi-ask",
			"todo":  "allow",
		},
	}
	ApplyScheduledPermissions(p)

	if p.Default != "allow" {
		t.Errorf("default = %q, want allow", p.Default)
	}
	if p.Rules["read"] != "allow" {
		t.Errorf("read = %q, want allow", p.Rules["read"])
	}
	if p.Rules["write"] != "allow" {
		t.Errorf("write = %q, want allow", p.Rules["write"])
	}
	if p.Rules["bash"] != "semi-judge" {
		t.Errorf("bash = %q, want semi-judge preserved", p.Rules["bash"])
	}
	if p.Rules["todo"] != "allow" {
		t.Errorf("todo = %q, want allow", p.Rules["todo"])
	}
}

func TestApplyScheduledPermissionsNil(t *testing.T) {
	ApplyScheduledPermissions(nil)
}
