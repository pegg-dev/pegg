---
icon: lucide/shell
---

# Adding Context

Pegg has several ways to give the agent more context than a single prompt: file
attachments, `@` mentions, project instructions, rules, and skills.

## Attaching files

### In the TUI

- **Paste a path** — pasting a single line that is an existing file path attaches
  that file. In a multi-line paste, every line that is a file path is attached.
- **Attachment bar** — attachments appear above the input as cards with type icons
  (image, audio, file). Focus it with ++tab++, navigate with ++left++/++right++, and
  remove with ++backspace++/++delete++.
- Attachments are sent with your next message.

### In the CLI

```bash
pegg run --file screenshot.png --file notes.md "What's wrong with my UI?"
```

`--file` is repeatable. The MIME type is inferred from the extension:

| Extension | Attachment |
|---|---|
| `.png`, `.jpg`, `.jpeg`, `.gif`, `.webp` | image |
| `.mp3`, `.wav` | audio |
| `.pdf`, `.txt`, `.md`, `.json`, `.csv`, `.xml`, `.zip`, `.tar`, `.gz` | file (typed) |
| anything else | `application/octet-stream` |

### Validation

Image and audio attachments are checked against the active model's input modalities.
If the model does not advertise that modality, the attachment is rejected with
*"Model does not support image/audio attachments"* instead of failing at the API.
File attachments are accepted by all models.

## `@` mentions

Typing `@` in the input opens the mention picker. It offers:

- **Agents** — subagent types you can address in your message.
- **Files and folders** — discovered by scanning the workspace.
- **Attachments** already in the attachment bar.

Mentions are inserted as atomic chips and sent to the model as literal `@name`
text. Unlike skills, mentions are **not** expanded by Pegg — the model decides how
to use the reference (for example `@internal/server` prompts it to look at that
directory).

## Long text

Pasting text longer than 100 characters inserts it as a collapsible **long text**
chip showing a short preview. The full text is sent with the message. Shorter
pastes are inserted character by character.

## Persistent context

Some context is always available to the agent, no matter how you phrase the prompt:

| Source | When it applies |
|---|---|
| [`AGENTS.md`](../configurations/rules.md#rules-vs-agentsmd) | Read from the current working directory, injected as **Project Instructions** |
| [`rules/*.md`](../configurations/rules.md) | Global (`~/.pegg/rules/`) and project (`.pegg/rules/`) rule files appended to the system prompt |
| [Skills](../configurations/skills.md) | Every skill is listed; including `/name` in a message injects its instructions |
| Git status | The current branch and dirty/clean state are included in the environment block |

## Injecting skills into context

Include `/skill-name` in any message or in a subagent task message to load the
skill's instructions. In the TUI, type `/` to pick a skill. See
[Skills](../configurations/skills.md#invoking-a-skill) and
[Commands](commands.md).

## Context and sessions

History accumulates across turns and is persisted per
[session](sessions.md). When you resume a session, its prior messages are replayed
into the model context, so context added earlier is still available.