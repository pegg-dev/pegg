package usage

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

func TestMessageCountFiresAt20(t *testing.T) {
	bus := event.New()
	rem := New()
	if err := rem.Start(bus); err != nil {
		t.Fatal(err)
	}
	defer rem.Stop(bus)

	a := startAgent(t, bus, "test-agent")
	model := llm.Model{ID: "m", Config: &llm.ModelConfig{MaxInputTokens: 100000}}

	for i := 0; i < 19; i++ {
		bus.Publish(agent.TopicAgentMessage, agent.AgentMessage{
			AgentID: a.ID,
			Model:   model,
		})
	}
	if a.HasPendingNotifications() {
		t.Fatal("should not fire at 19 messages")
	}

	bus.Publish(agent.TopicAgentMessage, agent.AgentMessage{
		AgentID: a.ID,
		Model:   model,
	})
	if !a.HasPendingNotifications() {
		t.Fatal("should fire at 20 messages")
	}
}

func TestMessageCountDoesNotFireBefore20(t *testing.T) {
	bus := event.New()
	rem := New()
	if err := rem.Start(bus); err != nil {
		t.Fatal(err)
	}
	defer rem.Stop(bus)

	a := startAgent(t, bus, "test-agent")
	model := llm.Model{ID: "m", Config: &llm.ModelConfig{MaxInputTokens: 100000}}

	for i := 0; i < 15; i++ {
		bus.Publish(agent.TopicAgentMessage, agent.AgentMessage{
			AgentID: a.ID,
			Model:   model,
		})
	}
	if a.HasPendingNotifications() {
		t.Fatal("should not fire before 20 messages")
	}
}

func TestTokenUsageFiresAt20Percent(t *testing.T) {
	bus := event.New()
	rem := New()
	if err := rem.Start(bus); err != nil {
		t.Fatal(err)
	}
	defer rem.Stop(bus)

	a := startAgent(t, bus, "test-agent")
	model := llm.Model{ID: "m", Config: &llm.ModelConfig{MaxInputTokens: 100000}}

	bus.Publish(agent.TopicAgentUsage, agent.AgentUsage{
		AgentID: a.ID,
		Model:   model,
		Usage:   llm.Usage{TotalTokens: 25000},
	})
	if !a.HasPendingNotifications() {
		t.Fatal("should fire at 25% usage")
	}
}

func TestTokenUsageDoesNotFireBelow20(t *testing.T) {
	bus := event.New()
	rem := New()
	if err := rem.Start(bus); err != nil {
		t.Fatal(err)
	}
	defer rem.Stop(bus)

	a := startAgent(t, bus, "test-agent")
	model := llm.Model{ID: "m", Config: &llm.ModelConfig{MaxInputTokens: 100000}}

	bus.Publish(agent.TopicAgentUsage, agent.AgentUsage{
		AgentID: a.ID,
		Model:   model,
		Usage:   llm.Usage{TotalTokens: 15000},
	})
	if a.HasPendingNotifications() {
		t.Fatal("should not fire at 15% usage")
	}
}

func TestTokenUsageFiresAtMultipleThresholds(t *testing.T) {
	bus := event.New()
	rem := New()
	if err := rem.Start(bus); err != nil {
		t.Fatal(err)
	}
	defer rem.Stop(bus)

	a := startAgent(t, bus, "test-agent")
	model := llm.Model{ID: "m", Config: &llm.ModelConfig{MaxInputTokens: 100000}}

	bus.Publish(agent.TopicAgentUsage, agent.AgentUsage{
		AgentID: a.ID,
		Model:   model,
		Usage:   llm.Usage{TotalTokens: 25000},
	})
	if !a.HasPendingNotifications() {
		t.Fatal("should fire at 20%")
	}

	bus.Publish(agent.TopicAgentUsage, agent.AgentUsage{
		AgentID: a.ID,
		Model:   model,
		Usage:   llm.Usage{TotalTokens: 45000},
	})
	if !a.HasPendingNotifications() {
		t.Fatal("should fire at 40%")
	}

	bus.Publish(agent.TopicAgentUsage, agent.AgentUsage{
		AgentID: a.ID,
		Model:   model,
		Usage:   llm.Usage{TotalTokens: 50000},
	})
	if !a.HasPendingNotifications() {
		t.Fatal("should still have pending notifications")
	}

	bus.Publish(agent.TopicAgentUsage, agent.AgentUsage{
		AgentID: a.ID,
		Model:   model,
		Usage:   llm.Usage{TotalTokens: 65000},
	})
	if !a.HasPendingNotifications() {
		t.Fatal("should fire at 60%")
	}
}

