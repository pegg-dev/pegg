---
icon: lucide/workflow
---

# Workflow examples

This page collects ready-made workflows for every mode, plus variations:
different providers, custom labels, enterprise servers, dry runs, and extra
guards. Copy, adapt, and combine them.

All examples assume the bot repository is published at `peggbot/peggbot`
(change the `uses:` line while it is still a private repo, or pin a commit
SHA for production: `uses: peggbot/peggbot@<sha>`).

---

## Solve an issue (basic)

Triggers when an issue is labeled `peggbot`, solves it and opens a PR.

```yaml
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
        with:
          fetch-depth: 0

      - name: Run peggbot
        uses: peggbot/peggbot@v1
        with:
          mode: solve-issue
          token: ${{ secrets.GITHUB_TOKEN }}
          llm-provider: ${{ vars.PEGGBOT_PROVIDER }}
          llm-model: ${{ vars.PEGGBOT_MODEL }}
          llm-api-key: ${{ secrets.PEGGBOT_API_KEY }}
          prompt-file: .github/prompts/solve-issue.md
          issue-number: ${{ github.event.issue.number }}
          branch-prefix: peggbot
          base-branch: main
          max-iterations: '80'
          timeout-minutes: '40'
```

What happens:

1. The workflow runs only for the `peggbot` label (`if:` on `label.name`).
2. `fetch-depth: 0` gives the agent full history for `git diff`, logs and
   tests.
3. The agent reads the issue (title, body, comments, images), reproduces and
   fixes it, runs your checks, then:
   - creates `peggbot/issue-<n>` from `main`,
   - commits with a conventional message and pushes,
   - opens a PR with `Closes #<n>` via `github_create_pull_request`,
   - comments on the issue with the PR link.

!!! note
    The `if:` condition uses the workflow's `github.event.label.name`, while
    the action itself derives the issue number from the event. The explicit
    `issue-number` input is a safety net and documents intent.

---

## Solve an issue with OpenAI

Same behavior, different provider — swap `llm-provider`, the model variable,
and the secret:

```yaml
      - name: Run peggbot
        uses: peggbot/peggbot@v1
        with:
          mode: solve-issue
          token: ${{ secrets.GITHUB_TOKEN }}
          llm-provider: openai
          llm-model: ${{ vars.PEGGBOT_OPENAI_MODEL }}
          llm-api-key: ${{ secrets.PEGGBOT_OPENAI_API_KEY }}
          issue-number: ${{ github.event.issue.number }}
```

Because the engine is configured entirely from inputs, any pegg provider
works — including `llm-base-url` for proxies or self-hosted gateways:

```yaml
          llm-provider: openai
          llm-model: gpt-4o
          llm-api-key: ${{ secrets.PROXY_KEY }}
          llm-base-url: https://gateway.example.com/v1
```

---

## Solve an issue with a custom label and branch scheme

Use a different label and shape the branch/PR naming:

```yaml
on:
  issues:
    types: [labeled]

jobs:
  solve:
    if: github.event.label.name == 'autofix'
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: peggbot/peggbot@v1
        with:
          mode: solve-issue
          token: ${{ secrets.GITHUB_TOKEN }}
          llm-provider: ${{ vars.PEGGBOT_PROVIDER }}
          llm-model: ${{ vars.PEGGBOT_MODEL }}
          llm-api-key: ${{ secrets.PEGGBOT_API_KEY }}
          issue-number: ${{ github.event.issue.number }}
          branch-prefix: bot/autofix        # branches: bot/autofix/issue-12
          base-branch: develop              # PRs merge into develop
          max-iterations: '100'
          timeout-minutes: '50'
```

### Trial run first (dry-run)

Never written to GitHub — the agent still works, but comments, PRs, labels
and releases are skipped and the intended actions are logged:

```yaml
          dry-run: 'true'
```

Run this once in a test issue to see the agent's plan and tool calls in the
workflow log before letting it write anything.

---

## Review a pull request (basic)

Triggers when the bot is requested as a reviewer. It runs the built-in
`/review` pipeline and submits a verdict.

