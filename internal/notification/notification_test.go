package notification

import (
	"errors"
	"strings"
	"testing"

	"github.com/peggco/pegg/internal/agent"
	"github.com/peggco/pegg/internal/core/config"
	"github.com/peggco/pegg/internal/core/event"
)

func newTestNotifier(t *testing.T, enabled bool) (*Notifier, *fakeDriver, event.Bus) {
	t.Helper()
	bus := event.New()
	fake := &fakeDriver{}
	n := New(bus, nil, enabled, fake)
	if err := n.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = n.Close() })
	return n, fake, bus
}

func TestNotifierAskNotifiesWhenBlurred(t *testing.T) {
	n, fake, bus := newTestNotifier(t, true)

	bus.Publish(agent.TopicAgentAsk, agent.AgentAsk{
		AgentID:   "a1",
		AgentName: "coder",
		Questions: []agent.AskQuestion{{ID: "decision", Question: `Tool "bash" requested permission.`}},
	})
	bus.WaitAsync()

	if got := fake.notifications(); len(got) != 0 {
		t.Fatalf("expected no notification while focused, got %v", got)
	}

	n.SetFocused(false)
	bus.Publish(agent.TopicAgentAsk, agent.AgentAsk{
		AgentID:   "a1",
		AgentName: "coder",
		Questions: []agent.AskQuestion{{ID: "decision", Question: `Tool "bash" requested permission.`}},
	})
	bus.WaitAsync()

	got := fake.notifications()
	if len(got) != 1 {
		t.Fatalf("notifications: got %d, want 1", len(got))
	}
	if !strings.Contains(got[0].Title, "Permission required") {
		t.Fatalf("title: got %q, want it to contain %q", got[0].Title, "Permission required")
	}
}

func TestNotifierAskNonPermissionTitle(t *testing.T) {
	n, fake, bus := newTestNotifier(t, true)
	n.SetFocused(false)

	bus.Publish(agent.TopicAgentAsk, agent.AgentAsk{
		AgentID:   "a1",
		AgentName: "coder",
		Questions: []agent.AskQuestion{{ID: "q", Question: "Which option do you prefer?"}},
	})
	bus.WaitAsync()

	got := fake.notifications()
	if len(got) != 1 {
		t.Fatalf("notifications: got %d, want 1", len(got))
	}
	if !strings.Contains(got[0].Title, "Input required") {
		t.Fatalf("title: got %q, want it to contain %q", got[0].Title, "Input required")
	}
}

func TestNotifierFinished(t *testing.T) {
	n, fake, bus := newTestNotifier(t, true)
	n.SetFocused(false)

	bus.Publish(agent.TopicAgentFinished, agent.AgentFinished{
		AgentID:   "a1",
		AgentName: "coder",
		Output:    "All done!",
	})
	bus.WaitAsync()

	got := fake.notifications()
	if len(got) != 1 {
		t.Fatalf("notifications: got %d, want 1", len(got))
	}
	if !strings.Contains(got[0].Title, "Agent finished: coder") {
		t.Fatalf("title: got %q", got[0].Title)
	}
	if got[0].Message != "All done!" {
		t.Fatalf("message: got %q, want %q", got[0].Message, "All done!")
	}
}

func TestNotifierSubagentFiltered(t *testing.T) {
	n, fake, bus := newTestNotifier(t, true)
	n.SetFocused(false)

	bus.Publish(agent.TopicAgentStarted, agent.AgentStarted{
		AgentID:       "sub1",
		AgentName:     "worker",
		ParentAgentID: "a1",
	})
	bus.Publish(agent.TopicAgentFinished, agent.AgentFinished{AgentID: "sub1", AgentName: "worker", Output: "sub done"})
	bus.Publish(agent.TopicAgentError, agent.AgentError{AgentID: "sub1", AgentName: "worker", Err: errors.New("boom")})
	bus.WaitAsync()

	if got := fake.notifications(); len(got) != 0 {
		t.Fatalf("expected no notifications for subagents, got %v", got)
	}

	bus.Publish(agent.TopicAgentStarted, agent.AgentStarted{AgentID: "root1", AgentName: "orchestrator"})
	bus.Publish(agent.TopicAgentFinished, agent.AgentFinished{AgentID: "root1", AgentName: "orchestrator", Output: "root done"})
	bus.WaitAsync()

	if got := fake.notifications(); len(got) != 1 {
		t.Fatalf("notifications: got %d, want 1", len(got))
	}
}

func TestNotifierError(t *testing.T) {
	n, fake, bus := newTestNotifier(t, true)
	n.SetFocused(false)

	bus.Publish(agent.TopicAgentError, agent.AgentError{
		AgentID:   "a1",
		AgentName: "coder",
		Err:       errors.New("provider exploded"),
	})
	bus.WaitAsync()

	got := fake.notifications()
	if len(got) != 1 {
		t.Fatalf("notifications: got %d, want 1", len(got))
	}
	if !strings.Contains(got[0].Title, "Agent failed: coder") {
		t.Fatalf("title: got %q", got[0].Title)
	}
	if got[0].Message != "provider exploded" {
		t.Fatalf("message: got %q", got[0].Message)
	}
}

func TestNotifierFocusTopics(t *testing.T) {
	n, _, bus := newTestNotifier(t, true)

	if !n.Focused() {
		t.Fatal("expected to start focused")
	}

	bus.Publish(event.TopicAppBlurred)
	bus.Publish(event.TopicAppFocused)
	bus.Publish(event.TopicAppBlurred)
	bus.WaitAsync()

	if n.Focused() {
		t.Fatal("expected blurred state after app.blurred")
	}
}

