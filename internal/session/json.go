package session

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	json "github.com/goccy/go-json"

	"github.com/peggco/pegg/internal/core/config"
	"github.com/peggco/pegg/internal/utils/query"
)

const DriverJSON = "json"

func init() {
	RegisterDriver(DriverJSON, func(cfg config.SessionConfig) (Store, error) {
		return NewJSONStore()
	})
}

type jsonFile struct {
	Session   Session    `json:"session"`
	Messages  []Message  `json:"messages"`
	Snapshots []Snapshot `json:"snapshots"`
}

type JSONStore struct {
	mu  sync.RWMutex
	dir string
}

func NewJSONStore() (*JSONStore, error) {
	dir, err := config.GetConfigPath("sessions")
	if err != nil {
		return nil, err
	}
	return newJSONStore(dir)
}

func NewJSONStoreAt(dir string) (*JSONStore, error) {
	return newJSONStore(dir)
}

func newJSONStore(dir string) (*JSONStore, error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("session: create data directory: %w", err)
	}
	return &JSONStore{dir: dir}, nil
}

func (s *JSONStore) filePath(id string) string {
	return filepath.Join(s.dir, id+".json")
}

func (s *JSONStore) load(id string) (*jsonFile, error) {
	data, err := os.ReadFile(s.filePath(id))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	var f jsonFile
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, err
	}
	return &f, nil
}

func (s *JSONStore) persist(f *jsonFile) error {
	data, err := json.Marshal(f)
	if err != nil {
		return err
	}
	return os.WriteFile(s.filePath(f.Session.ID), data, 0644)
}

func (s *JSONStore) Create(sess Session) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := os.Stat(s.filePath(sess.ID)); err == nil {
		return ErrDuplicate
	}
	return s.persist(&jsonFile{Session: sess})
}

func (s *JSONStore) Update(sess Session) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	f, err := s.load(sess.ID)
	if err != nil {
		return err
	}
	f.Session = sess
	return s.persist(f)
}

func (s *JSONStore) Get(id string) (*Session, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	f, err := s.load(id)
	if err != nil {
		return nil, err
	}
	return &f.Session, nil
}

func (s *JSONStore) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.Remove(s.filePath(id)); err != nil {
		if os.IsNotExist(err) {
			return ErrNotFound
		}
		return err
	}
	return nil
}

func (s *JSONStore) List(q query.Query) ([]Session, int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil, 0, err
	}
	var all []Session
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		f, err := s.load(strings.TrimSuffix(e.Name(), ".json"))
		if err != nil {
			if err == ErrNotFound {
				continue
			}
			return nil, 0, err
		}
		all = append(all, f.Session)
	}

	filtered, err := filterSessions(all, q)
	if err != nil {
		return nil, 0, err
	}
	total := len(filtered)

	sortSessions(filtered, q.Sort)
	size, number := normalizePage(q.Page)
	start := (number - 1) * size
	if start > len(filtered) {
		start = len(filtered)
	}
	end := start + size
	if end > len(filtered) {
		end = len(filtered)
	}
	return filtered[start:end], total, nil
}

func (s *JSONStore) InsertMessage(m Message) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	f, err := s.load(m.SessionID)
	if err != nil {
		return err
	}
	f.Messages = append(f.Messages, m)
	return s.persist(f)
}

func (s *JSONStore) Messages(sessionID string) ([]Message, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	f, err := s.load(sessionID)
	if err != nil {
		return nil, err
	}
	out := make([]Message, len(f.Messages))
	copy(out, f.Messages)
	return out, nil
}

func (s *JSONStore) TruncateAfter(sessionID, messageID string) ([]Message, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	f, err := s.load(sessionID)
	if err != nil {
		return nil, err
	}
	idx := -1
	for i, m := range f.Messages {
		if m.ID == messageID {
			idx = i
			break
		}
	}
	if idx == -1 {
		return nil, ErrMessageNotFound
	}
	removed := append([]Message(nil), f.Messages[idx+1:]...)
	f.Messages = f.Messages[:idx+1]
	if err := s.persist(f); err != nil {
		return nil, err
	}
	return removed, nil
}

func (s *JSONStore) TruncateFrom(sessionID, messageID string) ([]Message, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	f, err := s.load(sessionID)
	if err != nil {
		return nil, err
	}
	idx := -1
	for i, m := range f.Messages {
		if m.ID == messageID {
			idx = i
			break
		}
	}
	if idx == -1 {
		return nil, ErrMessageNotFound
	}
	removed := append([]Message(nil), f.Messages[idx:]...)
	f.Messages = f.Messages[:idx]
	if err := s.persist(f); err != nil {
		return nil, err
	}
	return removed, nil
}

func (s *JSONStore) SaveSnapshot(snap Snapshot) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	f, err := s.load(snap.SessionID)
	if err != nil {
		return err
	}
	f.Snapshots = append(f.Snapshots, snap)
	return s.persist(f)
}

func (s *JSONStore) Snapshots(sessionID string) ([]Snapshot, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	f, err := s.load(sessionID)
	if err != nil {
		return nil, err
	}
	out := make([]Snapshot, len(f.Snapshots))
	copy(out, f.Snapshots)
	return out, nil
}

func (s *JSONStore) GetSnapshot(id string) (*Snapshot, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		f, err := s.load(strings.TrimSuffix(e.Name(), ".json"))
		if err != nil {
			continue
		}
		for _, snap := range f.Snapshots {
			if snap.ID == id {
				return &snap, nil
			}
		}
	}
	return nil, ErrSnapshotNotFound
}

