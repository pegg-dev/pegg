---
icon: lucide/file-text
---

# Prompts

Every mode ships with an embedded default system prompt. You extend or
override it per workflow with repository-owned prompt files and skills.

## Prompt files (`.github/prompts/*.md`)

The `prompt-file` input points at a Markdown file inside the checked-out
repository. Its content is appended to the mode's default system prompt as
"Additional instructions from the workflow" — perfect for repository
conventions that the generic prompt cannot know.

```yaml
      - uses: peggbot/peggbot@v1
        with:
          mode: review-pr
          token: ${{ secrets.GITHUB_TOKEN }}
          llm-provider: ${{ vars.PEGGBOT_PROVIDER }}
          llm-model: ${{ vars.PEGGBOT_MODEL }}
          llm-api-key: ${{ secrets.PEGGBOT_API_KEY }}
          prompt-file: .github/prompts/review-pr.md
```

A typical `review-pr.md`:

```markdown
# Review PR — additional instructions

Appended to the default review-pr prompt.

- Flag missing tests for new code paths as `[Warning]`.
- Check that error messages are lowercase and wrapped with `%w`.
- Prefer `errors.Is` over direct error comparison.
- Reviewer tone: precise, kind, and concrete. Suggest the fix, not just the
  problem.
```

Each mode reads a different file, so the same action can carry a separate
prompt for solving, reviewing, responding and release notes:

```
.github/prompts/
├── solve-issue.md      # repository build/test rules
├── review-pr.md        # review conventions
├── respond.md          # answer style and language rules
└── release-notes.md    # release note format
```

## Skills (`skills-dir`)

`skills-dir` points at a directory of **skill folders** — one folder per
skill, each containing a `SKILL.md` file with YAML frontmatter. The folders
are loaded into the agent with the SDK skill engine, which also wires
`/skill-name` expansion into agent inputs.

```yaml
          prompt-file: .github/prompts/release-notes.md
          skills-dir: .github/prompts
```

A skill folder inside the same tree:

```
.github/prompts/
└── release-notes/
    └── SKILL.md
```

`SKILL.md`:

```markdown
---
name: release-notes
description: >-
  Custom release-note formatting rules for this repository.
when_to_use: When generating release notes.
---

# Repository release notes

- Group by area: `### Core`, `### TUI`, `### CLI`, `### Docs`.
- Prefix every bullet with the component tag, e.g. `**[Core]** Fixed ...`.
- Never list dependabot bumps individually; fold them into a single
  "Dependency updates" bullet.
```

### Skill frontmatter fields

| Field | Description |
|---|---|
| `name` | Lowercase, `a-z0-9` and hyphens. Defaults to the folder name |
| `description` | Shown to the agent so it knows when to use the skill |
| `when_to_use` | Extra guidance for the agent |
| `arguments` / `argument-hint` | Usage hints for `/name arg1 arg2` expansion |
| `allowed-tools` | Optional tool restriction for the skill |

Both mechanisms compose: default prompt + `prompt-file` instructions + loaded
skills. Keep the prompt file for repository rules and skills for reusable
procedures.

## The default prompts

The embedded defaults are the base layer. Key contracts:

- **solve-issue** — analyze → plan → implement → verify (run the project's
  checks) → branch `<prefix>/issue-<n>` → commit conventionally → push →
  `github_create_pull_request` with `Closes #<n>` → comment on the issue. If
  the issue is not reproducible, comment why instead of opening a PR.
- **review-pr** — runs `/review review this PR carefully and leave comment`,
  using `github_get_pull_request` / `github_get_pr_diff`, posting inline
  comments and a verdict.
- **respond** — answers the admin's question with evidence (`file:line`
  citations), never posts comments itself (the bot posts the final message).
- **release-notes** — groups commits by conventional-commit type into
  `## New Features & Enhancements` / `## Bug Fixes` /
  `## Refactors & Under the Hood` / `## Documentation & Chores` with `###`
  area subsections, rewrites subjects into bold-labeled bullets, publishes via
  `github_update_release`.

You never need to repeat these contracts in your prompt files — only the
deltas.