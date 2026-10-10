---
icon: lucide/rocket
---

# Pegg Overview

Pegg is a provider-agnostic AI coding agent written in Go. It runs as an interactive
terminal application, a headless CLI, an HTTP API server, an
[Agent Client Protocol](usage/acp.md) server, or as an embeddable
[Go SDK](../sdk/index.md).

The agent can read and edit files, run shell commands, search the web, manage todos,
delegate work to subagents, load skills, follow project rules, and talk to MCP and
language servers — all inside a sandboxed workspace.

## What is Pegg

<div class="grid cards" markdown>

- [:lucide-bot:{ .lg } **Multi-provider by design**](providers-and-models.md)

    Bring your own key for 32 cloud providers, or point a generic OpenAI-compatible
    driver at any local or self-hosted endpoint. Models are discovered from the
    provider API and enriched with context-window, pricing, and capability metadata.

- [:lucide-terminal:{ .lg } **Built for the terminal**](usage/tui.md)

    A tcell-based TUI with streaming markdown, syntax-highlighted diffs, tool cards,
    a subagent view, live context/cost tracking, 26 themes, and `@`/`/` pickers.

- [:lucide-shield-check:{ .lg } **Permissions you control**](features/permissions.md)

    Five permission modes per tool, an LLM judge, a persistent allow/reject store,
    a VFS sandbox with `.gitignore` awareness, and double-`Esc` emergency stop.

- [:lucide-heart-handshake:{ .lg } **Subagents and parallel work**](features/subagents.md)

    The orchestrator delegates to explorer, planner, and developer subagents, which
    run concurrently, persist their state, and can be resumed with follow-up messages.

- [:lucide-bell:{ .lg } **Reminders**](features/reminders.md)

    System reminders keep every agent informed mid-turn: usage and context-window
    status, background subagent results, and more — injected automatically.

- [:lucide-calendar-clock:{ .lg } **Scheduled automations**](features/scheduling.md)

    Run the agent on a cron schedule — daily summaries, weekly checks, recurring
    reminders — with each run saved as its own session. Fires while the TUI,
    server, or connect daemon is running.

- [:lucide-brain:{ .lg } **Project memory**](features/memory.md)

    A markdown-based memory system: an observer distills every run into
    observations, a librarian curates them, and future runs get a budget-bounded
    memory panel of what was done, decided, and learned.

- [:lucide-monitor-smartphone:{ .lg } **Connect to the web**](features/connect.md)

    Pair the app on your machine with the Pegg website and chat with your agent
    from a browser — sessions, settings, and tools stay local, and only a one-time
    pairing token is needed.

- [:lucide-puzzle:{ .lg } **Extensible**](configurations/skills.md)

    Skills, rules, MCP servers, LSP servers, custom tools, middlewares, providers,
    agents, and an event bus with WordPress-style hooks.

- [:lucide-cable:{ .lg } **Automation ready**](usage/http.md)

    Run one-shot headless prompts from CI, or expose the agent over a REST API
    (with SSE streaming) and ACP for editor integrations.

</div>

## Model Access

Pegg does not proxy your requests through a hosted service. You configure providers
locally and every request goes straight from your machine to the provider.

- **Bring your own key** — add providers with `pegg login`, the TUI settings page,
  or directly in `~/.peggco/pegg.json`.
- **Custom endpoints** — use the generic `openai`, `claude`, or `gemini` driver with a
  `base_url` to target any compatible gateway.
- **Local runtimes** — anything speaking the OpenAI chat-completions API works, for
  example a locally hosted inference server.

See [Providers & Models](providers-and-models.md) for the full list and
[Config](config.md) for the configuration reference.

## Applications

These are end-user applications built on top of Pegg's agent core:

<div class="grid cards" markdown>

- [:octicons-terminal-24:{ .lg } **CLI**](usage/cli.md)

    Interactive chat or fully headless automation for CI/CD and scripting.
    Piped stdin becomes the prompt; output streams as plain text.

    `pegg run "explain this repository"`

- [:octicons-browser-24:{ .lg } **TUI**](usage/tui.md)

    The default interface. Run `pegg` with no arguments on a terminal to open
    the chat UI with attachments, subagent transcripts, and live settings.

    `pegg`

- [:octicons-globe-24:{ .lg } **Server**](usage/http.md)

    A REST API with session management and Server-Sent Events streaming, plus an
    ACP endpoint for editor clients.

    `pegg serve`

- [:octicons-code-24:{ .lg } **ACP**](usage/acp.md)

    Speak JSON-RPC 2.0 with the agent over stdio, HTTP+SSE, or WebSocket.

    `pegg serve --acp --stdio`

- [:octicons-package-24:{ .lg } **Go SDK**](../sdk/index.md)

    Embed the engine in your own Go program: chat, completions, sessions, files,
    tools, and events.

    `go get github.com/peggco/pegg/pkg/sdk`

</div>

## Next steps

<div class="grid cards cards-list" markdown>

- [:lucide-download:{ .lg } **Install Pegg**](installation.md)

    Build or download the binary, then log in to a provider.

- [:lucide-settings:{ .lg } **Configure it**](config.md)

    Every key in `~/.peggco/pegg.json`, explained.

- [:lucide-square-terminal:{ .lg } **Learn the commands**](features/commands.md)

    `/init`, `/batch`, and `/review` are the built-in slash commands.

- [:lucide-bug:{ .lg } **Something broken?**](troubleshooting.md)

    Logs, cache resets, and the `doctor` command.

</div>
