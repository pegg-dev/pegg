package schedule

import (
	"bytes"
	"context"
	"os"
	"os/exec"

	"github.com/peggco/pegg/internal/core/logger"
	"github.com/peggco/pegg/internal/llm"
)

type ExecuteRequest struct {
	Schedule  Schedule
	RunID     string
	SessionID string
}

type ExecuteResult struct {
	ExitCode int
	Output   string
	Usage    llm.Usage
}

type Executor interface {
	Execute(ctx context.Context, req ExecuteRequest) ExecuteResult
}

type subprocessExecutor struct {
	log *logger.Logger
}

func newSubprocessExecutor(log *logger.Logger) Executor {
	return &subprocessExecutor{log: log}
}

func (e *subprocessExecutor) Execute(ctx context.Context, req ExecuteRequest) ExecuteResult {
	exe, err := os.Executable()
	if err != nil {
		return ExecuteResult{ExitCode: -1, Output: "resolve executable: " + err.Error()}
	}

	args := []string{
		"schedule", "run",
		"--id", req.Schedule.ID,
		"--run-id", req.RunID,
		"--session", req.SessionID,
	}
	cmd := exec.CommandContext(ctx, exe, args...)
	cmd.Dir = req.Schedule.Workspace
	cmd.Env = append(os.Environ(), "PEGG_SCHEDULE_RUN=1", "PEGG_SCHEDULER=0")

	var buf limitedBuffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf

	if err := cmd.Start(); err != nil {
		return ExecuteResult{ExitCode: -1, Output: "start scheduled run: " + err.Error()}
	}

	waitErr := cmd.Wait()
	result := ExecuteResult{Output: buf.String()}
	if cmd.ProcessState != nil {
		result.ExitCode = cmd.ProcessState.ExitCode()
	}
	if waitErr != nil && result.ExitCode == 0 {
		result.ExitCode = -1
	}
	if ctx.Err() != nil {
		result.ExitCode = -1
		if result.Output == "" {
			result.Output = ctx.Err().Error()
		}
	}
	if result.ExitCode != 0 && e.log != nil {
		e.log.Fdebug("schedule: run %s exited %d: %s", req.RunID, result.ExitCode, result.Output)
	}
	return result
}

const maxOutputBytes = 16 << 10

type limitedBuffer struct {
	buf bytes.Buffer
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	if b.buf.Len() < maxOutputBytes {
		remaining := maxOutputBytes - b.buf.Len()
		if remaining > len(p) {
			remaining = len(p)
		}
		b.buf.Write(p[:remaining])
	}
	return len(p), nil
}

func (b *limitedBuffer) String() string { return b.buf.String() }
