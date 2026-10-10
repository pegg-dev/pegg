---
icon: lucide/link
---

# HTTP API

`pegg serve` starts a headless REST API server with session management and
Server-Sent Events (SSE) streaming. It uses the same engine as the CLI and TUI.

## Starting the server

```bash
pegg serve
# http server listening on 127.0.0.1:8080
```

| Flag | Config key | Default |
|---|---|---|
| `--host` | `server.host` | `127.0.0.1` |
| `--port` | `server.port` | `8080` |

```bash
pegg serve --host 0.0.0.0 --port 9000
```

The server shuts down gracefully on ++ctrl+c++ / SIGTERM with a 5 second timeout.
Read timeout is 30s, idle timeout 120s; the write timeout is disabled so SSE
connections can stay open.

## Authentication and CORS

Auth is opt-in through `server.required_headers` in
[Config](../config.md#server). Every request must carry each configured header:

```json
{
  "server": {
    "host": "127.0.0.1",
    "port": 8080,
    "required_headers": { "Authorization": "Bearer secret" }
  }
}
```

- A missing header returns `401 {"error": "missing required header: Authorization"}`.
- A configured value is compared case-insensitively; a mismatch returns
  `401 {"error": "invalid header: Authorization"}`.
- An empty configured value only requires the header to be present.

CORS is always enabled: `Access-Control-Allow-Origin: *`, methods
`GET, POST, PUT, DELETE, OPTIONS`, and headers `Content-Type, Authorization` (plus
any custom `server.headers`). `OPTIONS` requests receive `204 No Content`.

## Response format

All responses are JSON. Errors always use the same shape:

```json
{ "error": "message is required" }
```

## Endpoints

| Method | Path | Description |
|---|---|---|
| `GET` | `/api/health` | Liveness and version |
| `GET` | `/api/models` | Cached models for every configured provider |
| `POST` | `/api/run` | Run the orchestrator, synchronously or as SSE |
| `GET` | `/api/sessions` | List sessions |
| `GET` | `/api/sessions/{id}` | Get one session |
| `GET` | `/api/sessions/{id}/messages` | Get a session's messages |
| `DELETE` | `/api/sessions/{id}` | Delete a session |

### `GET /api/health`

```bash
curl http://127.0.0.1:8080/api/health
```

```json
{ "status": "ok", "version": "0.1.0" }
```

### `GET /api/models`

```bash
curl http://127.0.0.1:8080/api/models
```

```json
{
  "models": [
    {
      "provider": "openai",
      "model": { "id": "gpt-4o", "name": "GPT-4o", "owned_by": "openai" }
    }
  ]
}
```

Providers that fail to list models are skipped. The endpoint waits for the LLM
manager to finish its initial sync.

### `POST /api/run`

Request body:

| Field | Type | Required | Description |
|---|---|---|---|
| `message` | string | yes | The prompt |
| `provider` | string | no | Provider to use |
| `model` | string | no | Model to use |
| `session_id` | string | no | Resume an existing session |
| `files` | string[] | no | Server-side file paths to attach |
| `stream` | bool | no | `true` for SSE, `false` (default) for a single JSON response |

Model resolution mirrors the CLI: explicit provider+model must match, otherwise the
preferred model is selected with a 30 second timeout. Unknown `session_id` returns
`404`.

#### Synchronous response

```bash
curl -s http://127.0.0.1:8080/api/run \
  -H 'Content-Type: application/json' \
  -d '{"message":"Summarize README.md","provider":"openai","model":"gpt-4o"}'
```

```json
{
  "output": "Pegg is a provider-agnostic AI coding agent...",
  "usage": {
    "prompt_tokens": 812,
    "completion_tokens": 96,
    "total_tokens": 908,
    "cost": 0.0023
  },
  "iterations": 2,
  "finish_reason": "stop"
}
```

`finish_reason` is one of `stop`, `length`, `content_filter`, `tool_calls`, or
`null`. Agent failures return `500` with an `error` body.

#### Streaming response (SSE)

Set `"stream": true`. The server responds with `Content-Type: text/event-stream`
and flushes events as they are produced:

```
event: agent
data: {"type":"token","agent_id":"...","agent_name":"orchestrator","content":"Pegg"}

event: agent
data: {"type":"tool_call","tool_name":"read","tool_args":"{\"filePath\":\"README.md\"}"}

event: agent
data: {"type":"tool_result","tool_name":"read","tool_output":"Path: README.md ..."}

event: agent
data: {"type":"compaction","strategy":"sliding-window","messages":30,"tokens":8120}

event: agent
data: {"type":"done","done":true,"usage":{"prompt_tokens":812,"completion_tokens":96,"total_tokens":908}}

event: done
data: {"type":"done","done":true,"usage":{"prompt_tokens":812,"completion_tokens":96,"total_tokens":908}}
```

| Event name | Meaning |
|---|---|
| `agent` | A stream event (`token`, `tool_call`, `tool_result`, `compaction`, or the internal `done`) |
| `error` | `{"error": "..."}` — the run failed |
| `done` | Final event with usage |

Each event's `data` payload has these fields (empty fields omitted):

```json
{
  "type": "token",
  "agent_id": "…",
  "agent_name": "orchestrator",
  "content": "…",
  "reasoning": "…",
  "tool_name": "…",
  "tool_args": "…",
  "tool_output": "…",
  "error": "…",
  "usage": { "prompt_tokens": 0, "completion_tokens": 0, "total_tokens": 0 },
  "strategy": "…",
  "messages": 0,
  "tokens": 0,
  "done": false
}
```

When the context is compacted mid-run, an event with `"type":"compaction"` is
emitted carrying the `strategy` used, the number of messages the model now sees
(`messages`), and the prompt token count at compaction time (`tokens`).

!!! note

    A successful stream ends with two `done` payloads: one under `event: agent`
    (the agent's internal completion) and one under `event: done`. Event delivery
    uses a buffered channel with drop-on-backpressure semantics, so clients should
    treat the stream as best-effort telemetry and use the final `done` event for
    usage.

### Session endpoints

```bash
# List sessions (page/size optional; size is capped at 100)
curl 'http://127.0.0.1:8080/api/sessions?page=1&size=20'

# Get one session
curl http://127.0.0.1:8080/api/sessions/3f2b1c8e-...

# Get messages
curl http://127.0.0.1:8080/api/sessions/3f2b1c8e-.../messages

# Delete
curl -X DELETE http://127.0.0.1:8080/api/sessions/3f2b1c8e-...
# {"status":"deleted"}
```

Sessions are sorted by `updated_at` descending.

Session object:

```json
{
  "id": "3f2b1c8e-...",
  "title": "Fix parser bug",
  "provider": "openai",
  "model": "gpt-4o",
  "reasoning_effort": "medium",
  "project_dir": "/home/user/project",
  "parent_id": "",
  "created_at": "2026-09-10T09:15:00Z",
  "updated_at": "2026-09-10T09:42:11Z",
  "usage": { "prompt_tokens": 1200, "completion_tokens": 300, "total_tokens": 1500, "cost": 0.01 }
}
```

Message object:

```json
{
  "id": "…",
  "session_id": "3f2b1c8e-...",
  "seq": 4,
  "role": "assistant",
  "content": "Here is the fix...",
  "reasoning": null,
  "name": "",
  "tool_call_id": "",
  "tool_calls": [],
  "created_at": "2026-09-10T09:16:02Z"
}
```

`content` may be a string or an array of content parts. `role` is `user`,
`assistant`, `system`, or `tool`.

## Notes

- `files` are read from the server's filesystem and attached as
  `application/octet-stream` regardless of extension.
- Each `/api/run` call creates a fresh orchestrator; state lives in sessions.
- There is no WebSocket endpoint on the HTTP API. Use ACP if you need one.
- Start a session from the CLI/TUI or by passing `session_id`; the API does not have
  a "create session" endpoint.
- The background [scheduler](../features/scheduling.md) runs while the server is up,
  so due schedules fire even when no request is in flight.