func TestNotifierDisabled(t *testing.T) {
	n, fake, bus := newTestNotifier(t, false)
	n.SetFocused(false)

	bus.Publish(agent.TopicAgentFinished, agent.AgentFinished{AgentID: "a1", AgentName: "coder", Output: "x"})
	bus.WaitAsync()

	if got := fake.notifications(); len(got) != 0 {
		t.Fatalf("expected no notifications when disabled, got %v", got)
	}
	if n.Enabled() {
		t.Fatal("expected enabled accessor to report false")
	}
}

func TestNotifierFansOutToAllDrivers(t *testing.T) {
	bus := event.New()
	first := &fakeDriver{}
	second := &fakeDriver{}
	n := New(bus, nil, true, first, second)
	if err := n.Start(); err != nil {
		t.Fatal(err)
	}
	defer n.Close()
	n.SetFocused(false)

	bus.Publish(agent.TopicAgentFinished, agent.AgentFinished{AgentID: "a1", AgentName: "coder", Output: "done"})
	bus.WaitAsync()

	if got := first.notifications(); len(got) != 1 {
		t.Fatalf("first driver notifications: got %d, want 1", len(got))
	}
	if got := second.notifications(); len(got) != 1 {
		t.Fatalf("second driver notifications: got %d, want 1", len(got))
	}
}

func TestNotifierDriverErrorDoesNotBlockOthers(t *testing.T) {
	bus := event.New()
	failing := &fakeDriver{err: errors.New("no daemon")}
	working := &fakeDriver{}
	n := New(bus, nil, true, failing, working)
	if err := n.Start(); err != nil {
		t.Fatal(err)
	}
	defer n.Close()
	n.SetFocused(false)

	bus.Publish(agent.TopicAgentFinished, agent.AgentFinished{AgentID: "a1", AgentName: "coder", Output: "x"})
	bus.WaitAsync()

	if got := failing.notifications(); len(got) != 0 {
		t.Fatalf("failing driver should not record sends, got %v", got)
	}
	if got := working.notifications(); len(got) != 1 {
		t.Fatalf("working driver notifications: got %d, want 1", len(got))
	}
}

func TestNotifierCloseClosesAllDrivers(t *testing.T) {
	bus := event.New()
	first := &closeTrackingDriver{}
	second := &closeTrackingDriver{}
	n := New(bus, nil, true, first, second)
	if err := n.Start(); err != nil {
		t.Fatal(err)
	}

	if err := n.Close(); err != nil {
		t.Fatal(err)
	}
	if !first.closed || !second.closed {
		t.Fatalf("expected both drivers closed, got first=%v second=%v", first.closed, second.closed)
	}

	n.SetFocused(false)
	bus.Publish(agent.TopicAgentFinished, agent.AgentFinished{AgentID: "a1", AgentName: "coder", Output: "x"})
	bus.WaitAsync()

	if first.sends != 0 || second.sends != 0 {
		t.Fatalf("expected no notifications after close, got first=%d second=%d", first.sends, second.sends)
	}
}

func TestNotifierCloseUnsubscribes(t *testing.T) {
	bus := event.New()
	fake := &fakeDriver{}
	n := New(bus, nil, true, fake)
	if err := n.Start(); err != nil {
		t.Fatal(err)
	}
	if err := n.Close(); err != nil {
		t.Fatal(err)
	}
	n.SetFocused(false)

	bus.Publish(agent.TopicAgentFinished, agent.AgentFinished{AgentID: "a1", AgentName: "coder", Output: "x"})
	bus.WaitAsync()

	if got := fake.notifications(); len(got) != 0 {
		t.Fatalf("expected no notifications after close, got %v", got)
	}
}

func TestNotificationModuleViaRegistry(t *testing.T) {
	RegisterDriver("capture", func(config.NotificationConfig) (Driver, error) {
		return &fakeDriver{}, nil
	})

	bus := event.New()
	n, err := NotificationModule(config.NotificationConfig{Drivers: []string{"capture"}, Enabled: true}, bus, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer n.Close()

	fake, ok := n.drivers[0].(*fakeDriver)
	if !ok {
		t.Fatalf("driver type: got %T, want *fakeDriver", n.drivers[0])
	}

	n.SetFocused(false)
	bus.Publish(agent.TopicAgentAsk, agent.AgentAsk{
		AgentID: "a1",
		Questions: []agent.AskQuestion{
			{ID: "decision", Question: `Tool "write" requested permission.`},
		},
	})
	bus.WaitAsync()

	if got := fake.notifications(); len(got) != 1 {
		t.Fatalf("notifications: got %d, want 1", len(got))
	}
}

type closeTrackingDriver struct {
	closed bool
	sends  int
}

func (d *closeTrackingDriver) Notify(n Notification) error {
	d.sends++
	return nil
}

func (d *closeTrackingDriver) Close() error {
	d.closed = true
	return nil
}

func TestTruncateRunes(t *testing.T) {
	long := strings.Repeat("界", 300)
	got := truncateRunes(long, maxMessageRunes)
	if r := []rune(got); len(r) != maxMessageRunes+3 {
		t.Fatalf("length: got %d, want %d", len(r), maxMessageRunes+3)
	}
	if !strings.HasSuffix(got, "...") {
		t.Fatalf("expected ellipsis suffix, got %q", got[len(got)-3:])
	}
}
