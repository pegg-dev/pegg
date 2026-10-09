package cline

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/peggco/pegg/internal/core/config"
	"github.com/peggco/pegg/internal/llm"
	"github.com/peggco/pegg/internal/llm/subscription"
)

func TestClineAuthHeader(t *testing.T) {
	if got := clineAuthHeader("workos:abc"); got != "Bearer workos:abc" {
		t.Fatalf("got %q", got)
	}
	if got := clineAuthHeader("abc"); got != "Bearer workos:abc" {
		t.Fatalf("refreshed token should get the workos prefix, got %q", got)
	}
}

func TestRegistered(t *testing.T) {
	for _, name := range []string{ProviderName, ProviderPassName} {
		if !llm.HasProvider(name) {
			t.Fatalf("%s provider not registered", name)
		}
		if _, ok := subscription.Get(name); !ok {
			t.Fatalf("%s not registered as a subscription provider", name)
		}
	}
}

func TestNewFromConfigNotLoggedIn(t *testing.T) {
	t.Setenv("CLINE_DATA_DIR", t.TempDir())
	if _, err := NewFromConfig(config.LLMConfig{}); err == nil {
		t.Fatal("expected an error when not signed in")
	}
}

func TestNewFromConfigLoggedIn(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CLINE_DATA_DIR", dir)
	exp := time.Now().Add(time.Hour).UnixMilli()
	providers := `{"version":1,"providers":{"cline":{"settings":{"provider":"cline","auth":{"accessToken":"workos:tok","refreshToken":"rt","expiresAt":` +
		strconv.FormatInt(exp, 10) + `}}}}}`
	settingsDir := filepath.Join(dir, "settings")
	if err := os.MkdirAll(settingsDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(settingsDir, "providers.json"), []byte(providers), 0o600); err != nil {
		t.Fatal(err)
	}

	p, err := NewFromConfig(config.LLMConfig{})
	if err != nil {
		t.Fatal(err)
	}
	if p.Name() != ProviderName {
		t.Fatalf("provider name = %q", p.Name())
	}
	_ = context.Background()
}

func TestClinePassStaticModels(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CLINE_DATA_DIR", dir)
	exp := time.Now().Add(time.Hour).UnixMilli()
	providers := `{"version":1,"providers":{"cline-pass":{"settings":{"provider":"cline-pass","auth":{"accessToken":"workos:tok","refreshToken":"rt","expiresAt":` +
		strconv.FormatInt(exp, 10) + `}}}}}`
	settingsDir := filepath.Join(dir, "settings")
	if err := os.MkdirAll(settingsDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(settingsDir, "providers.json"), []byte(providers), 0o600); err != nil {
		t.Fatal(err)
	}

	p, err := NewPassFromConfig(config.LLMConfig{})
	if err != nil {
		t.Fatal(err)
	}
	models, err := p.ListModels(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != len(clinePassModels) {
		t.Fatalf("models = %d, want %d", len(models), len(clinePassModels))
	}
	for _, m := range models {
		if !strings.HasPrefix(m.ID, "cline-pass/") {
			t.Fatalf("model %q is not a cline-pass model", m.ID)
		}
	}
}
