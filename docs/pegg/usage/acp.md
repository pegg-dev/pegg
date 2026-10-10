---
icon: lucide/cable
---

# ACP

Pegg implements the **Agent Client Protocol** (ACP): a JSON-RPC 2.0 protocol for
driving a coding agent from an editor or other client. The server exposes the same
orchestrator used by the CLI and TUI.

## Starting the server

=== "stdio"

    ```bash
    pegg serve --acp --stdio
    ```

    Newline-delimited JSON-RPC on stdin/stdout. This is the transport used by editor
    integrations that spawn the agent as a subprocess.

=== "HTTP"

    ```bash
    pegg serve --acp
    # acp server listening on http://127.0.0.1:8080/acp
    ```

    JSON-RPC over HTTP with Server-Sent Events for responses and notifications, plus
    optional WebSocket upgrade. The handler accepts any URL path; `/acp` is the
    convention.

Host and port come from `--host` / `--port` or the `server` config block. The ACP
handler does not apply the HTTP API's auth or CORS middleware.

!!! note

    The background [scheduler](../features/scheduling.md) runs while the ACP server is
    up, so due schedules fire independently of the connected editor session.

## Protocol basics

- Every message is a JSON-RPC 2.0 envelope. `"jsonrpc"` must be exactly `"2.0"`.
- Batch requests are not supported.
- A message with `method` and no `id` is a notification: handlers run, no response is
  written.
- A message with `id` and no `method` is a response to a server-initiated request.

### Error codes

| Code | Meaning |
|---|---|
| `-32700` | Parse error |
| `-32600` | Invalid request |
| `-32601` | Method not found |
| `-32602` | Invalid params |
| `-32603` | Internal error |
| `-32800` | Cancelled |

## Methods

| Method | Params | Result |
|---|---|---|
| `initialize` | `protocolVersion`, `clientCapabilities?`, `clientInfo?` | Capabilities and agent info |
| `session/new` | `cwd` (required), `mcpServers?`, `additionalDirectories?` | `{ "sessionId": "..." }` |
| `session/load` | `sessionId`, `cwd`, `mcpServers?`, `additionalDirectories?` | `null` (replays history as notifications) |
| `session/resume` | same as `session/load` | `{}` (no replay) |
| `session/prompt` | `sessionId`, `prompt` | `{ "stopReason": "..." }` |
| `session/delete` | `sessionId` | `null` |
| `session/close` | `sessionId` | `{}` |
| `session/list` | — | `{ "sessions": [...], "nextCursor": "" }` |
| `session/cancel` | `sessionId` | no response |
| `$/cancelRequest` | — | no response (no-op) |

Unknown methods return `-32601`.

### `initialize`

`protocolVersion` is required and must be at least `1`. The result advertises what
the agent supports:

```json
{
  "protocolVersion": 1,
  "agentCapabilities": {
    "loadSession": true,
    "promptCapabilities": { "image": true, "audio": false, "embeddedContext": true },
    "mcpCapabilities": { "http": true, "sse": false },
    "sessionCapabilities": {
      "delete": {},
      "resume": {},
      "close": {},
      "additionalDirectories": {}
    }
  },
  "agentInfo": { "name": "pegg", "title": "Pegg AI Agent", "version": "0.1.0" },
  "authMethods": []
}
```

### `session/new`

Creates a new ACP session. `cwd` is required. Pegg also creates a persisted
session (titled `ACP Session`) in its session store and links the two.

```json
{ "jsonrpc": "2.0", "id": 1, "method": "session/new", "params": { "cwd": "/home/user/project" } }
```

```json
{ "jsonrpc": "2.0", "id": 1, "result": { "sessionId": "9d1f..." } }
```

!!! warning

    The `sessionId` returned by `session/new` is the **ACP session id**, not the
    persisted Pegg session id. `session/load` and `session/resume`, however,
    expect the **Pegg** session id.

`mcpServers` and `additionalDirectories` are accepted but currently ignored.

### `session/prompt`

Only the first `text` content block is used as the agent input. While the agent
runs, the server pushes `session/update` notifications (see below). The result's
`stopReason` is one of:

| Stop reason | Meaning |
|---|---|
| `end_turn` | The agent finished normally |
| `cancelled` | The prompt was cancelled via `session/cancel` |
| `error` | The run failed |

```json
{ "jsonrpc": "2.0", "id": 2, "method": "session/prompt",
  "params": { "sessionId": "9d1f...", "prompt": [{ "type": "text", "text": "Fix the failing test" }] } }
```

```json
{ "jsonrpc": "2.0", "id": 2, "result": { "stopReason": "end_turn" } }
```

### `session/list`

Lists persisted sessions (page 1, size 50, newest first). `nextCursor` is currently
always empty.

```json
{
  "sessions": [
    { "id": "3f2b...", "title": "Fix parser bug",
      "createdAt": "2026-09-10T09:15:00Z", "updatedAt": "2026-09-10T09:42:11Z" }
  ],
  "nextCursor": ""
}
```

## Server notifications

The server pushes `session/update` notifications:

```json
{
  "jsonrpc": "2.0",
  "method": "session/update",
  "params": {
    "sessionId": "9d1f...",
    "update": { "sessionUpdate": "agent_message_chunk", "messageId": "…",
                "content": { "type": "text", "text": "Working on it" } }
  }
}
```

