package codex

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/peggco/pegg/internal/core/config"
	"github.com/peggco/pegg/internal/llm"
	"github.com/peggco/pegg/internal/llm/subscription"
)

func TestRegistered(t *testing.T) {
	if !llm.HasProvider(ProviderName) {
		t.Fatal("codex provider not registered")
	}
	if _, ok := subscription.Get(ProviderName); !ok {
		t.Fatal("codex not registered as a subscription provider")
	}
}

func TestNewFromConfigNotLoggedIn(t *testing.T) {
	t.Setenv("CODEX_HOME", t.TempDir())
	if _, err := NewFromConfig(config.LLMConfig{}); err == nil {
		t.Fatal("expected an error when not signed in")
	}
}

func TestNewFromConfigLoggedIn(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CODEX_HOME", dir)
	claims, _ := json.Marshal(map[string]any{"exp": time.Now().Add(time.Hour).Unix()})
	access := "e30." + base64.RawURLEncoding.EncodeToString(claims) + ".sig"
	auth := `{"auth_mode":"chatgpt","tokens":{"access_token":"` + access + `","refresh_token":"rt","account_id":"acct"}}`
	if err := os.WriteFile(filepath.Join(dir, "auth.json"), []byte(auth), 0o600); err != nil {
		t.Fatal(err)
	}

	p, err := NewFromConfig(config.LLMConfig{})
	if err != nil {
		t.Fatal(err)
	}
	models, err := p.ListModels(context.Background())
	if err != nil || len(models) == 0 {
		t.Fatalf("models=%v err=%v", models, err)
	}
}
