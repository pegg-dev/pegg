package agent

import (
	"context"
	"strings"
	"testing"

	"github.com/peggco/pegg/internal/llm"
)

const hookMarker = "%%skill-test%%"

func registerMarkerHook(t *testing.T) {
	t.Helper()
	OnMessageInput(func(in MessageInput) MessageInput {
		if strings.Contains(in.Text, hookMarker) {
			in.Text += " [skill loaded]"
		}
		return in
	})
}

func TestOnMessageInputAppliedOnRun(t *testing.T) {
	registerMarkerHook(t)
	a, prov := newTestAgent(t)
	prov.responses = []mockResponse{{content: "done"}}

	if _, err := a.Run(context.Background(), "use skill "+hookMarker); err != nil {
		t.Fatal(err)
	}

	prov.mu.Lock()
	defer prov.mu.Unlock()
	last := prov.lastReq.Messages[len(prov.lastReq.Messages)-1]
	content, _ := last.Content.(string)
	if last.Role != llm.RoleUser || !strings.Contains(content, "[skill loaded]") {
		t.Fatalf("user message not expanded: %+v", last)
	}
}

func TestOnMessageInputAppliedOnResume(t *testing.T) {
	registerMarkerHook(t)
	a, prov := newTestAgent(t)
	prov.responses = []mockResponse{{content: "done"}}

	history := []llm.Message{llm.UserMessage("previous")}
	if _, err := a.Resume(context.Background(), "follow up "+hookMarker, history); err != nil {
		t.Fatal(err)
	}

	prov.mu.Lock()
	defer prov.mu.Unlock()
	last := prov.lastReq.Messages[len(prov.lastReq.Messages)-1]
	content, _ := last.Content.(string)
	if last.Role != llm.RoleUser || !strings.Contains(content, "[skill loaded]") {
		t.Fatalf("user message not expanded: %+v", last)
	}
}

const reminderMarker = "%%reminder-test%%"

func registerReminderHook(t *testing.T) {
	t.Helper()
	OnMessageInput(func(in MessageInput) MessageInput {
		for _, r := range in.SystemReminders {
			if strings.Contains(r, reminderMarker) {
				return in
			}
		}
		in.SystemReminders = append(in.SystemReminders, "## Memory\n- "+reminderMarker)
		return in
	})
}

func TestSystemRemindersSeededOnRun(t *testing.T) {
	registerReminderHook(t)
	a, prov := newTestAgent(t)
	prov.responses = []mockResponse{{content: "done"}}

	if _, err := a.Run(context.Background(), "hello"); err != nil {
		t.Fatal(err)
	}

	prov.mu.Lock()
	defer prov.mu.Unlock()
	msgs := prov.lastReq.Messages
	if len(msgs) < 2 {
		t.Fatalf("expected reminder, user; got %d messages", len(msgs))
	}
	reminder, _ := msgs[0].Content.(string)
	if msgs[0].Role != llm.RoleSystem || !strings.Contains(reminder, "<system-reminder>") ||
		!strings.Contains(reminder, reminderMarker) {
		t.Fatalf("reminder message = %+v", msgs[0])
	}
	user, _ := msgs[1].Content.(string)
	if msgs[1].Role != llm.RoleUser || !strings.Contains(user, "hello") {
		t.Fatalf("user message = %+v", msgs[1])
	}
}

func TestSystemRemindersSeededOnResume(t *testing.T) {
	registerReminderHook(t)
	a, prov := newTestAgent(t)
	prov.responses = []mockResponse{{content: "done"}}

	history := []llm.Message{llm.UserMessage("previous")}
	if _, err := a.Resume(context.Background(), "follow up", history); err != nil {
		t.Fatal(err)
	}

	prov.mu.Lock()
	defer prov.mu.Unlock()
	msgs := prov.lastReq.Messages
	if len(msgs) < 3 {
		t.Fatalf("expected history, reminder, user; got %d messages", len(msgs))
	}
	if msgs[0].Role != llm.RoleUser {
		t.Fatalf("first message role = %v, want user (history)", msgs[0].Role)
	}
	reminder, _ := msgs[1].Content.(string)
	if msgs[1].Role != llm.RoleSystem || !strings.Contains(reminder, "<system-reminder>") {
		t.Fatalf("reminder message = %+v", msgs[1])
	}
	if msgs[2].Role != llm.RoleUser {
		t.Fatalf("last message role = %v, want user", msgs[2].Role)
	}
}