| `sessionUpdate` | Emitted when |
|---|---|
| `user_message_chunk` | Replaying a stored user message (`session/load`) |
| `agent_message_chunk` | Streaming assistant text |
| `tool_call` | A tool call starts (`toolCallId`, `title`, `kind: "other"`, `status: "pending"`) |
| `tool_call_update` | A tool call completes (`toolCallId`, `status: "completed"`) |
| `usage_update` | After a run with token usage (`used`, `size`, `cost`) |

When the context is compacted mid-run, an `agent_message_chunk` notification is
emitted with the text `[Context compacted (<strategy>) — <messages> messages,
<tokens> tokens]`.

The `usage_update` payload uses a fixed `size` of `200000` and a USD cost object.

## Transports

=== "stdio"

    Write one JSON-RPC message per line to stdin; read newline-delimited JSON from
    stdout. Empty lines are ignored. Closing stdin ends the server cleanly.

    ```bash
    echo '{"jsonrpc":"2.0","id":0,"method":"initialize","params":{"protocolVersion":1}}' \
      | pegg serve --acp --stdio
    ```

=== "HTTP + SSE"

    The flow is:

    1. `POST` an `initialize` request **without** a connection header. The response
       is `200` with an `Acp-Connection-Id` header and the initialize result.
    2. `GET` the same URL with `Accept: text/event-stream` and the
       `Acp-Connection-Id` header to open a long-lived event stream.
    3. `POST` further JSON-RPC requests with `Acp-Connection-Id`. The server returns
       `202 Accepted` immediately; the JSON-RPC response arrives on the SSE stream.
    4. `DELETE` with `Acp-Connection-Id` to close the connection.

    | Header | Purpose |
    |---|---|
    | `Acp-Connection-Id` | Connection identifier (returned by `initialize`) |
    | `Acp-Session-Id` | Routes responses/notifications to a per-session SSE stream |

    Status codes: `415` for a missing JSON content type, `400` for an invalid or
    missing connection, `404` for an unknown connection, `406` when the stream
    request lacks `Accept: text/event-stream`, `405` for unsupported methods.

    ```bash
    # 1. initialize
    curl -i http://127.0.0.1:8080/acp \
      -H 'Content-Type: application/json' \
      -d '{"jsonrpc":"2.0","id":0,"method":"initialize","params":{"protocolVersion":1,"clientInfo":{"name":"demo","version":"1.0"}}}'

    # 2. open the event stream (long-lived)
    curl -N http://127.0.0.1:8080/acp \
      -H 'Accept: text/event-stream' \
      -H 'Acp-Connection-Id: <uuid>'
    ```

=== "WebSocket"

    A `GET` with `Upgrade: websocket` establishes a WebSocket connection; JSON-RPC
    responses are sent as text frames on the same socket.

    !!! note

        In pure WebSocket mode, `session/update` notifications are only delivered if
        a matching SSE stream is also open. Use HTTP+SSE for full notification
        support.

## Complete example (stdio)

```
→ {"jsonrpc":"2.0","id":0,"method":"initialize","params":{"protocolVersion":1}}
← {"jsonrpc":"2.0","id":0,"result":{"protocolVersion":1,"agentCapabilities":{...},"agentInfo":{...}}}
→ {"jsonrpc":"2.0","id":1,"method":"session/new","params":{"cwd":"/home/user/project"}}
← {"jsonrpc":"2.0","id":1,"result":{"sessionId":"9d1f..."}}
→ {"jsonrpc":"2.0","id":2,"method":"session/prompt","params":{"sessionId":"9d1f...","prompt":[{"type":"text","text":"List the Go files"}]}}
← {"jsonrpc":"2.0","method":"session/update","params":{"sessionId":"9d1f...","update":{"sessionUpdate":"agent_message_chunk","content":{"type":"text","text":"Here are"}}}}
← {"jsonrpc":"2.0","method":"session/update","params":{"sessionId":"9d1f...","update":{"sessionUpdate":"tool_call","toolCallId":"…","title":"glob","kind":"other","status":"pending"}}}
← {"jsonrpc":"2.0","method":"session/update","params":{"sessionId":"9d1f...","update":{"sessionUpdate":"tool_call_update","toolCallId":"…","status":"completed"}}}
← {"jsonrpc":"2.0","method":"session/update","params":{"sessionId":"9d1f...","update":{"sessionUpdate":"usage_update","used":1540,"size":200000,"cost":{"amount":0.004,"currency":"USD"}}}}
← {"jsonrpc":"2.0","id":2,"result":{"stopReason":"end_turn"}}
```

## Cancellation

Send `session/cancel` with the session id to abort a running prompt. The running
agent's context is cancelled and the in-flight `session/prompt` resolves with
`stopReason: "cancelled"`. No response is written for the cancel message itself.
`$/cancelRequest` is accepted but does nothing.

## Outbound requests

The server can also call methods on the client (`fs/read_text_file`,
`fs/write_text_file`, `session/request_permission`). The plumbing is implemented, but
current tools do not invoke it, so clients do not need to handle these methods yet.
