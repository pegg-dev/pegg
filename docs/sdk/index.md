---
icon: lucide/rocket
---

# Get Started

The Pegg Go SDK (`pkg/sdk`) embeds the full agent engine in your own program. Use
it to run agentic chats, raw completions, persistent sessions, sandboxed file
operations, and custom tools without a separate server process.

## Install

The SDK is part of the main module:

```bash
go get github.com/peggco/pegg
```

```go
import "github.com/peggco/pegg/pkg/sdk"
```

## Minimal example

```go
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/peggco/pegg/pkg/sdk"
)

func main() {
	eng, err := sdk.Open(context.Background(), sdk.Options{
		APIKeys:   map[string]string{"openai": os.Getenv("OPENAI_API_KEY")},
		Workspace: ".",
	})
	if err != nil {
		panic(err)
	}
	defer eng.Close()

	resp, err := eng.Chat(context.Background(), sdk.ChatRequest{
		Input:    "Summarize the README in two sentences.",
		Provider: "openai",
		Model:    "gpt-4o",
	})
	if err != nil {
		panic(err)
	}
	fmt.Println(resp.Output)
}
```

## Streaming

`ChatStream` delivers the same events the TUI shows: tokens, reasoning, tool calls,
tool results, and a final `done`:

```go
_, err = eng.ChatStream(context.Background(), sdk.ChatRequest{
	Input: "Fix the failing test.",
}, func(ev sdk.ChatEvent) error {
	switch ev.Type {
	case sdk.EventToken:
		fmt.Print(ev.Content)
	case sdk.EventToolCall:
		fmt.Printf("\n[tool] %s\n", ev.ToolCall.Function.Name)
	case sdk.EventToolResult:
		fmt.Printf("[result] %s\n", ev.ToolOutput)
	}
	return nil
})
```

Returning an error from the handler aborts the run.

## Configuration options

`Options` controls how the engine is wired:

| Field | Description |
|---|---|
| `Config` | A programmatically built `*Config`. Defaults to the config on disk |
| `ConfigDir` | Directory containing a `pegg.json` to load instead |
| `APIKeys` | Map of provider/driver name → API key, merged into the config |
| `Providers` | Additional `LLMConfig` entries appended to the config |
| `Workspace` | Root of the sandboxed filesystem. Defaults to the current directory |
| `SessionDB` | SQLite session database path |
| `SessionDir` | Directory for JSON-backed sessions |
| `Cache` | Custom `cache.Cache` implementation. Defaults to an in-memory cache |
| `Logger` | Custom `*Logger`. Defaults to a warn-level logger |
| `DisableBuiltins` | Skip registering built-in tools, agents, and middlewares |
| `DisableRecorder` | Skip automatic session recording |
| `DisableScheduler` | Skip the background scheduler. Scheduled tasks otherwise run in-process through `Chat` |

The `Open` call loads config, starts the LLM manager, mounts the VFS, registers
built-ins (once per process), and publishes the `app.mounted` event. **Only one
engine may be open per process** — a second `Open` returns `sdk.ErrAlreadyOpen`.

## Concepts

| Component | Where to read |
|---|---|
| Chat & completions | [Chat](chat.md) |
| Sessions | [Sessions](sessions.md) |
| Scheduling | [Scheduling](../pegg/features/scheduling.md#sdk) |
| File operations | [Files](files.md) |
| Custom tools, middlewares, agents, providers, skills | [Extending](extending.md) |
| Events and error handling | [Events & Errors](events-errors.md) |