```yaml
name: Peggbot Review PR

on:
  pull_request:
    types: [review_requested]

permissions:
  contents: read
  pull-requests: write

jobs:
  review:
    if: >-
      github.event.requested_reviewer != null &&
      github.event.requested_reviewer.login == 'peggbot' &&
      github.event.pull_request.head.repo.full_name == github.repository
    runs-on: ubuntu-latest
    timeout-minutes: 45
    steps:
      - uses: actions/checkout@v4
        with:
          ref: ${{ github.event.pull_request.head.ref }}
          fetch-depth: 0

      - name: Run peggbot
        uses: peggbot/peggbot@v1
        with:
          mode: review-pr
          token: ${{ secrets.GITHUB_TOKEN }}
          llm-provider: ${{ vars.PEGGBOT_PROVIDER }}
          llm-model: ${{ vars.PEGGBOT_MODEL }}
          llm-api-key: ${{ secrets.PEGGBOT_API_KEY }}
          prompt-file: .github/prompts/review-pr.md
          pull-request-number: ${{ github.event.pull_request.number }}
          max-iterations: '80'
          timeout-minutes: '40'
```

What happens:

1. The `if:` condition fires only for `peggbot` as the requested reviewer
   **and** same-repository PRs (fork PRs get a read-only token and cannot post
   reviews — they are filtered out explicitly).
2. The PR head branch is checked out, so the agent can read the actual code.
3. The agent gathers context with `github_get_pull_request` and
   `github_get_pr_diff`, runs the `/review` pipeline (size assessment →
   context → hunk review → verification → gap sweep), then:
   - posts inline findings with `github_create_pr_review_comment`,
   - submits `APPROVE`, `REQUEST_CHANGES` or `COMMENT` with
     `github_submit_review`.

!!! tip
    Add the bot account as a repository collaborator (Read is enough) so it
    can be selected as a reviewer.

### Review only for a specific base branch

```yaml
    if: >-
      github.event.requested_reviewer.login == 'peggbot' &&
      github.event.pull_request.base.ref == 'main'
```

### Review with an OpenAI model and stricter iterations

```yaml
          llm-provider: openai
          llm-model: gpt-4o
          llm-api-key: ${{ secrets.PEGGBOT_OPENAI_API_KEY }}
          max-iterations: '120'
          timeout-minutes: '55'
```

---

## Respond to @peggbot mentions (basic)

Answers questions from admins/owners on issues **and** PR conversations.

```yaml
name: Peggbot Respond

on:
  issue_comment:
    types: [created]

permissions:
  issues: write
  pull-requests: write

jobs:
  respond:
    if: contains(github.event.comment.body, '@peggbot')
    runs-on: ubuntu-latest
    timeout-minutes: 20
    steps:
      - uses: actions/checkout@v4

      - name: Run peggbot
        uses: peggbot/peggbot@v1
        with:
          mode: respond
          token: ${{ secrets.GITHUB_TOKEN }}
          llm-provider: ${{ vars.PEGGBOT_PROVIDER }}
          llm-model: ${{ vars.PEGGBOT_MODEL }}
          llm-api-key: ${{ secrets.PEGGBOT_API_KEY }}
          prompt-file: .github/prompts/respond.md
          issue-number: ${{ github.event.issue.number }}
          required-permission: admin
          unauthorized-action: skip
          bot-login: peggbot
          max-iterations: '40'
          timeout-minutes: '15'
```

What happens:

1. Any comment containing `@peggbot` triggers the run.
2. The action checks the comment author:
   - bot's own comments → skipped (loop guard),
   - permission below `admin` → skipped silently (`unauthorized-action: skip`),
   - admins/owners → proceed.
3. The agent receives the full thread (title, body, every comment) plus
   downloaded image attachments, verifies claims against the checked-out code
   (`file:line` citations), and the bot posts the answer as a comment.

Example exchanges:

> **@owner:** @peggbot is this really an issue?

> **peggbot:** Yes — `internal/llm/manager.go:412` returns early when the
> provider reports `rate_limit_exceeded`, which skips the circuit breaker
> entirely. Repro: set `max_retries: 0` and fire 20 concurrent requests.

