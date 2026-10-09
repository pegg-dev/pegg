package memory

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/peggco/pegg/internal/core/config"
)

type Store struct {
	mu         sync.Mutex
	projectDir string
	globalDir  string
}

func NewStore(projectDir string) (*Store, error) {
	s := &Store{projectDir: projectDir}
	gd, err := config.GetConfigDir()
	if err == nil {
		s.globalDir = gd
	}
	if err := s.EnsureDirs(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) root() string {
	return filepath.Join(s.projectDir, config.ProjectConfigDirName, dirName)
}

func (s *Store) sessionsRoot() string {
	return filepath.Join(s.root(), sessionsDir)
}

func (s *Store) globalLessonsPath() string {
	return filepath.Join(s.globalDir, dirName, globalLessons)
}

func (s *Store) EnsureDirs() error {
	for _, d := range []string{s.root(), s.sessionsRoot()} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return fmt.Errorf("memory: create dir %s: %w", d, err)
		}
	}
	if s.globalDir != "" {
		if err := os.MkdirAll(filepath.Dir(s.globalLessonsPath()), 0o755); err != nil {
			return fmt.Errorf("memory: create global dir: %w", err)
		}
	}
	return nil
}

func (s *Store) Clear() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.RemoveAll(s.root()); err != nil {
		return fmt.Errorf("memory: clear: %w", err)
	}
	return nil
}

func (s *Store) path(name string) string {
	return filepath.Join(s.root(), name)
}

func (s *Store) dailySessionFile() string {
	return timeNow().Format("2006-01-02") + ".md"
}

func (s *Store) HasMemory() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	entries, err := s.readIndexLocked()
	if err != nil {
		return false
	}
	if len(entries) > 0 {
		return true
	}
	for _, name := range []string{activeCtxFile, decisionsFile, lessonsFile, notesFile} {
		if _, err := os.Stat(s.path(name)); err == nil {
			return true
		}
	}
	return false
}

func (s *Store) AppendEntries(blocks []string, entries []IndexEntry) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(blocks) == 0 || len(entries) == 0 {
		return 0, nil
	}

	existing, err := s.readIndexLocked()
	if err != nil {
		return 0, err
	}
	known := make(map[string]bool, len(existing))
	for _, e := range existing {
		known[e.ID] = true
	}

	var kept []string
	var newRows []IndexEntry
	seen := make(map[string]bool)
	for i := range entries {
		e := &entries[i]
		if e.ID == "" || seen[e.ID] || known[e.ID] {
			continue
		}
		seen[e.ID] = true
		known[e.ID] = true
		kept = append(kept, blocks[i])
		newRows = append(newRows, *e)
	}
	if len(kept) == 0 {
		return 0, nil
	}

	file := filepath.Join(s.sessionsRoot(), timeNow().Format("2006-01-02")+".md")
	content := strings.Join(kept, "\n\n") + "\n"
	f, err := os.OpenFile(file, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return 0, fmt.Errorf("memory: open session file: %w", err)
	}
	defer f.Close()
	if _, err := f.WriteString(content); err != nil {
		return 0, fmt.Errorf("memory: append session file: %w", err)
	}

	if err := s.appendIndexLocked(existing, newRows); err != nil {
		return 0, err
	}
	return len(newRows), nil
}

func (s *Store) appendIndexLocked(existing []IndexEntry, rows []IndexEntry) error {
	var b strings.Builder
	for _, r := range append(existing, rows...) {
		b.WriteString(r.row())
		b.WriteString("\n")
	}
	if err := os.WriteFile(s.path(indexFile), []byte(b.String()), 0o644); err != nil {
		return fmt.Errorf("memory: write index: %w", err)
	}
	return nil
}

func (s *Store) readIndexLocked() ([]IndexEntry, error) {
	data, err := os.ReadFile(s.path(indexFile))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("memory: read index: %w", err)
	}
	var entries []IndexEntry
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if e, ok := parseIndexRow(line); ok {
			entries = append(entries, e)
		}
	}
	return entries, nil
}

func (s *Store) ReadIndex() ([]IndexEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.readIndexLocked()
}

func (s *Store) HasHash(hash string) bool {
	if hash == "" {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := os.ReadFile(s.path(indexFile))
	if err != nil {
		return false
	}
	return strings.Contains(string(data), hash)
}

func (s *Store) ReadBlock(file, id string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := os.ReadFile(filepath.Join(s.sessionsRoot(), file))
	if err != nil {
		return "", false
	}
	return extractBlock(string(data), "## "+id)
}

func (s *Store) ReadCurated(name string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := os.ReadFile(s.path(name))
	if err != nil {
		return ""
	}
	return string(data)
}

func (s *Store) WriteCurated(name, content string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	content = strings.TrimSpace(content)
	if content == "" {
		return nil
	}
	if err := os.WriteFile(s.path(name), []byte(content+"\n"), 0o644); err != nil {
		return fmt.Errorf("memory: write %s: %w", name, err)
	}
	return nil
}

func (s *Store) AppendGlobalLessons(text string) error {
	if text == "" {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	p := s.globalLessonsPath()
	existing, err := os.ReadFile(p)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if strings.Contains(string(existing), text) {
		return nil
	}
	f, err := os.OpenFile(p, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(text + "\n")
	return err
}

func (s *Store) SearchFiles(query string, limit int) []SearchHit {
	s.mu.Lock()
	defer s.mu.Unlock()

	entries, err := s.readIndexLocked()
	if err != nil {
		return nil
	}
	files := map[string]bool{}
	for _, e := range entries {
		if e.File == "" {
			continue
		}
		files[e.File] = true
	}

	lower := strings.ToLower(query)
	terms := strings.Fields(lower)

	var hits []SearchHit
	for name := range files {
		full := filepath.Join(s.sessionsRoot(), name)
		if info, err := os.Stat(full); err != nil || info.Size() > 1<<20 {
			continue
		}
		data, err := os.ReadFile(full)
		if err != nil {
			continue
		}
		for _, block := range splitBlocks(string(data)) {
			title := firstHeading(block)
			score := scoreBlock(lower, terms, title, block)
			if score <= 0 {
				continue
			}
			hit := SearchHit{
				ID:    extractID(block),
				File:  name,
				Title: title,
				Text:  block,
				Score: score,
			}
			if _, typ, parsedTitle, date := parseBlockHeader(block); parsedTitle != "" {
				hit.Type = typ
				hit.Title = parsedTitle
				hit.Date = date
			}
			hits = append(hits, hit)
		}
	}
	sort.Slice(hits, func(i, j int) bool { return hits[i].Score > hits[j].Score })
	if len(hits) > limit {
		hits = hits[:limit]
	}
	return hits
}

func (s *Store) Stats() (count int, lastDate string, lastFile string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entries, err := s.readIndexLocked()
	if err != nil {
		return 0, "", ""
	}
	count = len(entries)
	if len(entries) > 0 {
		last := entries[len(entries)-1]
		lastDate = last.Date
		lastFile = last.File
	}
	return count, lastDate, lastFile
}

func hashOf(parts ...string) string {
	h := sha256.New()
	for _, p := range parts {
		h.Write([]byte(p))
	}
	return hex.EncodeToString(h.Sum(nil))
}

func timeNow() time.Time { return time.Now().UTC() }
