package schedule

import (
	"time"

	"github.com/peggco/pegg/internal/llm"
)

type RunStatus string

const (
	StatusRunning     RunStatus = "running"
	StatusSuccess     RunStatus = "success"
	StatusFailed      RunStatus = "failed"
	StatusTimeout     RunStatus = "timeout"
	StatusInterrupted RunStatus = "interrupted"
)

const (
	DefaultTimeoutSecs = 0
	DefaultMaxParallel = 1
)

type Schedule struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Cron        string     `json:"cron"`
	Prompt      string     `json:"prompt"`
	Workspace   string     `json:"workspace"`
	Provider    string     `json:"provider,omitempty"`
	Model       string     `json:"model,omitempty"`
	Tags        []string   `json:"tags,omitempty"`
	TimeoutSecs int        `json:"timeout_secs,omitempty"`
	MaxParallel int        `json:"max_parallel,omitempty"`
	Enabled     bool       `json:"enabled"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	NextRun     *time.Time `json:"next_run,omitempty"`
	LastRun     *time.Time `json:"last_run,omitempty"`
}

type RunRecord struct {
	RunID      string     `json:"run_id"`
	ScheduleID string     `json:"schedule_id"`
	SessionID  string     `json:"session_id,omitempty"`
	Provider   string     `json:"provider,omitempty"`
	Model      string     `json:"model,omitempty"`
	Status     RunStatus  `json:"status"`
	PID        int        `json:"pid,omitempty"`
	Trigger    string     `json:"trigger,omitempty"`
	StartedAt  time.Time  `json:"started_at"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
	DurationMs int64      `json:"duration_ms,omitempty"`
	ExitCode   int        `json:"exit_code,omitempty"`
	Error      string     `json:"error,omitempty"`
	Usage      llm.Usage  `json:"usage,omitempty"`
}

type CreateOptions struct {
	Name        string
	Cron        string
	Prompt      string
	Workspace   string
	Provider    string
	Model       string
	Tags        []string
	TimeoutSecs int
	MaxParallel int
	Disabled    bool
}

type UpdateOptions struct {
	Name        *string
	Cron        *string
	Prompt      *string
	Workspace   *string
	Provider    *string
	Model       *string
	Tags        *[]string
	TimeoutSecs *int
	MaxParallel *int
	Enabled     *bool
}

type Stats struct {
	Total         int        `json:"total"`
	Success       int        `json:"success"`
	Failed        int        `json:"failed"`
	Running       int        `json:"running"`
	SuccessRate   float64    `json:"success_rate"`
	AvgDurationMs int64      `json:"avg_duration_ms"`
	LastRun       *time.Time `json:"last_run,omitempty"`
	LastFailure   *time.Time `json:"last_failure,omitempty"`
}

func (s *Schedule) MaxParallelValue() int {
	if s == nil || s.MaxParallel <= 0 {
		return DefaultMaxParallel
	}
	return s.MaxParallel
}

func (s *Schedule) TimeoutValue() time.Duration {
	if s == nil || s.TimeoutSecs <= 0 {
		return 0
	}
	return time.Duration(s.TimeoutSecs) * time.Second
}
