package claude

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/peggco/pegg/internal/core/config"
	"github.com/peggco/pegg/internal/llm"
	"github.com/peggco/pegg/internal/llm/subscription"
)

func TestRegistered(t *testing.T) {
	if !llm.HasProvider(ProviderName) {
		t.Fatal("claude provider not registered")
	}
	if _, ok := subscription.Get(ProviderName); !ok {
		t.Fatal("claude not registered as a subscription provider")
	}
}

func TestNewFromConfigNotLoggedIn(t *testing.T) {
	t.Setenv("CLAUDE_CONFIG_DIR", t.TempDir())
	if _, err := NewFromConfig(config.LLMConfig{}); err == nil {
		t.Fatal("expected an error when not signed in")
	}
}

func TestNewFromConfigLoggedIn(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", dir)
	exp := time.Now().Add(time.Hour).UnixMilli()
	creds := `{"claudeAiOauth":{"accessToken":"tok","refreshToken":"rt","expiresAt":` +
		strconv.FormatInt(exp, 10) + `,"subscriptionType":"max"}}`
	if err := os.WriteFile(filepath.Join(dir, ".credentials.json"), []byte(creds), 0o600); err != nil {
		t.Fatal(err)
	}

	p, err := NewFromConfig(config.LLMConfig{})
	if err != nil {
		t.Fatal(err)
	}
	models, err := p.ListModels(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(models) == 0 {
		t.Fatal("expected a static model list")
	}
}
