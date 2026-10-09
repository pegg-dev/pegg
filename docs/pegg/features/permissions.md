---
icon: lucide/shield-check
---

# Permissions

The permission middleware decides which [tool](../configurations/tools.md) calls may
run. It combines a per-tool mode, a filesystem sandbox, and — depending on the mode
— an interactive prompt or a judge LLM.

## Modes

| Mode | Behavior |
|---|---|
| `allow` | Always runs. The filesystem sandbox is lifted for the call |
| `semi-ask` | Runs first; only asks the user if the sandbox denies it (or the tool is gated by policy) |
| `ask` | Always asks the user before running |
| `semi-judge` | Runs first; only sends to the judge LLM if the sandbox denies it |
| `judge` | Always sends to the judge LLM before running |

The effective mode for a tool is `permission.rules[<tool>]`, falling back to
`permission.default`, falling back to `semi-ask`. An invalid configured mode parses
to `ask`.

## Built-in defaults

| Tool | Default mode |
|---|---|
| `read`, `write`, `edit`, `delete`, `list`, `glob`, `grep` | `semi-ask` |
| `bash` | `semi-judge` |
| `todoread`, `todowrite`, `task`, `taskstatus` | `allow` |
| `webfetch`, `websearch`, `loadskill` | `allow` |
| `enterplanmode`, `exitplanmode` | `ask` |
| `askuserquestion` | `allow` (never gated) |

MCP tools default to `permission.default`. Override any tool with the
`permission.rules` map:

```json
{
  "permission": {
    "default": "semi-ask",
    "rules": { "bash": "ask", "write": "allow" },
    "judge_provider": "openrouter",
    "judge_model": "typesafe/jev-1.13",
    "judge_threshold": 0.8
  }
}
```

## The filesystem sandbox

File tools operate inside a virtual filesystem rooted at the workspace. Escaping the
root — absolute paths outside the workspace, `..` traversal, symlink escapes — fails
with an out-of-bounds error. `.gitignore` and `.peggignore` rules hide ignored
files.

The project's `.pegg/` directory is readable by all agents (sessions, todos,
plan and rule files), but only `.pegg/plans/` and `.pegg/rules/` are
writable; other `.pegg` paths return an ignored error on write. The planner
agent is additionally write-scoped to `.pegg/plans/` and the explorer is
read-only.

For `semi-ask` / `semi-judge`, the tool runs first: if it succeeds, no prompt or
judge is involved. Only an out-of-bounds result triggers the gate. After the user or
judge approves a path, the call is re-run with that specific path permitted. An
`allow`-mode call runs with the sandbox fully lifted.

The planner agent is write-scoped to `.pegg/plans/`; the explorer is read-only.

## Bash policy

`bash` uses `semi-judge`, but simple, safe commands never reach the judge:

- The command starts with a **bare** whitelisted binary — `go`, `npm`, `ls`, `pwd`,
  `node`, or `git` — **and**
- Contains no shell metacharacters (`;`, `|`, `&`, `>`, `<`, `$`, whitespace
  control chars, `&&`, `||`).

Path-qualified commands (`/bin/ls`, `./tool`) are never allowed. Everything else is
gated. Interactive shells or destructive commands therefore trigger a judge or
prompt, while `git status` and `npm run build` run directly.

## The prompt flow

When a call needs approval, the user is asked (in the TUI and CLI):

```
Allow the "bash" tool call?
  Allow / Allow All / Reject
```

| Choice | Effect |
|---|---|
| **Allow** | Runs this call once |
| **Allow All** | Runs this call and remembers it; same arguments never prompt again |
| **Reject** | Denies the call; an optional reason is collected and remembered |

Dismissing the prompt denies the call. The `askuserquestion` tool is exempt from all gating.

When the Pegg window is in the background, a permission prompt also raises a
desktop notification — see [Notifications](notifications.md).

## The judge flow

In `judge` / `semi-judge` mode a judge reviews the tool call and returns an
allow/deny verdict. Two judge engines are available:

- **Decision model** — a fast, structured decision model such as
  [JEV](../providers-and-models.md#decision-models) (TypeSafe, served via
  OpenRouter). It receives the tool call and the full conversation history and
  answers a single yes/no question (`safe_to_run`). The call is allowed when the
  yes-probability is at least `judge_threshold`.
- **Judge LLM** — a dedicated judge agent that reviews the tool call against the
  conversation history and returns a structured `{allow, reason}` verdict. If the
  tool was denied, its error message is included; tool outputs are not.

Which engine runs is resolved in this order:

1. **Explicit config** — `judge_provider` is set:
   - a decision-capable provider (currently `openrouter`) → decision model, using
     `judge_model` if given, otherwise the default (`typesafe/jev-1.13`);
   - any other provider → judge LLM with `judge_model`.
2. **Default decision** — no `judge_provider`: if any configured provider with an
   API key supports decisions (an OpenRouter key is enough), the decision model is
   used automatically.
3. **Fallback** — otherwise the judge LLM runs on the preferred (agent's) model.

If the decision model errors or returns no answer, Pegg falls back to the judge
LLM. Configure with `judge_provider` / `judge_model` / `judge_threshold`
([config reference](../config.md#permission)).

## Remembered decisions

Allow and reject decisions are stored in `~/.pegg/permissions.json`, keyed by a
hash of the canonicalized tool arguments.

- A previously allowed call runs immediately with the sandbox permitted.
- A previously rejected call fails immediately with
  `permission denied for tool "<name>": <reason>` (marked *previously rejected*).

Reset all remembered decisions:

```bash
rm ~/.pegg/permissions.json
```

## Denied calls

A denied call fails the tool execution with `permission denied for tool
"<name>": <reason>`. The agent sees the denial as a tool error and should adjust
its approach.

## Interrupting a run

Pressing ++esc++ twice within two seconds in the TUI cancels the running agent and
all subagents — including any in-flight permission prompt. See
[TUI](../usage/tui.md#interrupting-the-agent).