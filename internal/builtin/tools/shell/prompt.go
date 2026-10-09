package shell

import "github.com/peggco/pegg/internal/agent/prompt"

func bashToolPromptBuilder() *prompt.Prompt {
	return prompt.New().
		Paragraph("Executes a given bash command in a persistent shell session with optional timeout, ensuring proper handling and security measures.").
		Paragraph("Before executing the command, please follow these steps:").
		XMLTag("directory-verification",
			prompt.Heading(3, "Directory Verification"),
			prompt.List(
				prompt.ListItem("If the command will create new directories or files, first use the List tool to verify the parent directory exists and is the correct location."),
				prompt.ListItem("For example, before running `mkdir foo/bar`, first use List to check that `foo` exists and is the intended parent directory."),
			),
		).
		XMLTag("command-execution",
			prompt.Heading(3, "Command Execution"),
			prompt.OrderedList(
				prompt.ListItem("Always quote file paths that contain spaces with double quotes (e.g., `cd \"path with spaces/file.txt\"`)."),
				prompt.ListItem("Examples of proper quoting:", prompt.List(
					prompt.ListItem("`cd \"/Users/name/My Documents\"` (correct)"),
					prompt.ListItem("`cd /Users/name/My Documents` (incorrect - will fail)"),
					prompt.ListItem("`python \"/path/with spaces/script.py\"` (correct)"),
					prompt.ListItem("`python /path/with spaces/script.py` (incorrect - will fail)"),
				)),
				prompt.ListItem("After ensuring proper quoting, execute the command."),
				prompt.ListItem("Capture the output of the command."),
			),
		).
		XMLTag("usage-notes",
			prompt.Heading(3, "Usage Notes"),
			prompt.List(
				prompt.ListItem("The command argument is required."),
				prompt.ListItem("You can specify an optional timeout in milliseconds (up to 600000ms / 10 minutes). If not specified, commands will timeout after 120000ms (2 minutes)."),
				prompt.ListItem("If the output exceeds 30000 characters, output will be truncated before being returned to you."),
				prompt.ListItem("VERY IMPORTANT: You MUST avoid using search commands like `find` and `grep`. Instead use Grep, Glob, or Task to search. You MUST avoid read tools like `cat`, `head`, `tail`, and `ls`, and use Read and List to read files."),
				prompt.ListItem("If you _still_ need to run `grep`, STOP. ALWAYS USE ripgrep at `rg` (or /usr/bin/rg) first, which all opencode users have pre-installed."),
				prompt.ListItem("When issuing multiple commands, use the `;` or `&&` operator to separate them. DO NOT use newlines (newlines are ok in quoted strings)."),
				prompt.ListItem("Try to maintain your current working directory throughout the session by using absolute paths and avoiding usage of `cd`. You may use `cd` if the User explicitly requests it.", prompt.List(
					prompt.ListItem("Good example: `pytest /foo/bar/tests`"),
					prompt.ListItem("Bad example: `cd /foo/bar && pytest tests`"),
				)),
			),
		).
		XMLTag("git-committing",
			prompt.Heading(3, "Committing Changes with Git"),
			prompt.Paragraph("When the user asks you to create a new git commit, follow these steps carefully:"),
			prompt.OrderedList(
				prompt.ListItem("You have the capability to call multiple tools in a single response. When multiple independent pieces of information are requested, batch your tool calls together for optimal performance. ALWAYS run the following bash commands in parallel, each using the Bash tool:", prompt.List(
					prompt.ListItem("Run a `git status` command to see all untracked files."),
					prompt.ListItem("Run a `git diff` command to see both staged and unstaged changes that will be committed."),
					prompt.ListItem("Run a `git log` command to see recent commit messages, so that you can follow this repository's commit message style."),
				)),
				prompt.ListItem("Analyze all staged changes (both previously staged and newly added) and draft a commit message. Wrap your analysis process in <commit_analysis> tags."),
				prompt.ListItem("You have the capability to call multiple tools in a single response. When multiple independent pieces of information are requested, batch your tool calls together for optimal performance. ALWAYS run the following commands in parallel:", prompt.List(
					prompt.ListItem("Add relevant untracked files to the staging area."),
					prompt.ListItem("Run `git status` to make sure the commit succeeded."),
				)),
				prompt.ListItem("If the commit fails due to pre-commit hook changes, retry the commit ONCE to include these automated changes. If it fails again, it usually means a pre-commit hook is preventing the commit. If the commit succeeds but you notice that files were modified by the pre-commit hook, you MUST amend your commit to include them."),
			),
			prompt.List(
				prompt.ListItem("Use the git context at the start of this conversation to determine which files are relevant to your commit. Be careful not to stage and commit files (e.g. with `git add .`) that aren't relevant to your commit."),
				prompt.ListItem("NEVER update the git config."),
				prompt.ListItem("DO NOT run additional commands to read or explore code, beyond what is available in the git context."),
				prompt.ListItem("DO NOT push to the remote repository."),
				prompt.ListItem("IMPORTANT: Never use git commands with the -i flag (like `git rebase -i` or `git add -i`) since they require interactive input which is not supported."),
				prompt.ListItem("If there are no changes to commit (i.e., no untracked files and no modifications), do not create an empty commit."),
				prompt.ListItem("Ensure your commit message is meaningful and concise. It should explain the purpose of the changes, not just describe them."),
				prompt.ListItem("Return an empty response - the user will see the git output directly."),
			),
		).
		XMLTag("git-pr",
			prompt.Heading(3, "Creating Pull Requests"),
			prompt.Paragraph("Use the `gh` command via the Bash tool for ALL GitHub-related tasks including working with issues, pull requests, checks, and releases. If given a Github URL use the `gh` command to get the information needed."),
			prompt.Paragraph("When the user asks you to create a pull request, follow these steps carefully:"),
			prompt.OrderedList(
				prompt.ListItem("You have the capability to call multiple tools in a single response. When multiple independent pieces of information are requested, batch your tool calls together for optimal performance. ALWAYS run the following bash commands in parallel using the Bash tool, in order to understand the current state of the branch since it diverged from the main branch:", prompt.List(
					prompt.ListItem("Run a `git status` command to see all untracked files."),
					prompt.ListItem("Run a `git diff` command to see both staged and unstaged changes that will be committed."),
					prompt.ListItem("Check if the current branch tracks a remote branch and is up to date with the remote, so you know if you need to push to the remote."),
					prompt.ListItem("Run a `git log` command and `git diff main...HEAD` to understand the full commit history for the current branch (from the time it diverged from the `main` branch)."),
				)),
				prompt.ListItem("Analyze all changes that will be included in the pull request, making sure to look at all relevant commits (NOT just the latest commit, but ALL commits that will be included in the pull request), and draft a pull request summary. Wrap your analysis process in <pr_analysis> tags."),
				prompt.ListItem("You have the capability to call multiple tools in a single response. When multiple independent pieces of information are requested, batch your tool calls together for optimal performance. ALWAYS run the following commands in parallel:", prompt.List(
					prompt.ListItem("Create new branch if needed."),
					prompt.ListItem("Push to remote with -u flag if needed."),
					prompt.ListItem("Create PR using `gh pr create` with the format below. Use a HEREDOC to pass the body to ensure correct formatting."),
				)),
			),
			prompt.List(
				prompt.ListItem("NEVER update the git config."),
				prompt.ListItem("Return the PR URL when you're done, so the user can see it."),
			),
		).
		XMLTag("other-operations",
			prompt.Heading(3, "Other Operations"),
			prompt.List(
				prompt.ListItem("View comments on a Github PR: `gh api repos/foo/bar/pulls/123/comments`"),
			),
		)
}
