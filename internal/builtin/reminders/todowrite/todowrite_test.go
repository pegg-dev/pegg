package todowrite

import (
	"testing"

	"github.com/peggco/pegg/internal/agent"
	"github.com/peggco/pegg/internal/core/event"
	"github.com/peggco/pegg/internal/llm"
)

func startAgent(t *testing.T, bus event.Bus, name string) *agent.Agent {
	t.Helper()
	a := agent.New(name, agent.WithBus(bus))
	bus.Publish(agent.TopicAgentStarted, agent.AgentStarted{
		AgentID:   a.ID,
		AgentName: a.Name,
		Model:     a.Model,
		Agent:     a,
	})
	return a
}

func toolCallEvent(agentID, name string) agent.AgentToolCall {
	return agent.AgentToolCall{
		AgentID: agentID,
		Call:    llm.ToolCall{Function: llm.Function{Name: name}},
	}
}

func toolResultEvent(agentID, name string) agent.AgentToolResult {
	return agent.AgentToolResult{
		AgentID:  agentID,
		ToolName: name,
	}
}

func TestFiresAfter30ToolEvents(t *testing.T) {
	bus := event.New()
	rem := New()
	if err := rem.Start(bus); err != nil {
		t.Fatal(err)
	}
	defer rem.Stop(bus)

	a := startAgent(t, bus, "test-agent")

	for i := 0; i < 29; i++ {
		bus.Publish(agent.TopicAgentToolCall, toolCallEvent(a.ID, "bash"))
	}
	if a.HasPendingNotifications() {
		t.Fatal("should not fire at 29 events")
	}

	bus.Publish(agent.TopicAgentToolCall, toolCallEvent(a.ID, "bash"))
	if !a.HasPendingNotifications() {
		t.Fatal("should fire at 30 events")
	}
}

func TestDoesNotFireWithoutTools(t *testing.T) {
	bus := event.New()
	rem := New()
	if err := rem.Start(bus); err != nil {
		t.Fatal(err)
	}
	defer rem.Stop(bus)

	a := startAgent(t, bus, "test-agent")

	for i := 0; i < 100; i++ {
		bus.Publish(agent.TopicAgentToken, agent.AgentToken{
			AgentID: a.ID,
			Content: "thinking and content generation does not count",
		})
	}
	if a.HasPendingNotifications() {
		t.Fatal("should not fire for token-only activity")
	}
}

func TestTodoWriteResetsCounter(t *testing.T) {
	bus := event.New()
	rem := New()
	if err := rem.Start(bus); err != nil {
		t.Fatal(err)
	}
	defer rem.Stop(bus)

	a := startAgent(t, bus, "test-agent")

	for i := 0; i < 29; i++ {
		bus.Publish(agent.TopicAgentToolCall, toolCallEvent(a.ID, "bash"))
	}
	bus.Publish(agent.TopicAgentToolCall, toolCallEvent(a.ID, todoWriteTool))
	if a.HasPendingNotifications() {
		t.Fatal("todowrite usage should not queue a reminder")
	}

	for i := 0; i < 29; i++ {
		bus.Publish(agent.TopicAgentToolResult, toolResultEvent(a.ID, "bash"))
	}
	if a.HasPendingNotifications() {
		t.Fatal("should not fire at 29 events after reset")
	}

	bus.Publish(agent.TopicAgentToolResult, toolResultEvent(a.ID, "bash"))
	if !a.HasPendingNotifications() {
		t.Fatal("should fire at 30 events after reset")
	}
}

func TestToolResultsCount(t *testing.T) {
	bus := event.New()
	rem := New()
	if err := rem.Start(bus); err != nil {
		t.Fatal(err)
	}
	defer rem.Stop(bus)

	a := startAgent(t, bus, "test-agent")

	for i := 0; i < 30; i++ {
		bus.Publish(agent.TopicAgentToolResult, toolResultEvent(a.ID, "read"))
	}
	if !a.HasPendingNotifications() {
		t.Fatal("tool results should count as stream events")
	}
}

