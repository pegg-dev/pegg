package subagent

import (
	"context"
	"fmt"

	"github.com/peggco/pegg/internal/agent"
	"github.com/peggco/pegg/internal/llm"
	"github.com/peggco/pegg/internal/session"
)

func SpawnForked(parent *agent.Agent, name, sessionID, prompt string) (string, error) {
	if parent == nil {
		return "", fmt.Errorf("subagent: no parent agent")
	}
	if parent.Provider == nil {
		return "", fmt.Errorf("subagent: parent agent has no provider configured")
	}
	if parent.Bus != nil {
		store.subscribe(parent.Bus)
	}

	sa, err := store.spawn(name, parent.Name, nil, true, parent)
	if err != nil {
		return "", err
	}

	go func() {
		sub := parent.Clone(name)
		sub.ParentAgentID = parent.ID
		sub.DisplayName = name
		var history []llm.Message
		if sessionID != "" && sessionReader != nil {
			if msgs, readErr := sessionReader.Messages(sessionID); readErr == nil && len(msgs) > 0 {
				history = session.MessagesToLLM(msgs)
			}
		}
		if parent.Bus != nil {
			store.setAgentID(sa.Name, sub.ID)
			if sessionID != "" {
				parent.Bus.Publish(session.TopicSessionResume, session.SessionResume{
					AgentID:   sub.ID,
					SessionID: sessionID,
				})
			}
		} else {
			store.start(sa)
		}
		res, runErr := sub.Resume(context.Background(), prompt, history)
		store.finish(sa, agentOutput(res, runErr), runErr)
	}()

	return fmt.Sprintf("Started background subagent %q.\n", name), nil
}
