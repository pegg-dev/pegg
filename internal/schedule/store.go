package schedule

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	json "github.com/goccy/go-json"

	"github.com/peggco/pegg/internal/core/config"
)

const (
	schedulesFileName = "schedules.json"
	runsDirName       = "schedule_runs"
)

type Store interface {
	Load() ([]Schedule, error)
	Save([]Schedule) error
	ModTime() (time.Time, error)

	AppendRun(rec RunRecord) error
	LoadRun(scheduleID, runID string) (RunRecord, error)
	ListRuns(scheduleID string) ([]RunRecord, error)
	PruneRuns(scheduleID string, limit int) error
	DeleteRuns(scheduleID string) error
}

type fileStore struct {
	baseDir string
}

func NewStore(baseDir string) (Store, error) {
	if baseDir == "" {
		dir, err := config.GetConfigDir()
		if err != nil {
			return nil, fmt.Errorf("schedule: resolve config dir: %w", err)
		}
		baseDir = dir
	}
	return &fileStore{baseDir: baseDir}, nil
}

func (s *fileStore) schedulesPath() string {
	return filepath.Join(s.baseDir, schedulesFileName)
}

func (s *fileStore) runsDir(scheduleID string) string {
	return filepath.Join(s.baseDir, runsDirName, scheduleID)
}

func (s *fileStore) Load() ([]Schedule, error) {
	data, err := os.ReadFile(s.schedulesPath())
	if err != nil {
		if os.IsNotExist(err) {
			return []Schedule{}, nil
		}
		return nil, fmt.Errorf("schedule: read schedules: %w", err)
	}
	if len(data) == 0 {
		return []Schedule{}, nil
	}
	var list []Schedule
	if err := json.Unmarshal(data, &list); err != nil {
		return nil, fmt.Errorf("schedule: parse schedules: %w", err)
	}
	return list, nil
}

func (s *fileStore) Save(list []Schedule) error {
	if err := os.MkdirAll(s.baseDir, 0o755); err != nil {
		return fmt.Errorf("schedule: create config dir: %w", err)
	}
	if list == nil {
		list = []Schedule{}
	}
	data, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return fmt.Errorf("schedule: marshal schedules: %w", err)
	}
	return atomicWrite(s.schedulesPath(), data)
}

func (s *fileStore) ModTime() (time.Time, error) {
	info, err := os.Stat(s.schedulesPath())
	if err != nil {
		if os.IsNotExist(err) {
			return time.Time{}, nil
		}
		return time.Time{}, err
	}
	return info.ModTime(), nil
}

func (s *fileStore) AppendRun(rec RunRecord) error {
	dir := s.runsDir(rec.ScheduleID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("schedule: create runs dir: %w", err)
	}
	data, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		return fmt.Errorf("schedule: marshal run: %w", err)
	}
	return atomicWrite(filepath.Join(dir, rec.RunID+".json"), data)
}

func (s *fileStore) LoadRun(scheduleID, runID string) (RunRecord, error) {
	var rec RunRecord
	if scheduleID == "" || runID == "" {
		return rec, fmt.Errorf("schedule: schedule id and run id are required")
	}
	data, err := os.ReadFile(filepath.Join(s.runsDir(scheduleID), runID+".json"))
	if err != nil {
		return rec, err
	}
	if err := json.Unmarshal(data, &rec); err != nil {
		return rec, fmt.Errorf("schedule: parse run: %w", err)
	}
	return rec, nil
}

func (s *fileStore) ListRuns(scheduleID string) ([]RunRecord, error) {
	entries, err := os.ReadDir(s.runsDir(scheduleID))
	if err != nil {
		if os.IsNotExist(err) {
			return []RunRecord{}, nil
		}
		return nil, fmt.Errorf("schedule: read runs: %w", err)
	}
	runs := make([]RunRecord, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(s.runsDir(scheduleID), e.Name()))
		if err != nil {
			continue
		}
		var rec RunRecord
		if err := json.Unmarshal(data, &rec); err != nil {
			continue
		}
		runs = append(runs, rec)
	}
	sort.Slice(runs, func(i, j int) bool { return runs[i].StartedAt.After(runs[j].StartedAt) })
	return runs, nil
}

func (s *fileStore) PruneRuns(scheduleID string, limit int) error {
	if limit <= 0 {
		return nil
	}
	runs, err := s.ListRuns(scheduleID)
	if err != nil {
		return err
	}
	if len(runs) <= limit {
		return nil
	}
	for _, rec := range runs[limit:] {
		_ = os.Remove(filepath.Join(s.runsDir(scheduleID), rec.RunID+".json"))
	}
	return nil
}

func (s *fileStore) DeleteRuns(scheduleID string) error {
	if scheduleID == "" {
		return nil
	}
	return os.RemoveAll(s.runsDir(scheduleID))
}

func atomicWrite(path string, data []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
