---
icon: lucide/cog
---

# Config

Pegg is configured with a single JSON file per machine plus a few project-local
files. There is no YAML or TOML variant.

## File locations

| Scope | Path | Purpose |
|---|---|---|
| Global | `~/.peggco/pegg.json` | Providers, server, permission, theme, storage drivers, MCP and LSP servers |
| Project | `<project>/.pegg/` | Project data: `todos/`, `subagents/`, `plans/`, `rules/` |
| Project | `<project>/.mcp.json` | Project MCP servers |
| Project | `<project>/.lsp.json` | Project language servers |
| Global | `~/.pegg/permissions.json` | Remembered tool approvals and rejections |

The global file is created with defaults on first run. A missing file is not an
error — Pegg falls back to built-in defaults. An unparsable file is an error.

`pegg files` prints every resolved path and whether it exists.

## Viewing the config

```bash
pegg config show
```

Prints the resolved configuration as pretty-printed JSON with every API key masked.
There are no `config get` / `config set` subcommands — edit the file directly.

## Reference

### `providers[]`

Configured LLM providers. See [Providers & Models](providers-and-models.md).

| Key | Type | Default | Description |
|---|---|---|---|
| `provider` | string | — | Name of a registered provider (for example `"openai"`). Wins over `driver` |
| `driver` | string | — | Raw wire protocol: `"openai"`, `"claude"`, or `"gemini"`. Requires `base_url` |
| `api_key` | string | — | API key, stored in plaintext |
| `base_url` | string | — | Override the endpoint. Required for raw drivers, ignored by named providers |
| `timeout` | int | `120` | HTTP timeout in seconds for one-shot requests (model list sync, non-streaming calls). Streaming responses are never cut off by this — a model may reason for minutes before emitting the first token; stop a run with `Esc` |
| `max_retries` | int | — | Reserved; currently not read by the runtime |
| `headers` | object | — | Extra HTTP headers merged onto the driver defaults |

Either `provider` or `driver` must be set.

### `logger`

| Key | Type | Default | Description |
|---|---|---|---|
| `driver` | string | `"sqlite"` | `"sqlite"` writes `~/.pegg/logs.db`; `"console"` keeps an in-memory buffer printed to stdout |
| `max_log_count` | int | `1000` | Maximum retained log records |

### `cache`

| Key | Type | Default | Description |
|---|---|---|---|
| `driver` | string | `"sqlite"` | `"sqlite"` uses `~/.pegg/cache.db`; `"json"` uses `~/.pegg/cache.json` |

### `session`

| Key | Type | Default | Description |
|---|---|---|---|
| `driver` | string | `"sqlite"` | `"sqlite"` uses `~/.pegg/sessions.db`; `"json"` stores one file per session under `~/.pegg/sessions/` |

### `notification`

Desktop notifications while the app is in the background. See
[Notifications](features/notifications.md).

| Key | Type | Default | Description |
|---|---|---|---|
| `drivers` | string[] | `["os"]` | Drivers that receive every notification. `"os"` sends desktop notifications via the system notification service |
| `enabled` | bool | `true` | Master toggle. When `false`, no notification driver is created |

### `server`

Used by `pegg serve`. See [HTTP](usage/http.md) and [ACP](usage/acp.md).

| Key | Type | Default | Description |
|---|---|---|---|
| `host` | string | `"127.0.0.1"` | Bind address |
| `port` | int | `8080` | Listen port |
| `required_headers` | object | — | Required request headers for auth. A non-empty value must match (case-insensitively); an empty value only checks presence |
| `headers` | object | — | Extra headers added to every HTTP API response |

### `theme`

| Key | Type | Default | Description |
|---|---|---|---|
| `theme` | string | `"dark"` | TUI theme name. Changed at runtime with `Ctrl+T` and saved back to this file |

