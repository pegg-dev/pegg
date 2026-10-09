---
icon: lucide/app-window
---

# TUI

The terminal UI is the default way to use Pegg. Launch it with `pegg` on a
terminal, or explicitly with `pegg tui`.

## Layout

```
┌──────────────────────────────────────────────────────────┐
│                                                          │
│                    chat transcript                       │
│        (streaming markdown, tool cards, diffs)           │
│                                                          │
├──────────────────────────────────────────────────────────┤
│  attachments                                             │
├──────────────────────────────────────────────────────────┤
│  > input editor                                          │
├──────────────────────────────────────────────────────────┤
│  ● gpt-4o/openai · 12.4K (6%) · $0.0021 · Ctrl+P         │
└──────────────────────────────────────────────────────────┘
```

- **Chat transcript** — user cards, assistant markdown, thinking blocks, tool cards,
  and subagent cards. With an empty chat, an animated Pegg logo is shown.
- **Attachment bar** — appears only when files are attached.
- **Input editor** — up to 6 rows tall, with inline chips for skills, mentions, and
  long pasted text.
- **Status bar** — a running indicator (`●`), the active model as
  `name/provider`, reasoning effort when set, context usage, session cost, and a
  `Ctrl+P` hint.
- **Hint line** — transient errors and the `Press Esc to interrupt` hint.

## Global keys

| Key | Action |
|---|---|
| ++ctrl+c++ / ++ctrl+q++ | Quit |
| ++ctrl+t++ | Cycle to the next theme (saved to config) |
| ++ctrl+p++ | Open Settings |
| ++ctrl+shift+c++ | Copy the selected chat text (or input selection) |
| ++esc++ | Interrupt (see below) |
| ++tab++ | Toggle focus between the input and attachments (when attachments exist) |
| Mouse wheel | Scroll the chat 3 lines |
| Mouse drag | Select chat text (release to keep the selection) |
| Mouse click | Activate the item under the cursor |

### Interrupting the agent

While a run is active, pressing ++esc++ once shows *Press Esc to interrupt*.
Pressing ++esc++ again within **2 seconds** cancels the run and all subagents.
Pressing it once and waiting lets the run continue.

When the attachment bar has focus, ++esc++ returns focus to the input. While
viewing a subagent transcript, ++esc++ goes back to the main chat.

## Input editor

| Key | Action |
|---|---|
| ++enter++ | Submit |
| ++shift+enter++ | Insert newline |
| ++tab++ | Insert a tab character |
| ++left++ / ++right++ / ++up++ / ++down++ | Move the cursor (word-wrap aware) |
| ++home++ / ++end++ | Start / end of line |
| ++ctrl+left++ / ++ctrl+right++ | Word left / right |
| ++alt+left++ / ++alt+right++ | Word left / right |
| ++ctrl+a++ | Select all |
| ++ctrl+e++ | End of line |
| ++ctrl+k++ / ++ctrl+u++ | Delete to end / start |
| ++ctrl+w++ / ++alt+backspace++ | Delete word |
| ++ctrl+d++ | Delete character forward |
| ++ctrl+y++ | Paste from the kill ring |
| ++ctrl+z++ / ++ctrl+shift+z++ | Undo / redo (200 steps) |
| ++ctrl+shift+k++ | Delete line |
| ++ctrl+shift+up++ / ++ctrl+shift+down++ | Move line up / down |
| ++shift+arrows++ | Extend selection |
| ++up++ on the first row | Recall the previous prompt |
| ++down++ on the last row | Recall the next prompt, then the draft |

Skills (`/name`), mentions (`@name`), and long pasted text are atomic **chips**:
cursor movement and deletion treat each chip as a single unit.

### Prompt history

The last 200 submitted prompts are kept for recall. ++up++ loads the previous one
when the cursor is already on the first row, so arrow keys still move inside a
multi-line draft; ++down++ walks back toward the newest entry and finally
restores the draft you were typing.

History is per project and survives restarts: it is stored in
`.vesvai/prompt-history.json` and loaded at startup. Switching to another
session appends that session's own prompts to the end of the list, so the ones
you just saw come up first. Delete the file to clear the history.