func TestNotificationContent(t *testing.T) {
	bus := event.New()
	rem := New()
	if err := rem.Start(bus); err != nil {
		t.Fatal(err)
	}
	defer rem.Stop(bus)

	a := startAgent(t, bus, "test-agent")
	model := llm.Model{ID: "m", Config: &llm.ModelConfig{MaxInputTokens: 100000}}

	bus.Publish(agent.TopicAgentUsage, agent.AgentUsage{
		AgentID: a.ID,
		Model:   model,
		Usage:   llm.Usage{TotalTokens: 25000},
	})

	if !a.HasPendingNotifications() {
		t.Fatal("should have pending notification")
	}
}

func TestStateCleanupOnFinished(t *testing.T) {
	bus := event.New()
	rem := New()
	if err := rem.Start(bus); err != nil {
		t.Fatal(err)
	}
	defer rem.Stop(bus)

	a := startAgent(t, bus, "test-agent")
	model := llm.Model{ID: "m", Config: &llm.ModelConfig{MaxInputTokens: 100000}}

	for i := 0; i < 5; i++ {
		bus.Publish(agent.TopicAgentMessage, agent.AgentMessage{
			AgentID: a.ID,
			Model:   model,
		})
	}

	bus.Publish(agent.TopicAgentFinished, agent.AgentFinished{
		AgentID: a.ID,
	})

	rem.mu.Lock()
	_, exists := rem.states[a.ID]
	rem.mu.Unlock()
	if exists {
		t.Fatal("state should be cleaned up after finished")
	}
}

func TestStateCleanupOnError(t *testing.T) {
	bus := event.New()
	rem := New()
	if err := rem.Start(bus); err != nil {
		t.Fatal(err)
	}
	defer rem.Stop(bus)

	a := startAgent(t, bus, "test-agent")
	model := llm.Model{ID: "m", Config: &llm.ModelConfig{MaxInputTokens: 100000}}

	for i := 0; i < 5; i++ {
		bus.Publish(agent.TopicAgentMessage, agent.AgentMessage{
			AgentID: a.ID,
			Model:   model,
		})
	}

	bus.Publish(agent.TopicAgentError, agent.AgentError{
		AgentID: a.ID,
	})

	rem.mu.Lock()
	_, exists := rem.states[a.ID]
	rem.mu.Unlock()
	if exists {
		t.Fatal("state should be cleaned up after error")
	}
}

func TestNoContextWindowSkipsTokenReminder(t *testing.T) {
	bus := event.New()
	rem := New()
	if err := rem.Start(bus); err != nil {
		t.Fatal(err)
	}
	defer rem.Stop(bus)

	a := startAgent(t, bus, "test-agent")
	model := llm.Model{ID: "m"}

	bus.Publish(agent.TopicAgentUsage, agent.AgentUsage{
		AgentID: a.ID,
		Model:   model,
		Usage:   llm.Usage{TotalTokens: 50000},
	})
	if a.HasPendingNotifications() {
		t.Fatal("should not fire when no context window info")
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
	model := llm.Model{ID: "m", Config: &llm.ModelConfig{MaxInputTokens: 100000}}

	for i := 0; i < 20; i++ {
		bus.Publish(agent.TopicAgentMessage, agent.AgentMessage{
			AgentID: a1.ID,
			Model:   model,
		})
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
	model := llm.Model{ID: "m", Config: &llm.ModelConfig{MaxInputTokens: 100000}}

	bus.Publish(agent.TopicAgentUsage, agent.AgentUsage{
		AgentID: "orphan",
		Model:   model,
		Usage:   llm.Usage{TotalTokens: 25000},
	})
	bus.Publish(agent.TopicAgentMessage, agent.AgentMessage{
		AgentID: "orphan",
		Model:   model,
	})
}
