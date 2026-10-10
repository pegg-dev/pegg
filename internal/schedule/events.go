package schedule

import "time"

const (
	TopicScheduleCreated  = "schedule.created"
	TopicScheduleUpdated  = "schedule.updated"
	TopicScheduleDeleted  = "schedule.deleted"
	TopicScheduleStarted  = "schedule.started"
	TopicScheduleFinished = "schedule.finished"
	TopicScheduleFailed   = "schedule.failed"
)

type ScheduleEvent struct {
	ScheduleID string
	Name       string
	Workspace  string
}

type RunEvent struct {
	RunID      string
	ScheduleID string
	Name       string
	SessionID  string
	Status     RunStatus
	Error      string
	Duration   time.Duration
}
