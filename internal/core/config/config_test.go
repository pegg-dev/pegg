package config

import "testing"

func findProvider(cfg *Config, name string) *LLMConfig {
	for i := range cfg.Providers {
		if cfg.Providers[i].Provider == name {
			return &cfg.Providers[i]
		}
	}
	return nil
}

func TestDefaultConfigHasOpenCodeZen(t *testing.T) {
	p := findProvider(DefaultConfig(), "opencode-zen")
	if p == nil {
		t.Fatal("default config should include opencode-zen")
	}
	if p.APIKey != "" {
		t.Fatalf("opencode-zen API key = %q, want empty", p.APIKey)
	}
}

func TestUpsertProviderAdds(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	if err := UpsertProvider(LLMConfig{Provider: "groq", APIKey: "a"}); err != nil {
		t.Fatal(err)
	}
	if err := UpsertProvider(LLMConfig{Provider: "openai", APIKey: "b"}); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Providers) != 3 {
		t.Fatalf("providers = %+v, want 3 (default + groq + openai)", cfg.Providers)
	}
	if findProvider(cfg, "opencode-zen") == nil {
		t.Fatalf("default opencode-zen provider missing: %+v", cfg.Providers)
	}
}

func TestUpsertProviderUpdates(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	if err := UpsertProvider(LLMConfig{Provider: "groq", APIKey: "a"}); err != nil {
		t.Fatal(err)
	}
	if err := UpsertProvider(LLMConfig{Provider: "groq", APIKey: "b"}); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Providers) != 2 {
		t.Fatalf("providers = %+v, want 2 (default + groq)", cfg.Providers)
	}
	if p := findProvider(cfg, "groq"); p == nil || p.APIKey != "b" {
		t.Fatalf("groq provider = %+v, want API key b", p)
	}
}

func TestRemoveProvider(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	if err := UpsertProvider(LLMConfig{Provider: "groq", APIKey: "a"}); err != nil {
		t.Fatal(err)
	}
	if err := UpsertProvider(LLMConfig{Provider: "openai", APIKey: "b"}); err != nil {
		t.Fatal(err)
	}

	if err := RemoveProvider("groq"); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if findProvider(cfg, "groq") != nil {
		t.Fatalf("groq should be removed: %+v", cfg.Providers)
	}
	if findProvider(cfg, "openai") == nil {
		t.Fatalf("openai should remain: %+v", cfg.Providers)
	}
	if findProvider(cfg, "opencode-zen") == nil {
		t.Fatalf("default provider should remain: %+v", cfg.Providers)
	}

	if err := RemoveProvider("nope"); err == nil {
		t.Fatal("expected error for missing provider")
	}
}

func TestUpsertMCPServerAdds(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	if err := UpsertMCPServer("db", MCPServerConfig{Command: "npx", Args: []string{"-y"}}); err != nil {
		t.Fatal(err)
	}
	if err := UpsertMCPServer("remote", MCPServerConfig{URL: "https://example.com/sse"}); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.MCPServers) != 2 {
		t.Fatalf("mcp servers = %+v, want 2", cfg.MCPServers)
	}
	if cfg.MCPServers["db"].Command != "npx" {
		t.Fatalf("db = %+v", cfg.MCPServers["db"])
	}
	if cfg.MCPServers["remote"].URL != "https://example.com/sse" {
		t.Fatalf("remote = %+v", cfg.MCPServers["remote"])
	}
}

func TestUpsertMCPServerUpdates(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	if err := UpsertMCPServer("db", MCPServerConfig{Command: "npx"}); err != nil {
		t.Fatal(err)
	}
	if err := UpsertMCPServer("db", MCPServerConfig{Command: "go", Args: []string{"run", "server"}}); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.MCPServers) != 1 {
		t.Fatalf("mcp servers = %+v, want 1", cfg.MCPServers)
	}
	if cfg.MCPServers["db"].Command != "go" || len(cfg.MCPServers["db"].Args) != 2 {
		t.Fatalf("db = %+v", cfg.MCPServers["db"])
	}
}

func TestRemoveMCPServer(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	if err := UpsertMCPServer("db", MCPServerConfig{Command: "npx"}); err != nil {
		t.Fatal(err)
	}
	if err := UpsertMCPServer("remote", MCPServerConfig{URL: "https://example.com/sse"}); err != nil {
		t.Fatal(err)
	}

	if err := RemoveMCPServer("db"); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.MCPServers) != 1 || cfg.MCPServers["remote"].URL == "" {
		t.Fatalf("mcp servers = %+v, want only remote", cfg.MCPServers)
	}

	if err := RemoveMCPServer("nope"); err == nil {
		t.Fatal("expected error for missing server")
	}
}