## Skills and mentions

Type `/` at the start of a word to open the **skill picker**, or `@` to open the
**mention picker**. Both are fuzzy-filtered as you type.

| Key | Action |
|---|---|
| ++up++ / ++down++ | Move the selection |
| ++enter++ / ++tab++ | Accept the selected item |
| ++esc++ | Dismiss |
| ++space++ | Dismiss and keep typing |

- `/skill` inserts a skill chip that is expanded to the skill's instructions before
  the message reaches the model. See [Skills](../configurations/skills.md).
- `@name` inserts a mention chip for an agent, file, folder, or attachment. Mentions
  are sent to the model as literal `@name` text for it to interpret. See
  [Adding Context](../features/adding-context.md).

## Pasting and attachments

There is no system-clipboard integration; pasting uses the terminal's bracketed
paste. When you paste:

- A single line that is an existing file path is attached as a file.
- In a multi-line paste, every line that is an existing file path is attached.
- Text longer than 100 characters is inserted as a collapsible **long text** chip.

Attachments appear in the attachment bar with type icons (image, audio, file) and
pagination dots. Focus the bar with ++tab++, navigate with ++left++/++right++, and
remove with ++backspace++ or ++delete++.

Image and audio attachments are rejected if the active model's metadata does not
list that input modality.

## Chat items

| Item | Rendering |
|---|---|
| User message | Boxed card, with attachment chips |
| Assistant message | Rendered markdown (headings, lists, quotes, inline styles) |
| Thinking | Collapsible; animated while active, raw reasoning when expanded |
| Tool call | Card with name, enriched target (`read:path`, `bash:cmd`, ...), duration, expandable output |
| Subagent | Card with status, live activity line, output preview, usage, and a **History** action |
| Error | Bold red `✖ error: ...` |

### Selecting and copying text

