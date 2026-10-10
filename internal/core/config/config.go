package config

import (
	"fmt"
	"os"
	"path/filepath"

	json "github.com/goccy/go-json"
)

type LLMConfig struct {
	Provider   string            `json:"provider"`
	Driver     string            `json:"driver"`
	APIKey     string            `json:"api_key,omitempty"`
	BaseURL    string            `json:"base_url,omitempty"`
	Timeout    int               `json:"timeout,omitempty"`
	MaxRetries int               `json:"max_retries,omitempty"`
	Headers    map[string]string `json:"headers,omitempty"`
}

type LoggerConfig struct {
	Driver      string `json:"driver"`
	MaxLogCount int    `json:"max_log_count"`
}

type CacheConfig struct {
	Driver string `json:"driver"`
}

type NotificationConfig struct {
	Drivers []string `json:"drivers"`
	Enabled bool     `json:"enabled"`
}

type SessionConfig struct {
	Driver string `json:"driver"`
}

type MCPServerConfig struct {
	Command string            `json:"command,omitempty"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
	URL     string            `json:"url,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
}

type LanguageServerConfig struct {
	Command     string            `json:"command,omitempty"`
	Args        []string          `json:"args,omitempty"`
	Env         map[string]string `json:"env,omitempty"`
	FileTypes   []string          `json:"filetypes,omitempty"`
	RootMarkers []string          `json:"rootMarkers,omitempty"`
	Download    string            `json:"download,omitempty"`
	Install     string            `json:"install,omitempty"`
	Version     string            `json:"version,omitempty"`
	Enabled     *bool             `json:"enabled,omitempty"`
}

type ServerConfig struct {
	Host            string            `json:"host,omitempty"`
	Port            int               `json:"port,omitempty"`
	RequiredHeaders map[string]string `json:"required_headers,omitempty"`
	Headers         map[string]string `json:"headers,omitempty"`
}

type PermissionConfig struct {
	Default        string            `json:"default"`
	JudgeProvider  string            `json:"judge_provider,omitempty"`
	JudgeModel     string            `json:"judge_model,omitempty"`
	JudgeThreshold *float64          `json:"judge_threshold,omitempty"`
	Rules          map[string]string `json:"rules,omitempty"`
}

const DefaultJudgeThreshold = 0.6

func (p *PermissionConfig) JudgeThresholdValue() float64 {
	if p == nil || p.JudgeThreshold == nil {
		return DefaultJudgeThreshold
	}
	return *p.JudgeThreshold
}

type PluginConfig struct {
	Enabled bool     `json:"enabled,omitempty"`
	Exclude []string `json:"exclude,omitempty"`
}

type ConnectConfig struct {
	Enabled             bool     `json:"enabled"`
	RelayURL            string   `json:"relay_url,omitempty"`
	Token               string   `json:"token,omitempty"`
	Insecure            bool     `json:"insecure,omitempty"`
	AutoApprove         bool     `json:"auto_approve,omitempty"`
	MaxConcurrency      int      `json:"max_concurrency,omitempty"`
	AllowedMethods      []string `json:"allowed_methods,omitempty"`
	DeviceName          string   `json:"device_name,omitempty"`
	ResponseTimeoutSecs int      `json:"response_timeout_secs,omitempty"`
}

const DefaultConnectRelayURL = "wss://connect.pegg.dev"

const DefaultConnectConcurrency = 4

const DefaultConnectResponseTimeoutSecs = 180

type CompactionConfig struct {
	Enabled            bool     `json:"enabled"`
	Strategy           []string `json:"strategy"`
	Threshold          float64  `json:"threshold"`
	MaxMessages        int      `json:"max_messages"`
	MaxToolOutputChars int      `json:"max_tool_output_chars"`
	SummarizerProvider string   `json:"summarizer_provider,omitempty"`
	SummarizerModel    string   `json:"summarizer_model,omitempty"`
}

type RouterAgentConfig struct {
	Default    []string            `json:"default,omitempty"`
	Difficulty map[string][]string `json:"difficulty,omitempty"`
	Images     []string            `json:"images,omitempty"`
}

type SmartRouterConfig struct {
	Enabled  bool                         `json:"enabled"`
	Provider string                       `json:"provider,omitempty"`
	Model    string                       `json:"model,omitempty"`
	Agents   map[string]RouterAgentConfig `json:"agents,omitempty"`
}

type MemoryConfig struct {
	Enabled       bool     `json:"enabled"`
	Provider      string   `json:"provider,omitempty"`
	Model         string   `json:"model,omitempty"`
	Gate          string   `json:"gate,omitempty"`
	Threshold     *float64 `json:"threshold,omitempty"`
	ContextBudget int      `json:"context_budget,omitempty"`
	MaxResults    int      `json:"max_results,omitempty"`
	Consolidate   *bool    `json:"consolidate,omitempty"`
	SkipTools     []string `json:"skip_tools,omitempty"`
}

