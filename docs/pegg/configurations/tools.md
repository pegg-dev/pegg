---
icon: lucide/wrench
---

# Tools

Tools are the functions the model can call. Pegg ships with 20 built-in tools and
registers additional tools from connected [MCP servers](mcp.md). Custom tools can be
added with the [SDK](../../sdk/extending.md).

## File tools

All file tools operate on the sandboxed workspace and respect `.gitignore` and
`.peggignore` rules. Paths are virtual: relative to the workspace root, with `/` as
the separator. `~` expands to the home directory. Attempting to escape the root
returns a permission error, which the [permission middleware](../features/permissions.md)
can gate.

### `read`

Read a file with line numbers plus metadata (path, SHA-256 hash, size, line count).

| Parameter | Type | Required | Description |
|---|---|---|---|
| `filePath` | string | yes | Virtual path to the file |
| `offset` | integer | no | 1-indexed starting line for partial reads |
| `limit` | integer | no | Maximum number of lines to return |

Reading records a snapshot hash. LSP diagnostics are appended when a language server
is available.

### `write`

Create or overwrite a file. Parent directories are created automatically and writes
are atomic (temp file + rename), preserving existing permissions.

| Parameter | Type | Required | Description |
|---|---|---|---|
| `filePath` | string | yes | Virtual path |
| `content` | string | yes | Full file content |

### `edit`

Apply a text replacement. The file must have been read (or written) first; a hash
mismatch after an external change returns an error and requires a fresh read.

| Parameter | Type | Required | Description |
|---|---|---|---|
| `filePath` | string | yes | Virtual path |
| `oldString` | string | yes | Exact text to find |
| `newString` | string | yes | Replacement text |
| `replaceAll` | boolean | no | Replace all occurrences instead of the first |

### `delete`

Delete a file.

| Parameter | Type | Required | Description |
|---|---|---|---|
| `filePath` | string | yes | Virtual path |

### `list`

List a directory with name, path, size, and type.

| Parameter | Type | Required | Description |
|---|---|---|---|
| `path` | string | no | Directory to list; defaults to the workspace root |

### `glob`

Find files by pattern. Supports `**`, `*`, and `?`. Results are sorted newest
modification first.

| Parameter | Type | Required | Description |
|---|---|---|---|
| `pattern` | string | yes | Glob pattern |
| `path` | string | no | Directory to search from |

### `grep`

Search file contents with Go regular expressions. Binary files are skipped.

| Parameter | Type | Required | Description |
|---|---|---|---|
| `pattern` | string | yes | Regular expression |
| `path` | string | no | Directory to search from |
| `include` | string[] | no | File globs, for example `["*.go", "*.{ts,tsx}"]` |
| `mode` | string | no | `content` (default), `files_with_matches`, or `count` |
| `headLimit` | integer | no | Maximum results; `0` means unlimited |

## Shell

### `bash`

Execute a shell command with `sh -c`. stdout and stderr are captured and the exit
code is included in the output.

| Parameter | Type | Required | Description |
|---|---|---|---|
| `command` | string | yes | Shell command |
| `workdir` | string | no | Directory relative to the workspace root |
| `timeout` | integer | no | Timeout in seconds; defaults to 40 |
| `description` | string | no | Short explanation of the command |

- The process runs in its own process group and is killed (SIGKILL) on timeout.
- Non-zero exit codes are reported in the output, not as tool errors.
- There is no upper bound on `timeout`; choose a value appropriate for the command.

## Web

### `websearch`

Search the web (DuckDuckGo HTML endpoint) and return titles, URLs, and snippets.

| Parameter | Type | Required | Description |
|---|---|---|---|
| `query` | string | yes | Search query |
| `maxResults` | integer | no | 1–20; defaults to 10 |

### `webfetch`

Fetch a URL. HTML is converted to Markdown by default.

| Parameter | Type | Required | Description |
|---|---|---|---|
| `url` | string | yes | URL including `http://` or `https://` |
| `raw` | boolean | no | Return raw HTML instead of Markdown |

Responses are capped at 10 MiB; binary content is noted but not returned.

## Interaction

### `askuserquestion`

Ask the user one or more questions and block until answers are submitted.

| Parameter | Type | Required | Description |
|---|---|---|---|
| `questions[]` | array | yes | Questions to ask |
| `questions[].id` | string | yes | Identifier used to map answers back |
| `questions[].question` | string | yes | Question text |
| `questions[].type` | string | yes | `text` or `select` |
| `questions[].options` | string[] | for `select` | Options to choose from |
| `questions[].required` | boolean | no | Require an answer |

Returns `{"answers": {"id": "answer", ...}}`. In the TUI, questions open a modal
that supports free text and option lists (with a *Custom answer* entry).

## Todos

Todos persist in `.pegg/todos.json` in the project directory. Statuses are
`pending`, `in_progress`, `completed`, and `cancelled`; priorities are `high`,
`medium`, and `low`.

### `todoread`

| Parameter | Type | Required | Description |
|---|---|---|---|
| `status` | string | no | Filter by status |
| `priority` | string | no | Filter by priority |

### `todowrite`

