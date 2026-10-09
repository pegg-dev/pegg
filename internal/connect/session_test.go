package connect

import (
	"context"
	"testing"

	json "github.com/goccy/go-json"

	"github.com/peggco/pegg/internal/core/config"
	"github.com/peggco/pegg/internal/core/event"
	"github.com/peggco/pegg/internal/llm"
	"github.com/peggco/pegg/internal/session"
)

func TestSessionMessagesSequential(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	bus := event.New()

	sess, err := session.SessionModule(config.DefaultConfig().Session, bus, testLogger())
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()

	s1, err := sess.Create(session.CreateOptions{Title: "a", ProjectDir: "/p"})
	if err != nil {
		t.Fatal(err)
	}
	s2, err := sess.Create(session.CreateOptions{Title: "b", ProjectDir: "/p"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sess.AppendMessage(s1.ID, llm.UserMessage("hello-1")); err != nil {
		t.Fatal(err)
	}
	if _, err := sess.AppendMessage(s2.ID, llm.UserMessage("hello-2")); err != nil {
		t.Fatal(err)
	}

	c, err := New(Deps{
		Config:   config.DefaultConfig(),
		Bus:      bus,
		Log:      testLogger(),
		Sessions: sess,
		Version:  "test",
	}, config.DefaultConfig().Connect)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Stop()

	load := func(id string) []SessionMessage {
		t.Helper()
		params, _ := json.Marshal(map[string]string{"id": id})
		raw, err := c.handleSessionMessages(context.Background(), &Request{Params: params})
		if err != nil {
			t.Fatalf("messages(%s): %v", id, err)
		}
		msgs, _ := raw.(map[string]any)["messages"].([]SessionMessage)
		return msgs
	}

	assert := func(msgs []SessionMessage, want string) {
		t.Helper()
		if len(msgs) != 1 {
			t.Fatalf("got %d messages, want 1", len(msgs))
		}
		if got, _ := msgs[0].Content.(string); got != want {
			t.Fatalf("content = %q, want %q", got, want)
		}
	}

	assert(load(s1.ID), "hello-1")
	assert(load(s2.ID), "hello-2")
	assert(load(s1.ID), "hello-1")
}
