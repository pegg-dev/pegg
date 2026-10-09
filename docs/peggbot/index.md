---
icon: lucide/bot
---

# Peggbot

Peggbot is a GitHub bot built on the **Pegg Go SDK**. It runs as a GitHub
Action in your repository and gives the agent direct, tool-driven access to
your issues, pull requests, reviews and releases — no CLI, no server, no
webhooks to maintain.

The action is a Go binary (composite action) that embeds the SDK, opens an
engine against your checkout, registers the [GitHub toolkit](github-tools.md),
and lets the agent do the work. Every mode is fully autonomous: the agent
drives its own tools end to end.

## What it can do

| Mode | Trigger | What happens |
|---|---|---|
| `solve-issue` | Issue gets the `peggbot` label | The agent analyzes the issue, implements the fix, runs the project's checks, pushes a branch and opens a pull request with `Closes #n`. It comments on the issue with the PR link |
| `review-pr` | `peggbot` is requested as PR reviewer | The agent runs the built-in `/review` skill pipeline, posts inline comments with `github_create_pr_review_comment` and submits a verdict with `github_submit_review` (`APPROVE` / `REQUEST_CHANGES` / `COMMENT`) |
| `respond` | A comment mentions `@peggbot` | The agent answers with the full issue/PR thread and image attachments as context. **Only repository admins/owners can trigger it** (configurable) |
| `release-notes` | A release is published | The agent rewrites the auto-generated release body into structured, grouped release notes from the commits between the previous release and the new tag |

## How it works

```
┌────────────────────────────────────────────────────────────────┐
│  .github/workflows/peggbot-*.yml   (event triggers)          │
│        │ uses: peggbot/peggbot@v1                            │
│        ▼                                                       │
│  peggbot (composite action)                                  │
│   ├── git auth via insteadOf (token)                          │
│   ├── go build ./cmd/peggbot                                  │
│   └── run ──▶ sdk.Open(Workspace=checkout)                     │
│                ├── RegisterTool(GitHub toolkit)                │
│                ├── LoadSkills(.github/prompts)                 │
│                └── ChatStream(mode prompt + prompt file)       │
│                     │ agent uses bash/file/git/github tools    │
│                     ▼                                          │
│        GitHub API: comments, PRs, reviews, releases            │
└────────────────────────────────────────────────────────────────┘
```

- The agent always gets the built-in tool set (file ops, bash, web, todos,
  subagents, plan mode) plus the [GitHub toolkit](github-tools.md).
- System prompts are composed from an embedded default per mode plus an
  optional repository-owned prompt file from [`.github/prompts`](prompts.md).
- Tool calls stream into the workflow log so every run is auditable.

## Repository layout

| Path | Purpose |
|---|---|
| `action.yml` | Composite action definition and all inputs |
| `cmd/peggbot/` | Entry point: reads inputs + event payload, dispatches modes |
| `internal/config/` | Input parsing and validation |
| `internal/github/` | API client, event parsing, permission checks, attachment downloads |
| `internal/githubtools/` | The GitHub toolkit registered into the agent |
| `internal/modes/` | The four modes: solve, review, respond, release notes |
| `internal/prompts/` | Embedded default system prompts |
| `.github/workflows/` | Ready-made workflows for all four modes |
| `.github/prompts/` | Example prompt files and a sample skill |

## Quick start

```yaml
# .github/workflows/peggbot-solve-issue.yml
name: Peggbot Solve Issue

on:
  issues:
    types: [labeled]

permissions:
  contents: write
  pull-requests: write
  issues: write

jobs:
  solve:
    if: github.event.label.name == 'peggbot'
    runs-on: ubuntu-latest
    timeout-minutes: 45
    steps:
      - uses: actions/checkout@v4
      - uses: peggbot/peggbot@v1
        with:
          mode: solve-issue
          token: ${{ secrets.GITHUB_TOKEN }}
          llm-provider: ${{ vars.PEGGBOT_PROVIDER }}
          llm-model: ${{ vars.PEGGBOT_MODEL }}
          llm-api-key: ${{ secrets.PEGGBOT_API_KEY }}
          prompt-file: .github/prompts/solve-issue.md
```

1. Add a `peggbot` label to the repository.
2. Store the LLM key as a secret (`PEGGBOT_API_KEY`) and the model as a
   variable (`PEGGBOT_MODEL`).
3. Add the bot account as a collaborator (Read is enough for reviews; Write if
   it should also self-assign).
4. Label an issue and watch it solve itself.

## Before you enable it

- The bot **pushes branches and opens PRs** in your repository. Start with
  `dry-run: "true"` to observe behavior without any writes.
- `respond` mode is gated to admins/owners by default — see
  [Configuration](configuration.md) and the [workflow examples](workflows.md).
- Fork PRs are excluded from reviews by the example workflows (a read-only
  token cannot post reviews); same-repo PRs are fully supported.

More examples — different providers, custom labels, enterprise servers,
prompt-driven behavior — are in [Workflows](workflows.md).