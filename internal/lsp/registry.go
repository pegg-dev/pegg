package lsp

import (
	"sync"

	"github.com/peggco/pegg/internal/core/config"
)

var (
	serversMu sync.RWMutex
	servers   = make(map[string]config.LanguageServerConfig)
)

func Register(name string, cfg config.LanguageServerConfig) {
	if name == "" {
		return
	}
	serversMu.Lock()
	defer serversMu.Unlock()
	servers[name] = cfg
}

func Registered() map[string]config.LanguageServerConfig {
	serversMu.RLock()
	defer serversMu.RUnlock()
	out := make(map[string]config.LanguageServerConfig, len(servers))
	for k, v := range servers {
		out[k] = v
	}
	return out
}