See [TUI themes](usage/tui.md#themes) for the list.

### `permission`

Controls when tools may run. See [Permissions](features/permissions.md).

| Key | Type | Default | Description |
|---|---|---|---|
| `default` | string | `"semi-ask"` | Mode when no rule matches: `allow`, `semi-ask`, `ask`, `semi-judge`, `judge` |
| `rules` | object | — | Per-tool mode overrides, for example `{"bash": "ask", "write": "allow"}` |
| `judge_provider` | string | — | Provider used by the judge. Decision-capable providers (currently `openrouter`) use a [decision model](features/permissions.md#the-judge-flow); other providers use the judge LLM. Unset → a decision model is used automatically when any decision-capable provider has an API key |
| `judge_model` | string | — | Model used by the judge. For decision providers, the decision model ID (for example `typesafe/jev-1.13`); otherwise the judge LLM model |
| `judge_threshold` | number | `0.6` | Minimum yes-probability for the decision judge to allow a call |

### `compaction`

Controls how Pegg manages context window pressure. Configurable from the
TUI in Settings → Session → Compaction.

| Key | Type | Default | Description |
|---|---|---|---|
| `enabled` | bool | `true` | Master toggle for compaction |
| `strategy` | string[] | `["tool-clearing", "sliding-window"]` | Ordered list of strategies to try. Options: `tool-clearing`, `sliding-window`, `summarization` |
| `threshold` | float | `80` | Percentage of the model's context window that triggers compaction |
| `max_messages` | int | `50` | Maximum recent messages to keep in sliding-window mode |
| `max_tool_output_chars` | int | `4000` | Character limit for tool outputs; longer outputs are truncated |
| `summarizer_provider` | string | — | Provider for the summarization strategy. Falls back to the agent's provider |
| `summarizer_model` | string | — | Model for the summarization strategy. Falls back to the agent's model |

**Strategies:**

- **tool-clearing** — truncates individual tool output messages exceeding
  `max_tool_output_chars`. Applied passively on every LLM call.
- **sliding-window** — when the threshold is reached, keeps only the most recent N
  messages (calculated from token budget, falling back to `max_messages`).
- **summarization** — when the threshold is reached, uses a dedicated LLM agent to
  compress the conversation history into a summary. Falls back to sliding-window on
  failure.

**Persistence:** When a compaction runs, Pegg does not overwrite the original
conversation. Instead, the compacted messages are stored in a **new session linked
to the previous one** (a linked-list chain of sessions). The original session keeps
its full history; the newest session in the chain holds the compacted messages and
everything that follows. Loading a session always opens the newest session in its
chain first — the compacted view — and scrolling up walks back through earlier
(pre-compaction) conversations. When a session is resumed, compaction thresholds are
re-checked against the loaded history, so sliding-window and tool-clearing also apply
to resumed sessions.

### `smart_router`

Selects the best model per agent and task instead of using one model for everything.
When enabled, a [decision model](providers-and-models.md#decision-models) (JEV by
default) picks a model from your configured providers for the orchestrator and each
subagent. Pick **Smart Router** in the TUI at Settings → General → Model (listed
above the regular models when enabled), or pass `--model smart-router` on the CLI
for a single run.

| Key | Type | Default | Description |
|---|---|---|---|
| `enabled` | bool | `false` | Master toggle. When on, the orchestrator and every subagent are routed |
| `provider` | string | — | Decision provider used for routing (currently `openrouter`). Unset → any decision-capable provider with an API key |
| `model` | string | — | Decision model used for routing (default `typesafe/jev-1.13`) |
| `agents` | object | — | Per-agent model preferences, keyed by agent name (`orchestrator`, `planner`, `explorer`, `developer`, ...) |

Each agent entry accepts:

| Key | Type | Description |
|---|---|---|
| `default` | string[] | Preferred model IDs for this agent. Matching models are offered first, in order |
| `difficulty` | object | Per-task-difficulty lists keyed by `trivial`, `moderate`, `complex`. When configured, the decision model scores the task's difficulty first and the matching bucket's models are offered first |
| `images` | string[] | Preferred model IDs when the task has image attachments (e.g. a vision variant) |

**Routing behavior:**

- Routing is a system hook (`OnModelResolve`) registered by the router and applied
  by every agent at run start, before the provider check — so it works uniformly
  in the CLI, TUI, HTTP/ACP servers, and the SDK without per-entry-point code. The
  routed model is what appears in sessions and usage records.
- The candidate list is built from every configured provider that has an API key.
  Per-agent preferences are offered first (by ID or display name), then all
  remaining models. Preference resolution: `images` when the task has
  attachments, else the `difficulty` bucket, else `default`.
- When the run has image attachments, models without image input support are
  dropped from the list (even beyond the `images` preference).
- The decision model's pick is always accepted — there is no confidence threshold.
- Resolution order: difficulty score question (when `difficulty` is configured for
  the agent) → decision model choice question → LLM selection (using the
  preferred model) → the preferred model itself.
- The orchestrator is routed when its run model is **Smart Router** (picked in
  the TUI model list or `--model smart-router`); subagents are always routed when
  the router is enabled.

### `memory`

Persistent, markdown-based memory for coding agents. A background observer distills
each run's tool uses into observations, a librarian curates them into markdown files,
and a memory panel is injected into future runs. See [Memory](features/memory.md) for
the full guide.

| Key | Type | Default | Description |
|---|---|---|---|
| `enabled` | bool | `true` | Master switch for capture, storage, and panel injection |
| `provider` | string | — | Provider for the observer/summarizer/librarian LLM calls. Unset → the running agent's model |
| `model` | string | — | Model for memory LLM calls (only read when `provider` is set) |
| `gate` | string | `"auto"` | Relevance gate for `mem-search`: `auto`, `decision`, `llm`, `score`, or `off` |
| `threshold` | number | `0.6` | Minimum decision-model relevance for `decision` gating |
| `context_budget` | int | `8000` | Max chars of the injected memory panel |
| `max_results` | int | `5` | Default result count for `mem-search` |
| `consolidate` | bool | `true` | Run the librarian to keep the curated memory files up to date |
| `skip_tools` | string[] | — | Additional tools whose results are never captured |

### `mcp_servers`

Map of server name to [MCP server config](configurations/mcp.md).

| Key | Type | Description |
|---|---|---|
| `command` | string | Executable to spawn (stdio transport) |
| `args` | string[] | Command arguments |
| `env` | object | Extra environment variables (merged over the inherited environment) |
| `url` | string | SSE endpoint. When set, the server is treated as a remote SSE server |
| `headers` | object | HTTP headers for SSE requests |

### `language_servers`

Map of server name to [language server config](configurations/lsp.md).

| Key | Type | Description |
|---|---|---|
| `command` | string | Language server executable |
| `args` | string[] | Command arguments |
| `env` | object | Extra environment variables |
| `filetypes` | string[] | File extensions this server handles |
| `rootMarkers` | string[] | Files used to detect the project root |
| `download` | string | Static binary URL to download when the command is not found |
| `install` | string | Shell command to install the server when the command is not found |
| `version` | string | Informational version |
| `enabled` | bool | `false` removes a server inherited from a lower-priority layer |

## Complete example

```json
{
  "providers": [
    { "provider": "openai", "api_key": "sk-..." },
    {
      "provider": "openrouter",
      "api_key": "sk-or-..."
    },
    {
      "provider": "groq",
      "api_key": "gsk_...",
      "timeout": 60,
      "headers": { "X-Custom": "value" }
    },
    {
      "driver": "openai",
      "base_url": "http://localhost:11434/v1",
      "api_key": "ollama"
    }
  ],
  "logger": { "driver": "sqlite", "max_log_count": 1000 },
  "cache": { "driver": "sqlite" },
  "session": { "driver": "sqlite" },
  "server": {
    "host": "127.0.0.1",
    "port": 8080,
    "required_headers": { "Authorization": "Bearer secret" },
    "headers": { "X-Served-By": "pegg" }
  },
  "theme": "dark",
  "permission": {
    "default": "semi-ask",
    "judge_provider": "openrouter",
    "judge_model": "typesafe/jev-1.13",
    "judge_threshold": 0.8,
    "rules": { "bash": "ask", "write": "allow" }
  },
  "compaction": {
    "enabled": true,
    "strategy": ["tool-clearing", "sliding-window"],
    "threshold": 80,
    "max_messages": 50,
    "max_tool_output_chars": 4000
  },
  "smart_router": {
    "enabled": true,
    "agents": {
      "orchestrator": {
        "default": ["deepseek-v4-flash"],
        "difficulty": {
          "trivial": ["deepseek-v4-flash"],
          "moderate": ["deepseek-v4-flash"],
          "complex": ["deepseek-v3", "deepseek-r1"]
        },
        "images": ["deepseek-v4-flash-vision-exp"]
      },
      "planner": {
        "default": ["qwen3-coder", "qwen2.5-coder-32b"],
        "difficulty": {
          "trivial": ["qwen2.5-coder-7b"],
          "complex": ["qwen3-coder", "qwen2.5-coder-32b"]
        }
      },
      "developer": {
        "default": ["qwen2.5-coder-7b"],
        "difficulty": {
          "complex": ["deepseek-v3", "claude-sonnet"]
        },
        "images": ["claude-sonnet"]
      }
    }
  },
  "memory": {
    "enabled": true,
    "gate": "auto",
    "threshold": 0.6,
    "context_budget": 8000,
    "max_results": 5,
    "consolidate": true
  },
  "mcp_servers": {
    "db": {
      "command": "npx",
      "args": ["-y", "@modelcontextprotocol/server-postgres"],
      "env": { "DEBUG": "true" }
    },
    "remote": {
      "url": "https://example.com/sse",
      "headers": { "Authorization": "Bearer token" }
    }
  },
  "language_servers": {
    "gopls": {
      "command": "gopls",
      "filetypes": ["go"],
      "rootMarkers": ["go.mod"]
    }
  }
}
```

## Project-local configuration

Only MCP and LSP servers support project-level files. Other settings are global.

```json title=".mcp.json"
{
  "mcpServers": {
    "db": { "command": "npx", "args": ["-y", "@modelcontextprotocol/server-postgres"] }
  }
}
```

```json title=".lsp.json"
{
  "languageServers": {
    "gopls": { "command": "gopls", "filetypes": ["go"], "rootMarkers": ["go.mod"] }
  }
}
```

Project entries override global entries with the same name. Setting
`"enabled": false` on a project LSP entry removes an inherited server.

## Environment variables

Pegg does not read API keys or config paths from environment variables. The only
variable that affects behavior is `HOME` (via the OS home directory), which determines
where `~/.pegg/` lives. Spawned MCP and LSP subprocesses inherit the parent process
environment plus any `env` entries you configure.
