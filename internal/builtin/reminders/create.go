package reminders

import (
	"github.com/peggco/pegg/internal/builtin/reminders/tasks"
	"github.com/peggco/pegg/internal/builtin/reminders/todowrite"
	"github.com/peggco/pegg/internal/builtin/reminders/usage"
	"github.com/peggco/pegg/internal/core/event"
)

func Create(bus event.Bus) error {
	if bus == nil {
		return nil
	}
	if err := usage.New().Start(bus); err != nil {
		return err
	}
	if err := todowrite.New().Start(bus); err != nil {
		return err
	}
	if err := tasks.New().Start(bus); err != nil {
		return err
	}
	return nil
}
