---
icon: lucide/history
---

# Sessions

A session is a persistent conversation. Every turn — user input, assistant messages,
tool calls and results, usage — is recorded and can be resumed later, in any
interface.

## Storage

| Driver | Location | Notes |
|---|---|---|
| `sqlite` (default) | `~/.pegg/sessions.db` | Recommended; one table with full message history and snapshots |
| `json` | `~/.pegg/sessions/` | One JSON file per session |

Configure the driver with `session.driver` in [Config](../config.md#session). Sessions
record the `project_dir` at creation time, which is why listings default to the
current project.

## How sessions are created

- A session is created automatically the first time a run starts and none is active.
- The **recorder** subscribes to the agent event stream and persists messages as they
  happen — streamed tokens are committed in small batches, tool results and usage
  immediately.
- A **title generator** runs in the background on the first message: a dedicated
  model call (temperature 0.3) produces a title of at most 50 characters, in the
  same language as the message, with no tool names. Until then the title is
  `New Session <timestamp>`.
- Sessions track cumulative token usage and cost across all their runs.

## Resuming a session

=== "CLI"

    ```bash
    pegg run --session 3f2b1c8e-... "Continue the refactor"
    pegg run --select-session              # pick from a paginated list
    ```

    The stored history is replayed into the model context and shown as
    `--- session history ---`. Without an explicit provider/model, the session's
    stored provider and model are reused.

=== "TUI"

    Open Settings (++ctrl+p++) → **Session** → **Load session**. Sessions from the
    current directory are listed newest first. Loading restores the last 50
    messages; scrolling to the top loads older messages in batches of 50. A
    submitted message with an active session resumes it automatically.

=== "HTTP"

    ```bash
    curl -X POST http://127.0.0.1:8080/api/run \
      -H 'Content-Type: application/json' \
      -d '{"message":"continue","session_id":"3f2b1c8e-..."}'
    ```

=== "ACP"

    `session/resume` (no history replay) or `session/load` (replays history as
    `session/update` notifications). See [ACP](../usage/acp.md).

Resuming publishes a `session.resume` event so the recorder re-attaches the run to
the same session rather than creating a new one.

## Listing and inspecting

=== "CLI"

    ```bash
    pegg sessions list                    # current project only
    pegg sessions list --all --search fix # everywhere, filtered by title
    pegg sessions show 3f2b1c8e-...       # metadata + messages
    ```

=== "HTTP"

    ```bash
    curl 'http://127.0.0.1:8080/api/sessions?page=1&size=20'
    curl http://127.0.0.1:8080/api/sessions/3f2b1c8e-...
    curl http://127.0.0.1:8080/api/sessions/3f2b1c8e-.../messages
    ```

=== "ACP"

    `session/list` returns id, title, and timestamps for the most recent sessions.

Listings only show **original** sessions: compacted follow-up sessions (see below)
are internal views of the same conversation and never appear in session lists.

## Compaction and session chains

When [context compaction](../config.md#compaction) triggers, the compacted messages
are stored in a **new session linked to the current one**, forming a chain. The
original session keeps its full history; the newest session in the chain holds the
compacted view plus everything said afterwards.

- Resuming an original session opens the **newest session in its chain** — you see
  the latest compacted conversation first.
- In the TUI, scrolling to the top walks back through the chain: an
  `─ context compacted ─` divider marks each earlier conversation, ending with the
  original one.
- In the CLI, HTTP (SSE), and ACP, a compaction mid-run is surfaced as a
  `compaction` stream event / `↻ Context compacted (...)` line.
- Resuming re-applies compaction to the loaded history (sliding-window and
  tool-clearing), so resumed conversations stay within the context budget.
- Deleting an original session deletes its whole compaction chain.

## Managing sessions

| Action | CLI | TUI | HTTP | ACP |
|---|---|---|---|---|
| List | `sessions list` | Settings → Session → Load | `GET /api/sessions` | `session/list` |
| Show details | `sessions show <id>` | transcript | `GET /api/sessions/{id}` | — |
| Rename | — | Settings → Session → Change title | — | — |
| Delete | — | Settings → Session → Delete | `DELETE /api/sessions/{id}` | `session/delete` |
| Resume | `run --session` | Load session | `POST /api/run` | `session/resume` / `session/load` |

Deleting a session permanently removes its messages and usage from the store.

## Forking and reverting

The session layer also supports **forking** (copy a session up to a message into a
new session, recording a `parent_id`) and **reverting** (truncate after a message,
with the removed messages stored as a snapshot for later restore). These APIs are
exposed in the [Go SDK](../../sdk/sessions.md) but are not wired into the CLI, TUI, or
HTTP API yet.

## Tips

- Sessions are scoped to a project directory; use `--all` (CLI) to see everything.
- Run `pegg doctor` or check the status bar to see cumulative cost per session.
- ACP sessions created with `session/new` appear in listings titled `ACP Session`.