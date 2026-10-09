package plugin

import (
	"github.com/peggco/pegg/internal/core/cache"
	"github.com/peggco/pegg/internal/core/config"
	"github.com/peggco/pegg/internal/core/event"
	"github.com/peggco/pegg/internal/core/logger"
	"github.com/peggco/pegg/internal/llm"
	"github.com/peggco/pegg/internal/lsp"
	"github.com/peggco/pegg/internal/mcp"
	"github.com/peggco/pegg/internal/session"
	"github.com/peggco/pegg/internal/vfs"
)

func Module(
	cfg *config.Config,
	bus event.Bus,
	log *logger.Logger,
	cacheStore cache.Cache,
	llmMgr *llm.Manager,
	sess *session.Manager,
	fs *vfs.VFS,
	mcpMgr *mcp.Manager,
	lspMgr *lsp.Manager,
) (*Manager, error) {
	pluginCfg := cfg.Plugins
	if !pluginCfg.Enabled {
		log.Info("plugin: system disabled")
		return NewManager(log, cfg, bus, cacheStore, llmMgr, sess, fs, mcpMgr, lspMgr), nil
	}

	mgr := NewManager(log, cfg, bus, cacheStore, llmMgr, sess, fs, mcpMgr, lspMgr)
	mgr.SetExcludedPlugins(pluginCfg.Exclude)

	if err := mgr.LoadPlugins(); err != nil {
		log.Fwarn("plugin: failed to load plugins: %v", err)
	}

	plugins := mgr.ListPlugins()
	if len(plugins) > 0 {
		log.Finfo("plugin: loaded %d plugins", len(plugins))
		for _, p := range plugins {
			log.Finfo("plugin:   - %s v%s", p.Name, p.Version)
		}
	}

	return mgr, nil
}
