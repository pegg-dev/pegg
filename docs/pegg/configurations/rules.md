---
icon: lucide/shield-alert
---

# Rules

Rules are plain Markdown files whose contents are injected verbatim into every
agent's system prompt. Use them for project conventions that must always apply.

## Locations

| Scope | Path | Applies to |
|---|---|---|
| Global | `~/.pegg/rules/*.md` | Every project on the machine |
| Project | `<project>/.pegg/rules/*.md` | The current project only |

The project `.pegg/` directory is created automatically at startup. Only `.md`
files directly inside a rules directory are read; subdirectories are ignored. Files
are sorted by name within each directory, and empty files are skipped.

## How rules are injected

Rules are appended to the end of the system prompt, after the agent identity, the
environment block, the skills list, and `AGENTS.md`:

```
# Rules

WARNING: You MUST strictly follow these rules. Violation of these rules is unacceptable.

<contents of global rules, alphabetically>

<contents of project rules, alphabetically>
```

Global rules are always followed by project rules. There is no override or merge
logic — both are included, so avoid contradictions between them.

Every built-in agent receives the same rules.

## Writing rules

Keep rules short, concrete, and actionable. A rule file can contain any Markdown:

```markdown title=".pegg/rules/testing.md"
# Testing rules

- Run `make test` before reporting a task complete.
- Never modify files under `testdata/`.
- Prefer table-driven tests for new Go code.
```

```markdown title="~/.pegg/rules/style.md"
# Global style

- Never add comments unless the code cannot be understood without them.
- Use the project's existing formatting and linting tools.
- Do not commit changes unless explicitly asked.
```

## Creating rules with the agent

The built-in `/rule` skill lets you create or update rules through a conversation
instead of editing files by hand:

- **Explicit content** — tell Pegg the rule and it writes it, e.g.
  `"/rule: never commit .env files"`.
- **Unknown content** — if you don't say what the rule should be, the agent asks
  you with the `askuserquestion` tool.
- **Agent-proposed** — you can also ask Pegg to draft a rule from the
  conversation (e.g. `"/rule: save a rule based on our conversation"`); it scans
  for preferences and corrections you repeated and presents candidates for your
  approval before writing anything.

The agent picks the scope: project rules default to `<project>/.pegg/rules/`,
global rules to `~/.pegg/rules/`, and it asks when the scope is unclear. New
rules are injected into subsequent conversations without a restart.

## Rules vs. AGENTS.md

Pegg reads `AGENTS.md` from the current working directory and injects it as
**Project Instructions**, before the rules section. The two complement each other:

| | `AGENTS.md` | Rules |
|---|---|---|
| Purpose | Project overview, build commands, architecture notes | Hard constraints the agent must obey |
| Scope | Project root | Global and project rule directories |
| Tone | Descriptive | Imperative |

The `/init` skill generates an `AGENTS.md` for you. See
[Commands](../features/commands.md#init).

## Viewing rules

Open Settings with ++ctrl+p++ and switch to the **Rules** tab. It lists every loaded
rule file with a `global` or `project` label. The list is read-only; edit the files
directly, or use the `/rule` skill, to change them.