### Allow writes to ask too, and decline politely

```yaml
          required-permission: write
          unauthorized-action: comment
```

Now anyone with Write access can ask, and users below it get a polite
"peggbot only responds to repository write" comment instead of silence.

### Answer mentions on inline review comments

`pull_request_review_comment` events fire when someone replies to an inline
review comment. The conversation number comes from the PR, not the issue:

```yaml
name: Peggbot Respond Inline

on:
  pull_request_review_comment:
    types: [created]

permissions:
  issues: write
  pull-requests: write

jobs:
  respond:
    if: contains(github.event.comment.body, '@peggbot')
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with:
          ref: ${{ github.event.pull_request.head.ref }}

      - uses: peggbot/peggbot@v1
        with:
          mode: respond
          token: ${{ secrets.GITHUB_TOKEN }}
          llm-provider: ${{ vars.PEGGBOT_PROVIDER }}
          llm-model: ${{ vars.PEGGBOT_MODEL }}
          llm-api-key: ${{ secrets.PEGGBOT_API_KEY }}
          issue-number: ${{ github.event.pull_request.number }}
          required-permission: admin
```

### Tighten the mention pattern

Require the mention to be the first token of the comment (avoids accidental
triggers in long threads):

```yaml
    if: startsWith(github.event.comment.body, '@peggbot')
```

---

## Release notes (basic)

Runs after a release is published and rewrites the auto-generated body into
structured notes.

```yaml
name: Peggbot Release Notes

on:
  release:
    types: [published]

permissions:
  contents: write

jobs:
  notes:
    runs-on: ubuntu-latest
    timeout-minutes: 20
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0
          fetch-tags: true

      - name: Run peggbot
        uses: peggbot/peggbot@v1
        with:
          mode: release-notes
          token: ${{ secrets.GITHUB_TOKEN }}
          llm-provider: ${{ vars.PEGGBOT_PROVIDER }}
          llm-model: ${{ vars.PEGGBOT_MODEL }}
          llm-api-key: ${{ secrets.PEGGBOT_API_KEY }}
          prompt-file: .github/prompts/release-notes.md
          release-id: ${{ github.event.release.id }}
          max-iterations: '40'
          timeout-minutes: '15'
```

What happens:

1. The action fetches the release and finds the **previous** non-draft
   release.
2. It compares commits `previous-tag...new-tag` via the API and passes the
   subject lines (with authors) to the agent.
3. The agent groups commits by conventional-commit type into
   `## New Features & Enhancements`, `## Bug Fixes`,
   `## Refactors & Under the Hood`, `## Documentation & Chores` with `###`
   area subsections, rewrites each commit into a user-friendly bullet, and
   publishes the body with `github_update_release`.

Example output:

```markdown
## New Features & Enhancements

### Terminal User Interface (TUI)

* **Rendering Optimization:** Introduced incremental chat rendering and
  coalesced redraws for a smoother chat experience.
* **Settings System Tab:** Added a dedicated System tab to the settings page.

---

## Bug Fixes

### Core & Providers

* **Streaming:** Fixed an issue regarding the agent streaming deadline.
```

### Notes on tag pushes instead of releases

The `release-notes` mode needs a numeric release id. If you publish tags
without release objects (e.g. pure `git tag` + CI), resolve the id first with
the `gh` CLI:

```yaml
on:
  push:
    tags:
      - 'v*'

permissions:
  contents: write

jobs:
  notes:
    runs-on: ubuntu-latest
    timeout-minutes: 20
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0
          fetch-tags: true

      - name: Resolve release id
        id: release
        run: |
          TAG="${GITHUB_REF#refs/tags/}"
          ID="$(gh release view "$TAG" --json id -q .id)"
          echo "release_id=${ID}" >> "$GITHUB_OUTPUT"
        env:
          GH_TOKEN: ${{ secrets.GITHUB_TOKEN }}

      - uses: peggbot/peggbot@v1
        with:
          mode: release-notes
          token: ${{ secrets.GITHUB_TOKEN }}
          llm-provider: ${{ vars.PEGGBOT_PROVIDER }}
          llm-model: ${{ vars.PEGGBOT_MODEL }}
          llm-api-key: ${{ secrets.PEGGBOT_API_KEY }}
          prompt-file: .github/prompts/release-notes.md
          release-id: ${{ steps.release.outputs.release_id }}
```

