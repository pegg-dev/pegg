package server

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/peggco/pegg/internal/core/config"
	"github.com/peggco/pegg/internal/core/event"
	"github.com/peggco/pegg/internal/core/logger"
	"github.com/peggco/pegg/internal/llm"
	"github.com/peggco/pegg/internal/lsp"
	"github.com/peggco/pegg/internal/mcp"
	"github.com/peggco/pegg/internal/session"
	"github.com/peggco/pegg/internal/vfs"
)

type Server struct {
	cfg      *config.Config
	bus      event.Bus
	log      *logger.Logger
	sessions *session.Manager
	llmMgr   *llm.Manager
	mcpMgr   *mcp.Manager
	lspMgr   *lsp.Manager
	fs       *vfs.VFS
	httpSrv  *http.Server
}

func New(
	cfg *config.Config,
	bus event.Bus,
	log *logger.Logger,
	vfs *vfs.VFS,
	sessions *session.Manager,
	llmMgr *llm.Manager,
	mcpMgr *mcp.Manager,
	lspMgr *lsp.Manager,
) *Server {
	s := &Server{
		cfg:      cfg,
		bus:      bus,
		log:      log,
		sessions: sessions,
		llmMgr:   llmMgr,
		mcpMgr:   mcpMgr,
		lspMgr:   lspMgr,
		fs:       vfs,
	}

	mux := http.NewServeMux()
	s.registerRoutes(mux)

	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	s.httpSrv = &http.Server{
		Addr:         addr,
		Handler:      s.authMiddleware(s.corsMiddleware(s.loggingMiddleware(mux))),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 0,
		IdleTimeout:  120 * time.Second,
	}

	return s
}

func (s *Server) ListenAndServe() error {
	s.log.Finfo("http server listening on %s", s.httpSrv.Addr)
	err := s.httpSrv.ListenAndServe()
	if err == http.ErrServerClosed {
		return nil
	}
	return fmt.Errorf("http server: %w", err)
}

func (s *Server) Shutdown(ctx context.Context) error {
	s.log.Info("http server shutting down")
	return s.httpSrv.Shutdown(ctx)
}

func (s *Server) registerRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/health", s.handleHealth)
	mux.HandleFunc("GET /api/models", s.handleListModels)
	mux.HandleFunc("POST /api/run", s.handleRun)
	mux.HandleFunc("GET /api/sessions", s.handleListSessions)
	mux.HandleFunc("GET /api/sessions/{id}", s.handleGetSession)
	mux.HandleFunc("GET /api/sessions/{id}/messages", s.handleGetSessionMessages)
	mux.HandleFunc("DELETE /api/sessions/{id}", s.handleDeleteSession)
}
