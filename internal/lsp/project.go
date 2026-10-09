package lsp

import (
	"fmt"
	"os"
	"path/filepath"

	json "github.com/goccy/go-json"

	"github.com/peggco/pegg/internal/core/config"
)

const ProjectConfigFileName = ".lsp.json"

type projectConfig struct {
	LanguageServers map[string]config.LanguageServerConfig `json:"languageServers"`
}

func LoadProjectServers(dir string) (map[string]config.LanguageServerConfig, error) {
	path := filepath.Join(dir, ProjectConfigFileName)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]config.LanguageServerConfig{}, nil
		}
		return nil, fmt.Errorf("lsp: read %s: %w", path, err)
	}

	var cfg projectConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("lsp: parse %s: %w", path, err)
	}
	if cfg.LanguageServers == nil {
		cfg.LanguageServers = map[string]config.LanguageServerConfig{}
	}
	return cfg.LanguageServers, nil
}

func MergeServers(base, over map[string]config.LanguageServerConfig) map[string]config.LanguageServerConfig {
	out := make(map[string]config.LanguageServerConfig, len(base)+len(over))
	for name, cfg := range base {
		out[name] = cfg
	}
	for name, cfg := range over {
		if cfg.Enabled != nil && !*cfg.Enabled {
			delete(out, name)
			continue
		}
		out[name] = cfg
	}
	return out
}

func ProjectConfigPath(dir string) string {
	return filepath.Join(dir, ProjectConfigFileName)
}

func SaveProjectServers(dir string, servers map[string]config.LanguageServerConfig) error {
	path := ProjectConfigPath(dir)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("lsp: create dir %s: %w", dir, err)
	}
	data, err := json.MarshalIndent(projectConfig{LanguageServers: servers}, "", "  ")
	if err != nil {
		return fmt.Errorf("lsp: marshal project servers: %w", err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("lsp: write %s: %w", path, err)
	}
	return nil
}

func UpsertProjectServer(dir, name string, server config.LanguageServerConfig) error {
	servers, err := LoadProjectServers(dir)
	if err != nil {
		return err
	}
	servers[name] = server
	return SaveProjectServers(dir, servers)
}

func RemoveProjectServer(dir, name string) error {
	servers, err := LoadProjectServers(dir)
	if err != nil {
		return err
	}
	if _, ok := servers[name]; !ok {
		return fmt.Errorf("language server %q not found in project", name)
	}
	delete(servers, name)
	return SaveProjectServers(dir, servers)
}