const (
	DefaultMemoryThreshold  = 0.6
	DefaultMemoryBudget     = 8000
	DefaultMemoryMaxResults = 5

	GateAuto     = "auto"
	GateDecision = "decision"
	GateLLM      = "llm"
	GateScore    = "score"
	GateOff      = "off"
)

var gateModes = []string{GateAuto, GateDecision, GateLLM, GateScore, GateOff}

func (m *MemoryConfig) ThresholdValue() float64 {
	if m == nil || m.Threshold == nil {
		return DefaultMemoryThreshold
	}
	return *m.Threshold
}

func (m *MemoryConfig) BudgetValue() int {
	if m == nil || m.ContextBudget <= 0 {
		return DefaultMemoryBudget
	}
	return m.ContextBudget
}

func (m *MemoryConfig) MaxResultsValue() int {
	if m == nil || m.MaxResults <= 0 {
		return DefaultMemoryMaxResults
	}
	return m.MaxResults
}

func (m *MemoryConfig) ConsolidateValue() bool {
	if m == nil || m.Consolidate == nil {
		return true
	}
	return *m.Consolidate
}

func (m *MemoryConfig) GateValue() string {
	if m == nil || m.Gate == "" {
		return GateAuto
	}
	for _, mode := range gateModes {
		if m.Gate == mode {
			return mode
		}
	}
	return GateAuto
}

func GateModes() []string { return gateModes }

type Config struct {
	Providers       []LLMConfig                     `json:"providers"`
	Logger          LoggerConfig                    `json:"logger"`
	Cache           CacheConfig                     `json:"cache"`
	Session         SessionConfig                   `json:"session"`
	Notification    NotificationConfig              `json:"notification"`
	Server          ServerConfig                    `json:"server,omitempty"`
	Theme           string                          `json:"theme,omitempty"`
	Permission      *PermissionConfig               `json:"permission,omitempty"`
	Compaction      *CompactionConfig               `json:"compaction,omitempty"`
	SmartRouter     *SmartRouterConfig              `json:"smart_router,omitempty"`
	Memory          *MemoryConfig                   `json:"memory,omitempty"`
	Plugins         PluginConfig                    `json:"plugins,omitempty"`
	Connect         *ConnectConfig                  `json:"connect,omitempty"`
	MCPServers      map[string]MCPServerConfig      `json:"mcp_servers,omitempty"`
	LanguageServers map[string]LanguageServerConfig `json:"language_servers,omitempty"`
}

func DefaultConfig() *Config {
	return &Config{
		Providers: []LLMConfig{
			{
				Provider: "opencode-zen",
			},
		},
		Logger: LoggerConfig{
			Driver:      "sqlite",
			MaxLogCount: 1000,
		},
		Cache: CacheConfig{
			Driver: "sqlite",
		},
		Session: SessionConfig{
			Driver: "sqlite",
		},
		Notification: NotificationConfig{
			Drivers: []string{"os"},
			Enabled: true,
		},
		Server: ServerConfig{
			Host: "127.0.0.1",
			Port: 8080,
		},
		Theme: "dark",
		Permission: &PermissionConfig{
			Default: "semi-ask",
			Rules: map[string]string{
				"read":            "semi-ask",
				"write":           "semi-ask",
				"edit":            "semi-ask",
				"delete":          "semi-ask",
				"list":            "semi-ask",
				"glob":            "semi-ask",
				"grep":            "semi-ask",
				"bash":            "semi-judge",
				"todo":            "allow",
				"todoread":        "allow",
				"todowrite":       "allow",
				"webfetch":        "allow",
				"websearch":       "allow",
				"task":            "allow",
				"taskstatus":      "allow",
				"askuserquestion": "allow",
				"enterplanmode":   "ask",
				"exitplanmode":    "ask",
			},
		},
		Plugins: PluginConfig{
			Enabled: true,
		},
		Connect: &ConnectConfig{
			Enabled:        false,
			MaxConcurrency: DefaultConnectConcurrency,
		},
		Compaction: &CompactionConfig{
			Enabled:            true,
			Strategy:           []string{"tool-clearing", "sliding-window"},
			Threshold:          80,
			MaxMessages:        50,
			MaxToolOutputChars: 4000,
		},
		SmartRouter: &SmartRouterConfig{
			Enabled: false,
			Agents:  make(map[string]RouterAgentConfig),
		},
		Memory: &MemoryConfig{
			Enabled:       true,
			ContextBudget: DefaultMemoryBudget,
			MaxResults:    DefaultMemoryMaxResults,
		},
		MCPServers:      make(map[string]MCPServerConfig),
		LanguageServers: make(map[string]LanguageServerConfig),
	}
}

func GetConfigDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}
	return filepath.Join(home, GlobalConfigDirName), nil
}

func GetConfigPath(paths ...string) (string, error) {
	configDir, err := GetConfigDir()
	if err != nil {
		return "", err
	}
	allPaths := append([]string{configDir}, paths...)
	return filepath.Join(allPaths...), nil
}

func Load() (*Config, error) {
	config := DefaultConfig()

	configPath, err := GetConfigPath(GlobalConfigFileName)
	if err != nil {
		return config, nil
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return config, nil
		}
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	if err := json.Unmarshal(data, config); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	return config, nil
}

func Save(config *Config) error {
	configPath, err := GetConfigPath(GlobalConfigFileName)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(configPath), 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(configPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

func SaveIfNotExists(config *Config) error {
	configPath, err := GetConfigPath(GlobalConfigFileName)
	if err != nil {
		return err
	}

	if _, err := os.Stat(configPath); err == nil {
		return nil
	}

	return Save(config)
}

func UpsertProvider(provider LLMConfig) error {
	cfg, err := Load()
	if err != nil {
		return err
	}

	for i := range cfg.Providers {
		if cfg.Providers[i].Provider == provider.Provider {
			cfg.Providers[i] = provider
			return Save(cfg)
		}
	}

	cfg.Providers = append(cfg.Providers, provider)
	return Save(cfg)
}

func RemoveProvider(name string) error {
	cfg, err := Load()
	if err != nil {
		return err
	}

	found := false
	providers := cfg.Providers[:0]
	for _, p := range cfg.Providers {
		if p.Provider == name {
			found = true
			continue
		}
		providers = append(providers, p)
	}
	if !found {
		return fmt.Errorf("provider %q not found", name)
	}

	cfg.Providers = providers
	return Save(cfg)
}

func UpsertMCPServer(name string, server MCPServerConfig) error {
	cfg, err := Load()
	if err != nil {
		return err
	}

	if cfg.MCPServers == nil {
		cfg.MCPServers = make(map[string]MCPServerConfig)
	}
	cfg.MCPServers[name] = server
	return Save(cfg)
}

func RemoveMCPServer(name string) error {
	cfg, err := Load()
	if err != nil {
		return err
	}

	if _, ok := cfg.MCPServers[name]; !ok {
		return fmt.Errorf("mcp server %q not found", name)
	}

	delete(cfg.MCPServers, name)
	return Save(cfg)
}

func UpsertLanguageServer(name string, server LanguageServerConfig) error {
	cfg, err := Load()
	if err != nil {
		return err
	}

	if cfg.LanguageServers == nil {
		cfg.LanguageServers = make(map[string]LanguageServerConfig)
	}
	cfg.LanguageServers[name] = server
	return Save(cfg)
}

func RemoveLanguageServer(name string) error {
	cfg, err := Load()
	if err != nil {
		return err
	}

	if _, ok := cfg.LanguageServers[name]; !ok {
		return fmt.Errorf("language server %q not found", name)
	}

	delete(cfg.LanguageServers, name)
	return Save(cfg)
}

func SaveTheme(theme string) error {
	cfg, err := Load()
	if err != nil {
		return err
	}
	cfg.Theme = theme
	return Save(cfg)
}

func UpsertPermission(cfg *PermissionConfig) error {
	c, err := Load()
	if err != nil {
		return err
	}
	c.Permission = cfg
	return Save(c)
}

func UpsertCompaction(cfg *CompactionConfig) error {
	c, err := Load()
	if err != nil {
		return err
	}
	c.Compaction = cfg
	return Save(c)
}

func UpsertMemory(cfg *MemoryConfig) error {
	c, err := Load()
	if err != nil {
		return err
	}
	c.Memory = cfg
	return Save(c)
}

func UpsertConnect(cfg *ConnectConfig) error {
	c, err := Load()
	if err != nil {
		return err
	}
	c.Connect = cfg
	return Save(c)
}

func TogglePlugin(name string) (bool, error) {
	cfg, err := Load()
	if err != nil {
		return false, err
	}

	exclude := cfg.Plugins.Exclude
	found := false
	for i, n := range exclude {
		if n == name {
			exclude = append(exclude[:i], exclude[i+1:]...)
			found = true
			break
		}
	}
	if !found {
		exclude = append(exclude, name)
	}
	cfg.Plugins.Exclude = exclude

	if err := Save(cfg); err != nil {
		return false, err
	}

	return !found, nil
}
