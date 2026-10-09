---
icon: lucide/sliders-horizontal
---

# Configuration

Every peggbot run is configured through action inputs. Nothing is hard-coded:
each workflow can pin different models, prompts, permissions, branches, and
timeouts.

## Inputs

### Mode & authentication

| Input | Required | Default | Description |
|---|---|---|---|
| `mode` | yes | — | `solve-issue`, `review-pr`, `respond` or `release-notes` |
| `token` | yes | — | `${{ secrets.GITHUB_TOKEN }}` or a fine-grained PAT of the bot account |

!!! tip "Which token?"
    `GITHUB_TOKEN` is scoped to the current workflow permissions and works for
    all modes. Use a PAT when you want the bot's actions attributed to the bot
    account (e.g. reviews) or when the repository needs a persistent identity.

### LLM

| Input | Required | Default | Description |
|---|---|---|---|
| `llm-provider` | yes | — | Provider name as registered in pegg: `anthropic`, `openai`, `google`, … |
| `llm-model` | yes | — | Model id, e.g. `claude-sonnet-4-5`, `gpt-4o` |
| `llm-api-key` | yes | — | Provider API key — always pass as a secret |
| `llm-base-url` | no | — | Provider/base URL override (proxy, self-hosted endpoint) |

### Prompting

| Input | Default | Description |
|---|---|---|
| `prompt-file` | — | Markdown file inside the repo (e.g. `.github/prompts/solve-issue.md`) whose content is appended to the mode's system prompt |
| `skills-dir` | — | Directory of `SKILL.md` skill folders loaded into the agent via the SDK skill engine |

See [Prompts](prompts.md).

### Trigger context

These override the values derived from the workflow event. In the example
workflows they are always bound to event fields, so they rarely need editing.

| Input | Used by | Description |
|---|---|---|
| `issue-number` | `solve-issue`, `respond` | Issue or PR conversation number |
| `pull-request-number` | `review-pr`, `respond` | Pull request number |
| `release-id` | `release-notes` | Numeric release id |
| `comment-id` | `respond` | Trigger comment id (informational) |

### Behavior

| Input | Default | Description |
|---|---|---|
| `required-permission` | `admin` | Minimum actor permission for `respond`: `admin`, `write` or `read`. Owners and org admins count as `admin` |
| `unauthorized-action` | `skip` | What happens when the actor lacks permission: `skip` (silent) or `comment` (post a polite decline) |
| `bot-login` | `peggbot` | The bot's login. Its own comments are never answered (loop guard) |
| `branch-prefix` | `peggbot` | `solve-issue` branches become `<prefix>/issue-<n>` |
| `base-branch` | `main` | Base branch for PRs opened by `solve-issue` |
| `max-iterations` | `80` | Cap on agent loop iterations |
| `timeout-minutes` | `30` | Cap on agent runtime before the run is aborted |
| `max-attachments` | `5` | Max image attachments downloaded from a thread |
| `workspace` | repo root | Agent workspace root relative to the checkout |
| `dry-run` | `false` | When `true`, nothing is written to GitHub |

### Enterprise

| Input | Default | Description |
|---|---|---|
| `github-server-url` | `https://github.com` | Used to build the git push auth URL (set to your GHES host) |
| `ghes-api-base-url` | — | GitHub Enterprise API root, e.g. `https://ghe.example.com/api/v3/` |

## Environment

The action reads the standard GitHub Actions environment automatically:

| Variable | Used for |
|---|---|
| `GITHUB_REPOSITORY` | Owner/repo of the current repository |
| `GITHUB_ACTOR` | Who triggered the run (permission gate, loop guard) |
| `GITHUB_EVENT_PATH` | The webhook payload JSON |
| `GITHUB_WORKSPACE` | Default workspace root |
| `GITHUB_ACTION_PATH` | Action source directory (the compiled binary is built here) |

## Permission model

The **`respond`** mode is the only mode with an interactive trigger, so it is
the only one gated by default:

1. If the comment author is `bot-login`, the run stops immediately.
2. The actor's permission is resolved via
   `GET /repos/{owner}/{repo}/collaborators/{actor}/permission`.
3. `admin` < `write` < `read` < `none`; the run proceeds only when the actor's
   level satisfies `required-permission`.
4. On denial: `skip` logs and does nothing; `comment` posts a short decline.

`solve-issue` (label trigger) and `review-pr` (review request) are triggered
by the workflow's `if:` conditions; restrict them further with additional
conditions in the workflow, e.g. requiring a specific labeler.

## Provider keys

The engine is opened with `Options.APIKeys` and `Options.Providers` set from
`llm-provider`, `llm-model`, `llm-api-key` and `llm-base-url`, so a run works
with any provider the pegg SDK supports — no local config file needed.