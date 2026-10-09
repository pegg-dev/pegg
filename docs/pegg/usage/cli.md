---
icon: lucide/terminal
---

# CLI

The `pegg` binary is both the TUI launcher and a full command-line tool. Run
`pegg help` or `pegg <command> --help` at any time.

## Invocation behavior

| Invocation | Behavior |
|---|---|
| `pegg` on a terminal | Launches the [TUI](tui.md) |
| `pegg "message"` | Runs the orchestrator on the message (headless if stdin is not a terminal) |
| `echo "msg" \| pegg` | Reads stdin, appends it to any arguments, and runs the orchestrator |
| `pegg <command>` | Runs the subcommand |

Errors are printed to stderr as `pegg: <error>` and the process exits with code
`1`. Successful commands exit `0`. Interactive prompts cancelled with ++ctrl+c++
also exit `1`.

## `pegg run`

Run the orchestrator agent on a message, streaming its progress, tools, thinking,
and responses live. Without a message (or with `--chat`) it starts an interactive
chat in the same session.

```bash
pegg run "Explain the build system"
pegg run --provider openai --model gpt-4o "Review internal/agent"
pegg run --model smart-router "Fix the flaky tests"   # needs smart_router.enabled
pegg run --session 3f2b1c... "Continue where we left off"
```

