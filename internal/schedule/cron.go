package schedule

import (
	"fmt"
	"time"

	"github.com/robfig/cron/v3"
)

var parser = cron.NewParser(
	cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor,
)

func Parse(spec string) (cron.Schedule, error) {
	if spec == "" {
		return nil, fmt.Errorf("schedule: cron expression is required")
	}
	sched, err := parser.Parse(spec)
	if err != nil {
		return nil, fmt.Errorf("schedule: invalid cron expression %q: %w", spec, err)
	}
	return sched, nil
}

func Validate(spec string) error {
	_, err := Parse(spec)
	return err
}

func NextRun(spec string, from time.Time) (*time.Time, error) {
	sched, err := Parse(spec)
	if err != nil {
		return nil, err
	}
	next := sched.Next(from)
	if next.IsZero() {
		return nil, nil
	}
	return &next, nil
}
