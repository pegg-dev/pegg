package subagent

import (
	"github.com/peggco/pegg/internal/agent/tools"
	"github.com/peggco/pegg/internal/core/event"
	"github.com/peggco/pegg/internal/session"
)

type HistoryReader interface {
	Messages(sessionID string) ([]session.Message, error)
	Bus() event.Bus
}

var sessionReader HistoryReader

func SubAgentTools(reader HistoryReader) {
	sessionReader = reader
	if reader != nil {
		store.subscribe(reader.Bus())
	}
	tools.Register(subAgentTool())
	tools.Register(subAgentsStatusTool())
}
