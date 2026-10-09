package permission

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"sync"

	json "github.com/goccy/go-json"

	"github.com/peggco/pegg/internal/core/config"
)

type Store struct {
	mu   sync.Mutex
	path string
	data *storeData
}

type storeData struct {
	Allowed  map[string]*allowedEntry  `json:"allowed"`
	Rejected map[string]*rejectedEntry `json:"rejected"`
}

type allowedEntry struct {
	ToolName string `json:"tool_name"`
	ArgsHash string `json:"args_hash"`
}

type rejectedEntry struct {
	ToolName string `json:"tool_name"`
	ArgsHash string `json:"args_hash"`
	Reason   string `json:"reason"`
}

func NewStore() *Store {
	s := &Store{data: &storeData{Allowed: map[string]*allowedEntry{}, Rejected: map[string]*rejectedEntry{}}}
	if path, err := config.GetConfigPath("permissions.json"); err == nil {
		s.path = path
		s.load()
	}
	return s
}

func (s *Store) Hash(args string) string {
	sum := sha256.Sum256([]byte(canonicalArgs(args)))
	return hex.EncodeToString(sum[:])
}

func canonicalArgs(args string) string {
	trimmed := strings.TrimSpace(args)
	if trimmed == "" {
		return ""
	}
	var v any
	if err := json.Unmarshal([]byte(trimmed), &v); err == nil {
		if b, err := json.Marshal(v); err == nil {
			return string(b)
		}
	}
	return trimmed
}

func (s *Store) Key(toolName, args string) string {
	return toolName + ":" + s.Hash(args)
}

func (s *Store) IsAllowed(key string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.data.Allowed[key]
	return ok
}

func (s *Store) RejectionReason(key string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.data.Rejected[key]
	if !ok {
		return "", false
	}
	return e.Reason, true
}

func (s *Store) Allow(toolName, args string) {
	key := s.Key(toolName, args)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data.Allowed[key] = &allowedEntry{ToolName: toolName, ArgsHash: s.Hash(args)}
	s.saveLocked()
}

func (s *Store) Reject(toolName, args, reason string) {
	key := s.Key(toolName, args)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data.Rejected[key] = &rejectedEntry{ToolName: toolName, ArgsHash: s.Hash(args), Reason: reason}
	s.saveLocked()
}

func (s *Store) load() {
	if s.path == "" {
		return
	}
	data, err := os.ReadFile(s.path)
	if err != nil {
		return
	}
	var d storeData
	if err := json.Unmarshal(data, &d); err != nil {
		return
	}
	if d.Allowed == nil {
		d.Allowed = map[string]*allowedEntry{}
	}
	if d.Rejected == nil {
		d.Rejected = map[string]*rejectedEntry{}
	}
	s.data = &d
}

func (s *Store) saveLocked() {
	if s.path == "" {
		return
	}
	data, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return
	}
	_ = os.WriteFile(s.path, data, 0o644)
}
