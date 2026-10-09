package connect

import (
	"context"
	"testing"

	"github.com/peggco/pegg/internal/core/config"
)

func TestIdentitySignVerify(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	id, err := LoadOrCreateIdentity("test-device")
	if err != nil {
		t.Fatalf("create identity: %v", err)
	}
	if id.DeviceID == "" || id.PublicKey == "" {
		t.Fatal("identity missing fields")
	}

	nonce := "abc123"
	ts := int64(1700000000)
	sig := id.Sign(HelloMessage(id.DeviceID, nonce, ts))

	if err := VerifyHello(id.PublicKey, id.DeviceID, nonce, ts, sig); err != nil {
		t.Fatalf("verify valid signature: %v", err)
	}
	if err := VerifyHello(id.PublicKey, id.DeviceID, nonce, ts+1, sig); err == nil {
		t.Fatal("expected verification failure for tampered message")
	}
	if err := VerifyHello(id.PublicKey, "other", nonce, ts, sig); err == nil {
		t.Fatal("expected verification failure for wrong device id")
	}
}

func TestIdentityReloadIsStable(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	first, err := LoadOrCreateIdentity("dev")
	if err != nil {
		t.Fatal(err)
	}
	second, err := LoadOrCreateIdentity("dev")
	if err != nil {
		t.Fatal(err)
	}
	if first.DeviceID != second.DeviceID || first.PublicKey != second.PublicKey {
		t.Fatal("identity changed across loads")
	}
}

func TestResetIdentity(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if _, err := LoadOrCreateIdentity("dev"); err != nil {
		t.Fatal(err)
	}
	if err := ResetIdentity(); err != nil {
		t.Fatal(err)
	}
	fresh, err := LoadOrCreateIdentity("dev")
	if err != nil {
		t.Fatal(err)
	}
	if fresh.DeviceID == "" {
		t.Fatal("expected a new identity after reset")
	}
}

func TestHasScope(t *testing.T) {
	if !hasScope([]string{ScopeAdmin}, ScopeSettingsWrite) {
		t.Fatal("admin should imply all scopes")
	}
	if !hasScope([]string{ScopeChat}, ScopeChat) {
		t.Fatal("exact scope should match")
	}
	if hasScope([]string{ScopeRead}, ScopeSettingsWrite) {
		t.Fatal("missing scope should not match")
	}
	if !hasScope([]string{ScopeRead}, "") {
		t.Fatal("empty required scope always allowed")
	}
}

func TestMethodAllowed(t *testing.T) {
	c := &Connector{cfg: &config.ConnectConfig{AllowedMethods: []string{"status.get", "settings.*"}}}
	if !c.methodAllowed("status.get") {
		t.Fatal("exact allow should pass")
	}
	if !c.methodAllowed("settings.memory.update") {
		t.Fatal("prefix wildcard should pass")
	}
	if c.methodAllowed("chat.send") {
		t.Fatal("non-allowlisted method must be rejected")
	}

	all := &Connector{cfg: &config.ConnectConfig{}}
	if !all.methodAllowed("anything") {
		t.Fatal("empty allowlist permits everything")
	}
}

func TestMaxConcurrency(t *testing.T) {
	if got := maxConcurrency(nil); got != config.DefaultConnectConcurrency {
		t.Fatalf("nil cfg = %d, want default", got)
	}
	if got := maxConcurrency(&config.ConnectConfig{MaxConcurrency: 7}); got != 7 {
		t.Fatalf("got %d, want 7", got)
	}
}

func TestRunManagerAcquireRelease(t *testing.T) {
	c := &Connector{cfg: &config.ConnectConfig{MaxConcurrency: 1}, runCtx: context.Background(), stopped: make(chan struct{})}
	m := newRunManager(c)
	if !m.tryAcquire() {
		t.Fatal("first acquire should succeed")
	}
	if m.tryAcquire() {
		t.Fatal("second acquire should fail at concurrency limit")
	}
	m.release(&run{id: "x", agentID: "a"})
	if !m.tryAcquire() {
		t.Fatal("acquire should succeed after release")
	}
}
