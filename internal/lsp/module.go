package lsp

import (
	"fmt"

	"github.com/peggco/pegg/internal/core/config"
	"github.com/peggco/pegg/internal/core/logger"
	"github.com/peggco/pegg/internal/vfs"
)

func Module(global map[string]config.LanguageServerConfig, fs *vfs.VFS, log *logger.Logger) (*Manager, error) {
	project, err := LoadProjectServers(".")
	if err != nil {
		return nil, fmt.Errorf("lsp: load project servers: %w", err)
	}

	servers := MergeServers(MergeServers(Registered(), global), project)
	mgr := NewManager(fs, log)
	mgr.Start(servers)

	if len(servers) == 0 {
		log.Debug("lsp: no language servers configured")
	} else {
		log.Finfo("lsp: registered %d language server(s)", len(servers))
	}
	return mgr, nil
}
