package sdk_test

import (
	"context"
	"fmt"
	"os"

	"github.com/peggco/pegg/pkg/sdk"
)

func Example() {
	eng, err := sdk.Open(context.Background(), sdk.Options{
		APIKeys:   map[string]string{"openai": os.Getenv("OPENAI_API_KEY")},
		Workspace: ".",
	})
	if err != nil {
		fmt.Println("open:", err)
		return
	}
	defer eng.Close()

	resp, err := eng.Chat(context.Background(), sdk.ChatRequest{
		Input:    "Summarize the README in two sentences.",
		Provider: "openai",
		Model:    "gpt-4o",
	})
	if err != nil {
		fmt.Println("chat:", err)
		return
	}
	fmt.Println(resp.Output)
}

func Example_engine_streaming() {
	eng, err := sdk.Open(context.Background(), sdk.Options{
		Workspace: ".",
	})
	if err != nil {
		return
	}
	defer eng.Close()

	_, err = eng.ChatStream(context.Background(), sdk.ChatRequest{
		Input: "Fix the failing test.",
	}, func(ev sdk.ChatEvent) error {
		switch ev.Type {
		case sdk.EventToken:
			fmt.Print(ev.Content)
		case sdk.EventToolCall:
			fmt.Printf("\n[tool] %s\n", ev.ToolCall.Function.Name)
		}
		return nil
	})
	if err != nil {
		return
	}
}

func Example_engine_sessions() {
	eng, err := sdk.Open(context.Background(), sdk.Options{})
	if err != nil {
		return
	}
	defer eng.Close()

	s, err := eng.CreateSession(sdk.CreateSessionOptions{Title: "demo"})
	if err != nil {
		return
	}
	_ = s

	_, _ = eng.Chat(context.Background(), sdk.ChatRequest{
		Input:     "Continue the session",
		SessionID: s.ID,
	})

	msgs, err := eng.SessionMessages(s.ID)
	if err != nil {
		return
	}
	for _, m := range msgs {
		fmt.Println(m.Role, m.Content)
	}
}

func Example_engine_registerTool() {
	eng, err := sdk.Open(context.Background(), sdk.Options{})
	if err != nil {
		return
	}
	defer eng.Close()

	t := sdk.NewTool("greet", "greets the user", map[string]any{
		"type": "object",
		"properties": map[string]any{
			"name": map[string]any{"type": "string"},
		},
		"required": []string{"name"},
	}, func(ctx context.Context, args string) (string, error) {
		return "hello, " + args, nil
	})
	if err := eng.RegisterTool(t); err != nil {
		return
	}
}
