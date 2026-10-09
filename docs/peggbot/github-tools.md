---
icon: lucide/git-pull-request
---

# GitHub tools

The agent does not talk to GitHub through magic — it drives the same tools you
see in its logs. The toolkit is a plain Go package in the bot repository
(`internal/githubtools/`), built on `sdk.NewTool` and `go-github`, registered
into the engine at startup via `Engine.RegisterTool`.

Tool names are namespaced with `github_` and deliberately match what the
built-in `/review` skill expects to find (`github_create_pr_review_comment`,
`github_submit_review`).

## Issues & comments

| Tool | Purpose |
|---|---|
| `github_get_issue` | Full issue: title, body, state, labels, author, comment thread, and image attachment URLs |
| `github_post_issue_comment` | Post a comment on an issue or PR conversation |
| `github_update_issue` | Change title, body, or state (`open` / `closed`) |
| `github_add_labels` | Add labels |
| `github_remove_label` | Remove a label |

## Pull requests & reviews

| Tool | Purpose |
|---|---|
| `github_get_pull_request` | PR metadata, changed files (with patches), conversation, review comments, and reviews |
| `github_get_pr_diff` | The raw unified diff of the PR |
| `github_create_pull_request` | Open a PR from an existing branch (`draft` supported) |
| `github_create_pr_review_comment` | Inline comment on a file/line of the diff |
| `github_submit_review` | Submit `APPROVE`, `REQUEST_CHANGES` or `COMMENT`, optionally with inline comments attached |
| `github_list_pr_reviews` | Existing reviews on the PR |

## Permissions

| Tool | Purpose |
|---|---|
| `github_get_actor_permission` | Permission level of a user: `admin`, `write`, `read`, or `none` |

Used by `respond` mode as the permission gate, and available to the agent in
any mode that needs to gate its own actions.

## Releases

| Tool | Purpose |
|---|---|
| `github_get_release` | Release by numeric id |
| `github_get_release_by_tag` | Release by tag, e.g. `v0.1.3` |
| `github_update_release` | Replace the release body (notes) |
| `github_list_releases` | Most recent releases, newest first |
| `github_compare_tags` | Commits between two tags/refs — the raw material for release notes |

## Examples of agent-driven flows

### Solving an issue

1. `github_get_issue` — read the issue and its thread.
2. file/bash tools — reproduce, implement, verify.
3. `git` via bash — branch `peggbot/issue-42`, commit, push.
4. `github_create_pull_request` — title, head `peggbot/issue-42`, base
   `main`, body ending with `Closes #42`.
5. `github_post_issue_comment` — summarize and link the PR.

### Reviewing a pull request

1. `github_get_pull_request` + `github_get_pr_diff` — full context.
2. `/review` skill pipeline — explorer/developer/verifier subagents.
3. `github_create_pr_review_comment` — one call per verified finding.
4. `github_submit_review` — the verdict with a summary body.

### Writing release notes

1. `github_get_release` — current release.
2. `github_compare_tags` — `prev-tag...new-tag` commit list.
3. Rewrite into grouped sections (prompt-driven).
4. `github_update_release` — publish the new body.

### Reusing the toolkit yourself

The toolkit is exported from `internal/githubtools` and needs only the SDK's
`NewTool` building block, so any SDK consumer can use the same pattern:

```go
tools, err := githubtools.NewGitHubTools(githubtools.GitHubToolOptions{
	Token:   os.Getenv("GITHUB_TOKEN"),
	Owner:   "myorg",
	Repo:    "myrepo",
})
for _, t := range tools {
	eng.RegisterTool(t)
}
```