| Flag | Default | Description |
|---|---|---|
| `--provider` | auto | Provider to use |
| `--model` | auto | Model to use. `smart-router` selects the best model per task/agent (see [Smart Router](../providers-and-models.md#smart-router)) |
| `--select-model` | `false` | List available models and let you pick one (overrides `--model`) |
| `--session` | — | Session ID to continue |
| `--select-session` | `false` | List sessions and pick one to continue |
| `--file` | — | File to attach (repeatable) |
| `--show-thinking` | `false` | Print raw reasoning content instead of a `Thinking...` indicator |
| `--show-subagent` | `false` | Print subagent messages instead of a `Subagent` indicator |
| `--chat` | `false` | Keep chatting after the first run ends |

Model resolution follows the rules in
[Providers & Models](../providers-and-models.md#model-selection). When resuming a
session without an explicit provider/model, the session's stored model is reused.

### Output

Streaming output is plain text, with ANSI colors only when stdout is a terminal:

```
Running orchestrator (openai/gpt-4o)

> Explain the build system
tool [orchestrator] read(path=Makefile)
result [orchestrator] read: Path: Makefile | ...
The build is driven by a small Makefile...
• orchestrator finished (2 iterations, 1.2K tokens, $0.003100)
```

- Tool calls are shown as `tool [agent] name(args...)`; results are truncated to 500
  characters.
- `--show-thinking` prints the model's raw reasoning; otherwise a `Thinking...`
  spinner is shown on a TTY.
- `--show-subagent` prints subagent content instead of a `Subagent <name>` line.
- When the context is compacted mid-run, a line like
  `↻ Context compacted (sliding-window) — 30 messages, 8000 tokens` is printed.

### Chat mode

With `--chat`, or when the message is empty, `run` enters a line-based chat loop:

```
chat mode: type a message, exit/quit to end
> add a test for the parser
...
```

Exit words: `exit`, `quit`, `/exit`, `/quit`, `bye`.

### Attachments

`--file` can be repeated. The MIME type is inferred from the extension — images
(`.png`, `.jpg`, `.jpeg`, `.gif`, `.webp`) and audio (`.mp3`, `.wav`) are sent as
attachments, PDFs and text files as files. See
[Adding Context](../features/adding-context.md).

### Session selection

`--select-session` opens a paginated picker (10 per page) filtered to sessions from
the current project directory, showing title, provider/model, and creation time.

## `pegg serve`

Start a server: the HTTP REST API by default, or ACP with `--acp`.

```bash
pegg serve                          # HTTP API on 127.0.0.1:8080
pegg serve --port 9000 --host 0.0.0.0
pegg serve --acp                    # ACP over HTTP at /acp
pegg serve --acp --stdio            # ACP over stdin/stdout
```

| Flag | Default | Description |
|---|---|---|
| `--port` | config (`8080`) | Port to listen on |
| `--host` | config (`127.0.0.1`) | Host to bind to |
| `--acp` | `false` | Start the ACP server instead of the HTTP API |
| `--stdio` | `false` | Use the stdio transport (only with `--acp`) |

See [HTTP](http.md) and [ACP](acp.md).

## `pegg login`

Add or update a provider with an API key.

```bash
pegg login
pegg login --provider anthropic --api-key sk-ant-...
```

| Flag | Description |
|---|---|
| `--provider` | Provider name. Interactive picker when omitted |
| `--api-key` | API key. Masked prompt when omitted; may be empty |

The provider's model list is fetched before the config is saved, so a login only
succeeds when the credentials work.

## `pegg connect`

Connect this machine's agent to a website backend so you can chat with it from a
browser. Only a pairing token is needed — create one on the website under
**Agents → Connect a device**. The full walkthrough lives in
[Connect](../features/connect.md).

```bash
pegg connect <token>     # first run: connect and remember the token
pegg connect             # every run after that
```

| Subcommand | Description |
|---|---|
| `pegg connect login [token]` | Save a pairing token without connecting (masked prompt when omitted) |
| `pegg connect logout` | Forget the saved token (`--all` also resets the device identity) |
| `pegg connect status` | Relay, saved token (masked), and device identity |
| `pegg connect whoami` | This machine's device ID and public key |
| `pegg connect methods` | List every method the website may call |
| `pegg connect pair` | Print the device identity to register with a backend |

| Flag | Description |
|---|---|
| `--token` | Pairing token (may also be passed as the argument) |
| `--auto-approve` | Skip local confirmation for sensitive remote actions (not recommended) |
| `--insecure` | Allow `ws://` to a non-loopback host (not recommended) |
| `--device-name` | Human-friendly name for this device |
| `--allowed-method` | Restrict methods the website may call (repeatable, supports `prefix*`) |
| `--max-concurrency` | Maximum parallel runs (default from config) |
| `--response-timeout` | Seconds a run may produce no output before it is failed |

The relay endpoint is built into the app, so nothing but a token is needed; point a
self-hosted website at `connect.relay_url` in `~/.peggco/pegg.json`. The pairing
token is stored there too, and is only ever displayed masked.

## `pegg providers`

| Command | Description |
|---|---|
| `pegg providers add` | Add a provider with full config options |
| `pegg providers list` | List providers with masked key and cached model count |
| `pegg providers refresh [--provider NAME]` | Re-fetch models, bypassing the cache |
| `pegg providers remove NAME` | Remove a provider and its cached models |

`pegg providers add` exposes every provider config field:

```bash
pegg providers add --name anthropic --api-key your-api-key
pegg providers add --driver openai --base-url http://localhost:11434/v1 --api-key ollama
```

| Flag | Description |
|---|---|
| `--name` | Registered provider name, or a custom name when combined with `--driver`. Interactive picker (with a "Custom endpoint" option) when omitted |
| `--driver` | Driver for custom endpoints (`openai`, `claude`, `gemini`). Requires `--base-url` |
| `--api-key` | API key. Masked prompt in interactive mode; may be empty |
| `--base-url` | Base URL. Required for driver entries |
| `--header KEY=VALUE` | Request header (repeatable) |
| `--timeout` | Request timeout in seconds |
| `--max-retries` | Maximum retry count |

Running it with any of `--name`, `--driver`, or `--base-url` skips all prompts.
Models are fetched in the background after saving; a failed fetch does not block
the add — run `pegg providers refresh` to retry. Registered names use built-in
endpoints; `--base-url`/`--driver` only affect driver entries and custom names.

## `pegg models`

List cached models.

```bash
pegg models
pegg models --provider deepseek
```

| Flag | Description |
|---|---|
| `--provider` | Only show models for this provider |

## `pegg doctor`

Check provider connectivity and look for updates.

```
checking providers...
  openai OK (64 models)
  groq FAIL  llm: provider error: status 401
checking for updates...
  You are running the latest version
```

Each provider is queried with a fresh (uncached) model fetch. If a newer release
exists, `doctor` asks `Do you want to update? (y/N)` and can install it.

## `pegg config`

```bash
pegg config show
```

`show` prints the resolved configuration as pretty JSON with API keys masked. It is
the only `config` subcommand.

## `pegg sessions`

| Command | Description |
|---|---|
| `pegg sessions list` | List sessions for the current project |
| `pegg sessions show ID` | Show session metadata and messages |

`list` flags:

| Flag | Default | Description |
|---|---|---|
| `--search` | — | Search sessions by title |
| `--all` | `false` | Include sessions from all projects |
| `--page` | `1` | Page number |
| `--size` | `50` | Page size |

```bash
pegg sessions list --all --search refactor
pegg sessions show 3f2b1c8e-...
```

See [Sessions](../features/sessions.md).

## `pegg logs`

View application logs.

| Flag | Default | Description |
|---|---|---|
| `--search` | — | Search log messages |
| `--level` | — | Filter by `DEBUG`, `INFO`, `WARN`, or `ERROR` |
| `--page` | `1` | Page number |
| `--size` | `50` | Page size |

```bash
pegg logs --level ERROR
pegg logs --search "mcp" --size 100
```

With the SQLite logger (default) logs come from `~/.pegg/logs.db`. With the
console logger, only the current process's in-memory buffer is available.

## `pegg cache`

```bash
pegg cache clear
pegg cache clear --provider openai
```

| Flag | Description |
|---|---|
| `--provider` | Only clear this provider's cached models |

## `pegg files`

Print system file paths and whether they exist:

```
config  exists   /home/user/.peggco/pegg.json
logs    exists   /home/user/.pegg/logs.db
cache   exists   /home/user/.pegg/cache.db
sessions exists  /home/user/.pegg/sessions.db
lsps    missing  /home/user/.pegg/lsps
```

## `pegg mcp`

| Command | Description |
|---|---|
| `pegg mcp list [--global\|--project]` | List configured servers |
| `pegg mcp add [flags]` | Add a server (interactive unless flags are given) |
| `pegg mcp remove NAME [--global\|--project]` | Remove a server |
| `pegg mcp tools [--server NAME]` | List tools exposed by connected servers |

`add` flags: `--global`, `--project`, `--name`, `--transport local|http`,
`--command`, `--arg` (repeatable), `--env KEY=VALUE` (repeatable), `--url`,
`--header KEY=VALUE` (repeatable). See [MCP](../configurations/mcp.md).

## `pegg lsp`

| Command | Description |
|---|---|
| `pegg lsp list [--global\|--project]` | List language servers |
| `pegg lsp add [flags]` | Add a server (interactive unless flags are given) |
| `pegg lsp remove NAME [--global\|--project]` | Remove a server |
| `pegg lsp diag --file PATH` | Print cached diagnostics for a file |

`add` flags: `--global`, `--project`, `--name`, `--command`, `--arg` (repeatable),
`--filetype` (repeatable), `--rootmarker` (repeatable), `--download URL`,
`--env KEY=VALUE` (repeatable). See [LSP](../configurations/lsp.md).

## `pegg tui`

Launch the terminal UI explicitly. Equivalent to running `pegg` on a terminal.

## `pegg version`

```bash
pegg version
# pegg version 0.1.0
```

## `pegg update`

Check for a newer release and install it in place.

```
Current version: 0.1.0
Latest version: 0.2.0
Updating to version 0.2.0...
Successfully updated to version 0.2.0
```

## Shell completion

Cobra provides completion scripts for bash, zsh, fish, and PowerShell:

```bash
pegg completion bash > /etc/bash_completion.d/pegg
pegg completion zsh > "${fpath[1]}/_pegg"
pegg completion fish > ~/.config/fish/completions/pegg.fish
```

## Headless and scripting examples

```bash
# One-shot prompt, output to stdout
pegg run "List the public API of this package"

# Pipe context in
cat error.log | pegg "Explain the root cause"

# Run in CI with an explicit model and no interactive pickers
pegg run --provider openai --model gpt-4o-mini "Run the tests and fix failures"

# JSON-free plain text is emitted when stdout is not a terminal
OUTPUT="$(pegg run --provider groq --model llama-3.3-70b-versatile 'Summarize README.md')"
```

For structured output, use the [HTTP API](http.md) instead.
