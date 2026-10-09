package lsp

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/peggco/pegg/internal/core/config"
)

type Server struct {
	name string
	cfg  config.LanguageServerConfig

	mu       sync.Mutex
	client   *Client
	started  bool
	failed   bool
	starting bool
	opened   map[string]bool
	lastUsed time.Time
}

func NewServer(name string, cfg config.LanguageServerConfig) *Server {
	return &Server{
		name:     name,
		cfg:      cfg,
		opened:   make(map[string]bool),
		lastUsed: time.Now(),
	}
}

func (s *Server) Touch() {
	s.mu.Lock()
	s.lastUsed = time.Now()
	s.mu.Unlock()
}

func (s *Server) IdleFor() time.Duration {
	s.mu.Lock()
	defer s.mu.Unlock()
	return time.Since(s.lastUsed)
}

func (s *Server) Matches(path string) bool {
	ext := extensionOf(path)
	for _, ft := range s.cfg.FileTypes {
		if ft == ext {
			return true
		}
	}
	return false
}

func (s *Server) Client() *Client {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.client
}

func (s *Server) IsRunning() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.client != nil && s.started
}

func (s *Server) IsFailed() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.failed
}

func (s *Server) stop() {
	s.mu.Lock()
	c := s.client
	s.mu.Unlock()
	if c == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = c.Shutdown(ctx)
	_ = c.Close()

	s.mu.Lock()
	s.client = nil
	s.started = false
	s.opened = make(map[string]bool)
	s.mu.Unlock()
}

func extensionOf(path string) string {
	i := strings.LastIndexByte(path, '.')
	if i < 0 || i == len(path)-1 {
		return ""
	}
	return path[i+1:]
}
