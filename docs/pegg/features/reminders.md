---
icon: lucide/bell
---

# Reminders

Reminders are **system messages injected directly into an agent's LLM context** while
it is running. They let built-in systems (and future plugins) surface important
state to the agent mid-turn — without the agent having to poll or check anything.

Each reminder is formatted as an XML block:

```xml
<system-reminders>
<system-reminder tag="usage">
  Token usage: 25.0K/100.0K; 75.0K remaining
</system-reminder>
</system-reminders>
```

Reminders are delivered as part of the **next LLM request** of the target agent.
Multiple reminders arriving between two iterations are batched into a single
`<system-reminders>` block.

Reminders work for **every agent in the system** — the orchestrator, subagents,
and helper agents alike. They are registered globally at startup and driven by
the shared event bus, so no per-agent configuration is required.

## Usage reminder

The usage reminder tracks two things per agent while it runs:

| Trigger | Cadence |
|---|---|
| **Agent messages** | Every **20th** streamed agent message |
| **Context window** | Every **20%** of the model's real context window consumed (20%, 40%, 60%, 80%) |

The context window comes from the actual model metadata (`max input tokens`), so
the percentages reflect the true capacity of the model in use, not a fixed number.

The reminder reports the current token balance:

```
Token usage: 25.0K/100.0K; 75.0K remaining
```

`used` is the tokens consumed so far, `total` is the model's context window, and
`remaining` is the difference. The message-count reminder reports the same summary.

**Example** — after the 20th message of a run:

```xml
<system-reminders>
<system-reminder tag="usage">
  Token usage: 40.0K/200.0K; 160.0K remaining
</system-reminder>
</system-reminders>
```

This helps the agent keep its working context in mind — for instance, wrapping up
or delegating remaining work before the window fills.

## TodoWrite reminder

The todowrite reminder nudges the agent when it has been doing work for a while
without maintaining its todo list:

| Trigger | Cadence |
|---|---|
| **Stream events** | Every **30** stream events without a `todowrite` tool call |

Stream events are the agent's activity steps — tool calls and tool results.
Thinking and content generation (streamed tokens) do **not** count. Using the
`todowrite` tool resets the counter.

The reminder text:

```xml
<system-reminders>
<system-reminder tag="todowrite">
  The TodoWrite tool hasn't been used recently. If you're working on tasks that
  would benefit from tracking progress, consider using the TodoWrite tool to track
  progress. Also consider cleaning up the todo list if it has become stale and no
  longer matches what you are working on. Only use it if it's relevant to the
  current work. This is just a gentle reminder - ignore if not applicable.
</system-reminder>
</system-reminders>
```

The reminder repeats every 30 stream events as long as the agent keeps working
without calling `todowrite`.

## Task tools reminder

The task tools reminder nudges the agent when it has been working for a while
without delegating or checking on subagent tasks:

| Trigger | Cadence |
|---|---|
| **Stream events** | Every **30** stream events without a `task` or `taskstatus` tool call |

Stream events are the agent's activity steps — tool calls and tool results.
Thinking and content generation (streamed tokens) do **not** count. Using the
`task` or `taskstatus` tool resets the counter.

The reminder text:

```xml
<system-reminders>
<system-reminder tag="task_tools">
  The task tools haven't been used recently. If you're working on tasks that
  would benefit from tracking progress, consider using task to add new tasks and
  taskstatus to check on their status. Also consider cleaning up the task list
  if it has become stale. Only use these if relevant to the current work. This
  is just a gentle reminder - ignore if not applicable.
</system-reminder>
</system-reminders>
```

The reminder repeats every 30 stream events as long as the agent keeps working
without using the task tools.

## Built-in reminders

| Tag | Reminder | When |
|---|---|---|
| `usage` | Token usage status | Every 20 agent messages and every 20% of the context window |
| `todowrite` | TodoWrite nudge | Every 30 stream events without a `todowrite` call |
| `task_tools` | Task tools nudge | Every 30 stream events without a `task`/`taskstatus` call |
| `subagent` | Background subagent result | When a background subagent finishes or fails (see [Subagents](subagents.md)) |
| `background_subagent` | Standing note | While any background subagent is still running |

## Adding new reminders

Reminders live in `internal/builtin/reminders/<name>/`. A reminder is a component
that subscribes to [agent events](../../sdk/events-errors.md) on the shared bus,
tracks per-agent state, and queues a notification on the target agent:

- Subscribe to the agent topics you need (`agent.message`, `agent.usage`,
  `agent.finished`, `agent.error`, ...).
- Use the agent ID from the event payload to track state and to queue the
  notification with `QueueNotification`.
- Build the reminder content with the `reminder.New(tag, content)` helper; it is
  formatted as a `<system-reminder>` block automatically.

Register the reminder at startup in `internal/builtin/create.go`, next to the
usage reminder.