func TestTodoWriteResultsResetToo(t *testing.T) {
	bus := event.New()
	rem := New()
	if err := rem.Start(bus); err != nil {
		t.Fatal(err)
	}
	defer rem.Stop(bus)

	a := startAgent(t, bus, "test-agent")

	for i := 0; i < 29; i++ {
		bus.Publish(agent.TopicAgentToolCall, toolCallEvent(a.ID, "bash"))
	}
	bus.Publish(agent.TopicAgentToolResult, toolResultEvent(a.ID, todoWriteTool))

	for i := 0; i < 29; i++ {
		bus.Publish(agent.TopicAgentToolCall, toolCallEvent(a.ID, "bash"))
	}
	if a.HasPendingNotifications() {
		t.Fatal("should not fire at 29 events after reset")
	}
}

func TestFiresAgainAfterReminder(t *testing.T) {
	bus := event.New()
	rem := New()
	if err := rem.Start(bus); err != nil {
		t.Fatal(err)
	}
	defer rem.Stop(bus)

	a := startAgent(t, bus, "test-agent")

	for i := 0; i < 30; i++ {
		bus.Publish(agent.TopicAgentToolCall, toolCallEvent(a.ID, "bash"))
	}
	if !a.HasPendingNotifications() {
		t.Fatal("should fire at 30 events")
	}

	for i := 0; i < 29; i++ {
		bus.Publish(agent.TopicAgentToolCall, toolCallEvent(a.ID, "bash"))
	}
	if !a.HasPendingNotifications() {
		t.Fatal("should still have the first pending notification")
	}
}

func TestStateCleanupOnFinishedAndError(t *testing.T) {
	for _, topic := range []string{agent.TopicAgentFinished, agent.TopicAgentError} {
		bus := event.New()
		rem := New()
		if err := rem.Start(bus); err != nil {
			t.Fatal(err)
		}
		defer rem.Stop(bus)

		a := startAgent(t, bus, "test-agent")
		bus.Publish(agent.TopicAgentToolCall, toolCallEvent(a.ID, "bash"))

		if topic == agent.TopicAgentFinished {
			bus.Publish(agent.TopicAgentFinished, agent.AgentFinished{AgentID: a.ID})
		} else {
			bus.Publish(agent.TopicAgentError, agent.AgentError{AgentID: a.ID})
		}

		rem.mu.Lock()
		_, exists := rem.states[a.ID]
		rem.mu.Unlock()
		if exists {
			t.Fatalf("state should be cleaned up after %s", topic)
		}
	}
}

func TestMultipleAgentsTrackedIndependently(t *testing.T) {
	bus := event.New()
	rem := New()
	if err := rem.Start(bus); err != nil {
		t.Fatal(err)
	}
	defer rem.Stop(bus)

	a1 := startAgent(t, bus, "agent-1")
	a2 := startAgent(t, bus, "agent-2")

	for i := 0; i < 30; i++ {
		bus.Publish(agent.TopicAgentToolCall, toolCallEvent(a1.ID, "bash"))
	}

	if !a1.HasPendingNotifications() {
		t.Fatal("agent-1 should have notification")
	}
	if a2.HasPendingNotifications() {
		t.Fatal("agent-2 should not have notification")
	}
}

func TestNoAgentOnStartedSkipsQueue(t *testing.T) {
	bus := event.New()
	rem := New()
	if err := rem.Start(bus); err != nil {
		t.Fatal(err)
	}
	defer rem.Stop(bus)

	bus.Publish(agent.TopicAgentStarted, agent.AgentStarted{
		AgentID:   "orphan",
		AgentName: "orphan",
	})
	for i := 0; i < 30; i++ {
		bus.Publish(agent.TopicAgentToolCall, toolCallEvent("orphan", "bash"))
	}
}