func (s *JSONStore) RestoreSnapshot(sessionID string, messages []Message) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	f, err := s.load(sessionID)
	if err != nil {
		return err
	}
	f.Messages = append(f.Messages, messages...)
	sort.Slice(f.Messages, func(i, j int) bool { return f.Messages[i].Seq < f.Messages[j].Seq })
	return s.persist(f)
}

func (s *JSONStore) CompactionChildren(sessionID string) ([]Session, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil, err
	}
	var children []Session
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		f, err := s.load(strings.TrimSuffix(e.Name(), ".json"))
		if err != nil {
			continue
		}
		if f.Session.CompactionParentID == sessionID {
			children = append(children, f.Session)
		}
	}
	sort.Slice(children, func(i, j int) bool {
		return children[i].CreatedAt.Before(children[j].CreatedAt)
	})
	return children, nil
}

func (s *JSONStore) LatestInChain(sessionID string) (*Session, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	f, err := s.load(sessionID)
	if err != nil {
		return nil, err
	}
	current := f.Session

	for {
		entries, err := os.ReadDir(s.dir)
		if err != nil {
			return nil, err
		}
		var latest *Session
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
				continue
			}
			cf, err := s.load(strings.TrimSuffix(e.Name(), ".json"))
			if err != nil {
				continue
			}
			if cf.Session.CompactionParentID == current.ID {
				if latest == nil || cf.Session.CreatedAt.After(latest.CreatedAt) {
					s := cf.Session
					latest = &s
				}
			}
		}
		if latest == nil {
			return &current, nil
		}
		current = *latest
	}
}

func (s *JSONStore) Close() error {
	return nil
}

var allowedColumns = map[string]struct{}{
	"id": {}, "title": {}, "provider": {}, "model": {},
	"project_dir": {}, "parent_id": {}, "compaction_parent_id": {}, "created_at": {}, "updated_at": {},
}

func filterSessions(all []Session, q query.Query) ([]Session, error) {
	var out []Session
	for _, s := range all {
		ok := true
		for _, f := range q.Filters {
			match, err := matchFilter(s, f)
			if err != nil {
				return nil, err
			}
			if !match {
				ok = false
				break
			}
		}
		if q.Search != "" && ok {
			ok = searchMatch(s, q.Search, q.SearchColumns)
		}
		if ok {
			out = append(out, s)
		}
	}
	return out, nil
}

func matchFilter(s Session, f query.Filter) (bool, error) {
	v, ok := columnValue(s, f.Column)
	if !ok {
		return false, fmt.Errorf("session: column %q not allowed", f.Column)
	}
	sv := fmt.Sprint(v)
	fv := fmt.Sprint(f.Value)
	switch f.Operator {
	case query.OpEqual:
		return sv == fv, nil
	case query.OpNotEqual:
		return sv != fv, nil
	case query.OpGreater:
		return compareValues(v, f.Value) > 0, nil
	case query.OpGreaterEqual:
		return compareValues(v, f.Value) >= 0, nil
	case query.OpLess:
		return compareValues(v, f.Value) < 0, nil
	case query.OpLessEqual:
		return compareValues(v, f.Value) <= 0, nil
	case query.OpLike:
		return likeMatch(sv, fv), nil
	}
	return false, fmt.Errorf("session: operator %q not allowed", f.Operator)
}

func compareValues(a, b any) int {
	if at, ok := a.(interface{ Compare(time.Time) int }); ok {
		if bt, ok := b.(time.Time); ok {
			return at.Compare(bt)
		}
	}
	return strings.Compare(fmt.Sprint(a), fmt.Sprint(b))
}

func likeMatch(value, pattern string) bool {
	term := strings.Trim(pattern, "%_")
	return strings.Contains(value, term)
}

func searchMatch(s Session, search string, columns []string) bool {
	for _, col := range columns {
		v, ok := columnValue(s, col)
		if !ok {
			continue
		}
		if likeMatch(fmt.Sprint(v), "%"+search+"%") {
			return true
		}
	}
	return false
}

func columnValue(s Session, col string) (any, bool) {
	switch col {
	case "id":
		return s.ID, true
	case "title":
		return s.Title, true
	case "provider":
		return s.Provider, true
	case "model":
		return s.Model, true
	case "project_dir":
		return s.ProjectDir, true
	case "parent_id":
		return s.ParentID, true
	case "compaction_parent_id":
		return s.CompactionParentID, true
	case "created_at":
		return s.CreatedAt, true
	case "updated_at":
		return s.UpdatedAt, true
	}
	return nil, false
}

func sortSessions(list []Session, sorts []query.Sort) {
	if len(sorts) == 0 {
		sorts = []query.Sort{{Column: "created_at", Dir: query.Desc}}
	}
	sort.SliceStable(list, func(i, j int) bool {
		for _, s := range sorts {
			c := compareSessions(list[i], list[j], s.Column)
			if c == 0 {
				continue
			}
			if s.Dir == query.Desc {
				return c > 0
			}
			return c < 0
		}
		return false
	})
}

func compareSessions(a, b Session, col string) int {
	av, aok := columnValue(a, col)
	bv, bok := columnValue(b, col)
	if !aok || !bok {
		return 0
	}
	return compareValues(av, bv)
}

func normalizePage(p query.Page) (size, number int) {
	size = p.Size
	if size <= 0 {
		size = query.DefaultPageSize
	}
	if size > query.MaxPageSize {
		size = query.MaxPageSize
	}
	number = p.Number
	if number <= 0 {
		number = 1
	}
	return size, number
}