Create, update, delete, or change the status of a todo. Omit `id` to create a todo
with an auto-generated id (`todo-1`, `todo-2`, ...). Providing an existing `id`
performs a partial update. Dependencies are stored and displayed but not enforced.

| Parameter | Type | Required | Description |
|---|---|---|---|
| `id` | string | no | Existing id to update/delete, or a new custom id |
| `title` | string | for new | Todo title |
| `description` | string | no | Details |
| `status` | string | no | `pending`, `in_progress`, `completed`, `cancelled` |
| `priority` | string | no | `high`, `medium`, `low` |
| `dependsOn` | string[] | no | Todo ids this task depends on |
| `action` | string | no | Set to `delete` to remove the todo |

## Plan mode

### `enterplanmode`

Enter plan mode: attaches a standing **plan-mode reminder** that is injected into
every LLM request until `exitplanmode` is called. While active the agent is
read-only and must only research and plan. The tool call itself requires user
approval (`ask` by default).

### `exitplanmode`

Leave plan mode: removes the standing plan-mode reminder. Requires user approval
(`ask` by default).

## Skills

### `loadskill`

Load a skill by name. Returns the skill's `<skill:...>` block as a tool result, or —
for skills with `context: fork` — spawns a background subagent from a forked session
and returns a confirmation. See [Skills](skills.md). The same effect is triggered
automatically when a message contains `/skill-name`.

| Parameter | Type | Required | Description |
|---|---|---|---|
| `name` | string | yes | Skill name |
| `arguments` | object | no | Values substituted into `$arg_name` placeholders |

## Subagents

See [Subagents](../features/subagents.md) for the full workflow.

| Tool | Parameters | Description |
|---|---|---|
| `task` | `name`, `subagent_type`, `prompt`, `task_id[]` (optional), `background` (optional) | Spawn a subagent, or resume a finished one by reusing the same `name` |
| `taskstatus` | `agent_names[]` (optional) | Report status: `pending`, `running`, `completed`, `failed`, `interrupted` |

Registered agent types for `task`: `orchestrator`, `explorer`, `planner`, and
`developer`.

## Memory

The memory tools query the [markdown-based memory system](../features/memory.md).
They are registered when memory is enabled and available to the orchestrator,
planner, and developer agents.

### `mem-search`

Keyword search over the project's stored memory entries (observations, summaries,
and the curated files). Results are scored by title/body term matches and filtered
by the configured relevance gate (`auto`, `decision`, `llm`, `score`, or `off`),
then returned as XML. Use it *before* re-investigating something that may already
have been solved or learned in a past session.

| Parameter | Type | Required | Description |
|---|---|---|---|
| `query` | string | yes | Search query: keywords, a file path, an error message, or a natural-language question |
| `limit` | int | no | Maximum number of results (1–10, default from `memory.max_results`) |

### `mem-read`

Read the full markdown entry of a memory id (e.g. `obs-9f3a1b` or `sum-2c4d5e`).
IDs appear in `mem-search` results, in the memory panel, and in the memory index.

| Parameter | Type | Required | Description |
|---|---|---|---|
| `id` | string | yes | Memory entry id |

## Tool availability per agent

| Agent | Tools |
|---|---|
| `orchestrator` | All file tools, `askuserquestion`, `bash`, `task`, `taskstatus`, `todoread`, `todowrite`, `webfetch`, `websearch`, `loadskill`, `enterplanmode`, `exitplanmode`, `mem-search`, `mem-read` |
| `planner` | File tools write-scoped to `.pegg/plans`, `bash`, `webfetch`, `websearch`, `todoread`, `todowrite`, `mem-search`, `mem-read` |
| `developer` | All file tools, `bash`, `webfetch`, `websearch`, `todoread`, `todowrite`, `mem-search`, `mem-read` |
| `explorer` | `glob`, `grep`, `list`, `read`, `bash`, `webfetch`, `websearch` |

## Permissions

Tools are gated by the [permission middleware](../features/permissions.md). Effective
built-in defaults:

| Tool | Default mode |
|---|---|
| `read`, `write`, `edit`, `delete`, `list`, `glob`, `grep` | `semi-ask` |
| `bash` | `semi-judge` (whitelisted bare commands run directly) |
| `todoread`, `todowrite` | `allow` |
| `task`, `taskstatus` | `allow` |
| `webfetch`, `websearch`, `loadskill` | `allow` |
| `mem-search`, `mem-read` | `semi-ask` (falls back to `permission.default`) |
| `enterplanmode`, `exitplanmode` | `ask` |
| `askuserquestion` | `allow` (never gated) |

Override any tool with the `permission.rules` map in [Config](../config.md#permission).
MCP tools default to the configured `permission.default`.

## Tool results and truncation

Tools return full output to the model. Truncation happens only in the UI:

- TUI tool output cards show 15 lines collapsed; bash output 30 lines; diffs 30
  rows.
- The CLI truncates tool results to 500 characters.

## Registry

Tools live in a global registry and are resolved per agent by name at run time.
Names must be non-empty and unique; the convention is kebab-case. MCP tools are
registered as `<server>__<tool>`. See [MCP](mcp.md).
