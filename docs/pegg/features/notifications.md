---
icon: lucide/bell-ring
---

# Notifications

Desktop notifications surface the moments that need you — permission prompts,
agent completions, agent failures — as OS notifications, so you can step away
from the terminal and still know when Pegg is waiting.

Notifications are **event-driven**: a notifier listens on the shared event bus
and forwards relevant events to one or more configured drivers. Delivery is
**focus-gated**: a notification is only sent while the Pegg window is *not*
focused (in background).

## Focus detection

The TUI enables terminal focus reporting (xterm-1004) and republishes
focus/blur transitions on the event bus. The notifier starts in the *focused*
state, so nothing is delivered until the terminal reports that the window lost
focus.

Terminals without xterm-1004 focus reporting never report a blur, so the app
stays "focused" and no desktop notifications are sent. Headless CLI mode never
notifies either.

## What triggers a notification

| Event | Title | Message |
|---|---|---|
| Permission prompt (from the [permission middleware](permissions.md)) | `Permission required: <agent>` | The permission question (single line, truncated) |
| Any other interactive ask (e.g. `askuserquestion`) | `Input required: <agent>` | The question text |
| Agent finished | `Agent finished: <name>` | A snippet of the agent output; `Task completed` when empty |
| Agent error | `Agent failed: <name>` | The error message |

Subagent activity is filtered: only top-level orchestrator agents notify.
Completion and error events from [subagents](subagents.md) are ignored.

Messages are collapsed to a single line and truncated to 200 characters.

## Drivers

Drivers are configured as an **array** — every configured driver receives each
notification. The only built-in driver is `os`:

| Driver | Behavior |
|---|---|
| `os` | Desktop notification via [beeep](https://github.com/gen2brain/beeep): D-Bus (`org.freedesktop.Notifications`) with `notify-send` fallback on Linux, `terminal-notifier`/`osascript` on macOS, WinRT/PowerShell on Windows |

- Duplicate names in the array are deduplicated; empty strings are skipped.
- An unknown driver name fails startup with an error naming the driver.
- A failing driver is logged and never blocks the others.

## Configuration

```json
{
  "notification": {
    "drivers": ["os"],
    "enabled": true
  }
}
```

| Key | Type | Default | Description |
|---|---|---|---|
| `drivers` | string[] | `["os"]` | Drivers that receive every notification |
| `enabled` | bool | `true` | Master toggle; when `false` no driver is built at all |

See [Config](../config.md#notification) for the reference entry.

## Behavior notes

- Notification failures (for example, no notification daemon on Linux) are
  logged as warnings and never abort the app.
- Delivery runs on the event bus asynchronously, so a slow D-Bus call never
  stalls an agent run.
- Permission notifications fire when the ask event is published — that is, when
  the [permission middleware](permissions.md) is actually waiting for your
  decision.

## Extending

The notification package follows the same driver-register-module pattern as the
logger and cache: implement the `Driver` interface (`Notify(Notification) error`
and `Close() error`), register it with `RegisterDriver` in an `init()`
function, and list its name in `drivers`.