If the tag has no release object yet, the previous-release comparison falls
back to `github_get_release_by_tag` / `github_list_releases` inside the
prompt — or simply to the workspace git history.

---

## GitHub Enterprise Server

Everything targets your GHES instance — API and git push:

```yaml
jobs:
  solve:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: peggbot/peggbot@v1
        with:
          mode: solve-issue
          token: ${{ secrets.GITHUB_TOKEN }}
          llm-provider: ${{ vars.PEGGBOT_PROVIDER }}
          llm-model: ${{ vars.PEGGBOT_MODEL }}
          llm-api-key: ${{ secrets.PEGGBOT_API_KEY }}
          issue-number: ${{ github.event.issue.number }}
          github-server-url: https://ghe.example.com
          ghes-api-base-url: https://ghe.example.com/api/v3/
```

- `github-server-url` builds the git auth rewrite:
  `url."https://x-access-token:<token>@ghe.example.com/".insteadOf "https://ghe.example.com/"`.
- `ghes-api-base-url` points every API call (issues, PRs, reviews, releases,
  permissions) at the enterprise API root.

---

## Combining workflows: one file, many modes

You can enable multiple modes in a single workflow file by splitting them
into jobs — useful to keep the workflow list short:

```yaml
name: Peggbot

on:
  issues:
    types: [labeled]
  pull_request:
    types: [review_requested]
  issue_comment:
    types: [created]

permissions:
  contents: write
  pull-requests: write
  issues: write

jobs:
  solve:
    if: github.event_name == 'issues' && github.event.label.name == 'peggbot'
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: peggbot/peggbot@v1
        with:
          mode: solve-issue
          token: ${{ secrets.GITHUB_TOKEN }}
          llm-provider: ${{ vars.PEGGBOT_PROVIDER }}
          llm-model: ${{ vars.PEGGBOT_MODEL }}
          llm-api-key: ${{ secrets.PEGGBOT_API_KEY }}
          issue-number: ${{ github.event.issue.number }}

  review:
    if: >-
      github.event_name == 'pull_request' &&
      github.event.requested_reviewer != null &&
      github.event.requested_reviewer.login == 'peggbot'
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with:
          ref: ${{ github.event.pull_request.head.ref }}
      - uses: peggbot/peggbot@v1
        with:
          mode: review-pr
          token: ${{ secrets.GITHUB_TOKEN }}
          llm-provider: ${{ vars.PEGGBOT_PROVIDER }}
          llm-model: ${{ vars.PEGGBOT_MODEL }}
          llm-api-key: ${{ secrets.PEGGBOT_API_KEY }}
          pull-request-number: ${{ github.event.pull_request.number }}

  respond:
    if: github.event_name == 'issue_comment' && contains(github.event.comment.body, '@peggbot')
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: peggbot/peggbot@v1
        with:
          mode: respond
          token: ${{ secrets.GITHUB_TOKEN }}
          llm-provider: anthrop${{ vars.PEGGBOT_PROVIDER }}ic
          llm-model: ${{ vars.PEGGBOT_MODEL }}
          llm-api-key: ${{ secrets.PEGGBOT_API_KEY }}
          issue-number: ${{ github.event.issue.number }}
```

!!! warning
    With a combined trigger list, every job runs for **every** event — the
    `if:` conditions are what route each event to the right job. Prefer
    separate files unless the workflow list is getting long.

---

## Hardening checklist

- [ ] Pin the action to a release (`@v1`) or commit SHA instead of `@main`.
- [ ] Keep `llm-api-key` in secrets, never in plain `with:` literals.
- [ ] Start with `dry-run: 'true'` for `solve-issue`.
- [ ] Restrict `respond` to `admin` unless you have a reason to open it up.
- [ ] Keep `if:` guards on fork PRs for `review-pr`.
- [ ] Set `timeout-minutes` on every job — agent runs can be long.