Click and drag over any chat text to select it; release to keep the selection.
Press ++ctrl+shift+c++ to copy the selection to the system clipboard (via the
terminal's OSC 52 support). Card borders and padding are stripped from the copied
text. If the input editor has a selection, ++ctrl+shift+c++ copies that instead.

### User message menu

Click a user message (without dragging) to open a small menu. Move the mouse
over an option (or use the arrow keys) to highlight it.

| Option | Action |
|---|---|
| Copy text | Copy the message text to the clipboard |
| Revert message | Delete this message and every message after it, and put the message back into the input |
| Fork message | Create a new session containing the messages *before* this one, switch to it, and put the message back into the input |

A forked session is titled `<original title> - fork`. Revert keeps the same
session; a snapshot of the removed messages is saved so it can be undone.
Attachments on the message are restored into the attachment bar along with the
text.

### Specialized tool cards

- **BASH** — command header, syntax-highlighted output (30 lines collapsed),
  `exit code` footer.
- **DIFF** — unified diff for `edit` calls with added/removed highlighting and a
  `+N -M` summary.
- **WRITE** — syntax-highlighted file content for `write` calls.
- **TODO** — parsed todo list with status icons and an `N / M completed` footer.
- **ASK** — question/answer pairs collected from the user.

Fenced code in assistant messages is syntax-highlighted for Go, TypeScript,
JavaScript, Python, Rust, Bash, C/C++, Ruby, PHP, Swift, Kotlin, Scala, Elixir,
Haskell, CSS, HTML, JSON, YAML, Markdown, SQL, R, Lua, and Dart.

### Tool card keys

| Key | Action |
|---|---|
| ++enter++ / ++space++ | Expand or collapse the selected item |
| `]` / `[` | Next / previous item |
| ++up++ / ++down++ | Scroll one line |
| ++pgup++ / ++pgdn++ | Scroll one page |
| ++home++ / ++end++ | Jump to top / bottom |

## Subagents

Running and finished subagents appear as cards in the main transcript. Select a
running subagent and press ++enter++, or click **History**, to open its live
transcript. Press ++esc++ or click the back header to return. See
[Subagents](../features/subagents.md).

## Settings

Open with ++ctrl+p++. The overlay has nine tabs; switch with ++left++/++right++ when
the tab bar is focused. Press ++down++ to enter the tab content, ++up++ to return to
the tab bar.

The whole settings overlay is mouse-driven: click a tab to switch to it, click a
row to select and activate it, hover to highlight the row under the pointer, use
the wheel to scroll lists or adjust numeric rows, and click outside the dialog to
close it. Sub-menus (lists and text fields) respond to clicks too — click an item
to choose it, click the field to place the cursor, or click outside to go back.

| Tab | Contents |
|---|---|
| **General** | Provider (add or reconfigure), Model (searchable list), Theme, Reasoning effort |
| **Session** | Load/New/Delete session, Change title, Compaction settings |
| **MCP** | Connected MCP servers and their tools |
| **Skills** | Loaded skills with descriptions |
| **Rules** | Global and project rule files |
| **Plugins** | Installed plugins with enable/disable toggle |
| **Permissions** | Preset selector and per-tool permission modes |
| **Memory** | Memory system: observer model, relevance gating, context budget (see below) |
| **System** | App name, version, OS, architecture, and manual update check |

- **Provider** — configure an existing provider again or add a new one by pasting an
  API key into a masked field.
- **Model** — type to filter; the active model is marked with `●`. When the
  [smart router](../providers-and-models.md#smart-router) is enabled, a **Smart
  Router** entry appears at the top of the list: it auto-selects the best model
  for each task and agent at run time.
- **Reasoning** — available only for models that advertise reasoning options.
- **Session → Load** — sessions from the current directory, newest first. Loading
  restores the last 50 messages; scrolling to the top loads older messages in
  batches of 50.
- **Session → Delete** — asks for confirmation before permanently removing the
  session.
- **Session → Change title** — edit the generated session title.
- **Session → Compaction** — configure context compaction (see below).

### Compaction settings

Below the session management rows, the Session tab includes compaction configuration.
Navigate to a row with ++up++/++down++. Toggle switches (Enabled and the strategy
checkboxes) with ++enter++ or ++left++/++right++; adjust numeric values with
++left++/++right++:

| Setting | Values | Description |
|---|---|---|
| **Enabled** | on / off | Master toggle for compaction (Enter or arrows) |
| **`[x] tool-clearing`** | checkbox | Truncate oversized tool outputs (multiple strategies can be active) |
| **`[x] sliding-window`** | checkbox | Drop older messages past the threshold |
| **`[ ] summarization`** | checkbox | Summarize history with a dedicated LLM call |
| **Threshold** | 10%–100% (step 5) | Context usage % that triggers compaction |
| **Max messages** | 5–200 (step 5) | Messages kept in sliding-window mode |
| **Max tool output** | 500–20000 chars (step 500) | Truncation limit for tool output |

Strategies are independent checkboxes: any combination is allowed, including none
(compaction then does nothing until a strategy is re-enabled).

Changes save immediately to `~/.peggco/pegg.json`.

### Compaction persistence

Compaction does not destroy your history. When the context is compacted, the
compacted messages are saved to a **new session linked to the current one**, forming
a chain. The original session keeps every message it had; the newest session in the
chain holds the compacted view plus everything said afterwards.

- Loading a session always opens the **newest session in its chain** — you see the
  latest compacted conversation first.
- Scrolling to the top of the transcript loads the earlier (pre-compaction)
  conversation, marked with an `─ context compacted ─` divider, and keeps
  walking back through the chain as you continue scrolling.
- Resuming a session re-applies compaction to the loaded history (sliding-window
  and tool-clearing), so resumed conversations stay within the context budget.
- While chatting, a compacted run shows a `↻ Context compacted (...)` item in the
  transcript.

### Permissions tab

The Permissions tab has its own internal navigation. When focused on the tab bar,
++left++/++right++ switches tabs. Press ++down++ to enter the presets, ++down++ again
to reach the **Judge model** row, ++down++ again for the **Threshold** row, and
++down++ a fourth time to reach the tool list.
In the tool list, ++left++/++right++ cycles the permission
mode for the selected tool. ++up++ from presets returns to the tab bar.

Press ++enter++ on the **Judge model** row to pick which model judges tool calls.
The picker lists **decision models** first (JEV via OpenRouter, marked with the
provider and a `decision` suffix), then every available LLM model across your
configured providers. Selecting one writes `judge_provider` / `judge_model` to
`~/.peggco/pegg.json`; choosing a decision model also enables the decision judge
by default. See [Permissions](../features/permissions.md#the-judge-flow).

The **Threshold** row adjusts the decision judge's minimum yes-probability
(`judge_threshold`): ++left++/++right++ step it by `0.05`, clamped between `0.05`
and `1.0`, and the value is saved immediately. The row is disabled (and labeled
*decision only*) when the judge provider is not decision-capable.

### Memory tab

The Memory tab configures the [markdown-based memory system](../features/memory.md).
Press ++down++ to enter the rows, ++up++/++down++ to navigate, and ++enter++ or
++left++/++right++ to toggle switches and adjust values:

| Row | Description |
|---|---|
| **Enabled** | Master switch for memory capture and panel injection (on/off) |
| **Observer** | Provider/model for memory LLM calls, or *default (agent model)*. ++enter++ opens a searchable picker with decision providers and LLM models |
| **Relevance gating** | Cycle `auto` → `decision` → `llm` → `score` → `off` with ++left++/++right++ |
| **Relevance threshold** | `0.05`–`1.0`, stepped by `0.05`. Only active for `decision` gating (labeled *decision only* otherwise) |
| **Context budget** | `500`–`100000` chars, stepped by `500` |
| **Max results** | `1`–`10` |
| **Consolidate** | Run the librarian after each run (on/off) |
| **Entries / Last entry** | Read-only stats from the memory index |
| **Clear memory** | Deletes all memory files for this project (with confirmation) |

All changes save immediately to `~/.peggco/pegg.json` and apply live.

### System tab

The System tab displays application information (name, version, OS, architecture) and
a **Check for updates** button. Navigate to the button with ++down++ and press
++enter++ to check for a newer version. Status messages appear below the button:
"Checking...", "Already up to date", or an error. If an update is available, the
update modal opens automatically.

## Themes

Pegg ships 26 themes: `dark` (default), `light`, `dracula`, `catppuccin-mocha`,
`catppuccin-latte`, `catppuccin-frappe`, `catppuccin-macchiato`,
`tokyonight-storm`, `tokyonight-night`, `tokyonight-day`, `gruvbox-dark`,
`gruvbox-light`, `nord`, `onedark`, `solarized-dark`, `solarized-light`,
`rosepine`, `rosepine-moon`, `rosepine-dawn`, `monokai`, `monokai-night`,
`monokai-spectrum`, `kanagawa`, `kanagawa-dragon`, `everforest-dark`, and
`everforest-light`.

Press ++ctrl+t++ to cycle themes alphabetically, or pick one in
Settings → General → Theme. The choice is saved to the `theme` key in
`~/.peggco/pegg.json`.

## Notifications

- **Update modal** — shown at startup when a newer release exists, with
  *Update* / *Later* buttons (++left++/++right++ to choose, ++enter++ to confirm,
  ++esc++ to skip).
- **Errors** — appended to the transcript or shown on the hint line.
- **Transient messages** — retry progress (`Request failed ... retrying in Xs`) and
  the Esc interrupt hint appear above the status bar.
- **Desktop notifications** — permission prompts, agent completions, and agent
  errors raise an OS notification while the window is in the background. See
  [Notifications](../features/notifications.md).

## Session behavior

Each submitted message resumes the active session; if none is active, one is created
automatically and titled by a background model call. See
[Sessions](../features/sessions.md).

## Limitations

- No system clipboard copy/paste — use the terminal's own selection.
- No session fork or revert from the UI (available programmatically via the
  [SDK](../../sdk/sessions.md)).
- No command palette; Settings (++ctrl+p++) and the inline `/` and `@` pickers cover
  the same ground.
