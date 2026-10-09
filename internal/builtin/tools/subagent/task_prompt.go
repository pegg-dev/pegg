package subagent

import "github.com/peggco/pegg/internal/agent/prompt"

func taskToolPromptBuilder(agents []string) *prompt.Prompt {
	agentsAny := make([]any, len(agents))
	for i, agent := range agents {
		agentsAny[i] = agent
	}
	return prompt.New().
		Paragraph("Launch a new agent to handle complex, multistep tasks autonomously.").
		Paragraph("When using this tool, you must specify a subagent_type parameter to select which agent type to use.").
		Heading(2, "When NOT to Use This Tool").
		Paragraph("Skip using this tool when:").
		List("If you want to read a specific file path, use the Read or Glob tool instead",
			"If you are searching for a specific class definition like 'class Foo', use the Grep tool instead",
			"If you are searching for code within a specific file or set of 2-3 files, use the Read tool instead",
			"If no available agent is a good fit for the task, use other tools directly").
		Heading(2, "Usage Notes").
		OrderedList("Launch multiple agents concurrently whenever possible — use a single message with multiple tool calls to maximize performance.",
			"Once you have delegated work to an agent, do not duplicate that work yourself. Continue with non-overlapping tasks, or wait for the result. For background tasks, you will be notified automatically when the result is ready.",
			"Each agent invocation starts with a fresh context unless you provide name to resume the same subagent session (which continues with its previous messages and tool outputs). When starting fresh, your prompt should contain a highly detailed task description for the agent to perform autonomously and you should specify exactly what information the agent should return back to you in its final and only message to you.",
			"The agent's outputs should generally be trusted.",
			"Clearly tell the agent whether you expect it to write code or just to do research (search, file reads, web fetches, etc.), since it is not aware of the user's intent. Tell it how to verify its work if possible (e.g., relevant test commands).",
			"If the agent description mentions that it should be used proactively, try your best to use it without the user having to ask first. Use your judgement.").
		Heading(2, "Examples").
		XMLTag("example",
			prompt.Paragraph("User: Add a dark mode toggle to the application settings. Create todos, implement it, and run the tests when done."),
			prompt.Paragraph("Assistant: I'll create a todo list, mark them in progress, and delegate the implementation."),
			prompt.Paragraph("*Creates todos and marks them in progress:*"),
			prompt.Code("json", "{\n  \"id\": \"todo-1\", \"title\": \"Create dark mode toggle component\", \"status\": \"in_progress\", \"priority\": \"high\"\n}\n{\"id\": \"todo-2\", \"title\": \"Add state management with context\", \"status\": \"pending\", \"priority\": \"high\", \"dependsOn\": [\"todo-1\"]}\n{\"id\": \"todo-3\", \"title\": \"Implement CSS styles for dark theme\", \"status\": \"pending\", \"priority\": \"medium\", \"dependsOn\": [\"todo-1\"]}\n{\"id\": \"todo-4\", \"title\": \"Run tests and verify build\", \"status\": \"pending\", \"priority\": \"high\", \"dependsOn\": [\"todo-2\", \"todo-3\"]}"),
			prompt.Paragraph("*Spawns a background developer subagent linked to all todos:*"),
			prompt.Code("json", "{\n  \"name\": \"dark-mode-implementation\",\n  \"subagent_type\": \"developer\",\n  \"prompt\": \"Add a dark mode toggle to the Settings page. Create a Toggle component with context-based state management. Implement CSS custom properties for the dark theme and update existing components. Run all tests after completing.\",\n  \"task_id\": [\"todo-1\", \"todo-2\", \"todo-3\", \"todo-4\"],\n  \"background\": true\n}"),
			prompt.Paragraph("Assistant: I've started a subagent to implement dark mode. I'll continue with other tasks."),
			prompt.Paragraph("*The subagent finishes and a system reminder arrives:*"),
			prompt.Code("xml", "<system-reminders>\n<system-reminder tag=\"subagent\" task=\"todo-1,todo-2,todo-3,todo-4\" agent=\"dark-mode-implementation\">\n  Subagent \"dark-mode-implementation\" finished.\n  Response:\n  Dark mode toggle implemented. Toggle component created, state management\n  added via ThemeContext, CSS variables applied across all components.\n  All 47 tests pass.\n</system-reminder>\n</system-reminders>"),
			prompt.Paragraph("*Assistant marks all todos as completed:*"),
			prompt.Code("json", "{\"id\": \"todo-1\", \"status\": \"completed\"}\n{\"id\": \"todo-2\", \"status\": \"completed\"}\n{\"id\": \"todo-3\", \"status\": \"completed\"}\n{\"id\": \"todo-4\", \"status\": \"completed\"}"),
			prompt.Paragraph("Assistant: Dark mode is fully implemented! All 47 tests pass. Let me update the todo list."),
			prompt.XMLTag("reasoning",
				prompt.Paragraph("The assistant used the subagent tool because:"),
				prompt.OrderedList("Adding dark mode is a multi-step feature requiring UI, state management, and styling changes",
					"Todos track progress and dependencies for the user",
					"Running in background allows the assistant to continue with other work",
					"The task_id links tie each subagent to specific todos"))).
		XMLTag("example",
			prompt.Paragraph("User: I want to refactor the authentication system to support OAuth2. Create a plan first, then execute."),
			prompt.Paragraph("Assistant: This is a non-trivial architectural change. Let me first delegate to the planner to create a plan and todo hierarchy."),
			prompt.Paragraph("*Spawns a planner subagent:*"),
			prompt.Code("json", "{\n  \"name\": \"plan-oauth2-refactor\",\n  \"subagent_type\": \"planner\",\n  \"prompt\": \"Design a plan to refactor the authentication system to support OAuth2 alongside the existing username/password auth. The current auth is in internal/auth/ with a UserStore interface, JWT middleware, and login handler. Read the existing code, then write the implementation plan to .pegg/plans/ and create a matching persistent todo hierarchy with dependsOn links.\",\n  \"background\": false\n}"),
			prompt.Paragraph("*The planner returns a plan and creates todos. The assistant reads the plan and reviews the todos:*"),
			prompt.Paragraph("*Uses todowrite to read the created hierarchy:*"),
			prompt.Code("json", "{}"),
			prompt.Paragraph("*The returned todos show:*"),
			prompt.Code("text", "todo-1  Design OAuth2 provider interface          completed  high\n  todo-1.1  Implement Google OAuth2 provider        completed  high\n  todo-1.2  Implement GitHub OAuth2 provider         completed  medium\ntodo-2  Add OAuth2 callback handler                   pending    high    [todo-1]\ntodo-3  Update login page with OAuth buttons          pending    medium  [todo-1]\ntodo-4  Write integration tests                       pending    high    [todo-2, todo-3]\ntodo-5  Update documentation                          pending    low     [todo-4]"),
			prompt.Paragraph("*Assistant dispatches developer subagents for independent tasks in parallel:*"),
			prompt.Code("json", "{\n  \"name\": \"implement-oauth-callback\",\n  \"subagent_type\": \"developer\",\n  \"prompt\": \"Implement the OAuth2 callback handler in internal/auth/oauth.go. The handler should exchange the authorization code for tokens, fetch user info from the provider, and create or link local users. Follow the interface defined in the plan at .pegg/plans/2026-09-13-oauth2-refactor.md. Run tests after.\",\n  \"task_id\": [\"todo-2\"],\n  \"background\": true\n}"),
			prompt.Code("json", "{\n  \"name\": \"implement-oauth-ui\",\n  \"subagent_type\": \"developer\",\n  \"prompt\": \"Update the login page in frontend/src/Login.tsx to add OAuth2 sign-in buttons for Google and GitHub. Use the OAuth2 provider interface from internal/auth/oauth.go. The callback URL should be /auth/callback/:provider.\",\n  \"task_id\": [\"todo-3\"],\n  \"background\": true\n}"),
			prompt.Paragraph("*Both subagents run in parallel. When they finish, system reminders arrive and the assistant marks todos completed, then dispatches the test subagent:*"),
			prompt.Code("json", "{\"id\": \"todo-2\", \"status\": \"completed\"}\n{\"id\": \"todo-3\", \"status\": \"completed\"}"),
			prompt.Code("json", "{\n  \"name\": \"oauth2-integration-tests\",\n  \"subagent_type\": \"developer\",\n  \"prompt\": \"Write integration tests for the OAuth2 flow. Test the callback handler with mock providers, verify user creation and linking, and test the login page OAuth buttons. Place tests in internal/auth/oauth_test.go and frontend/src/__tests__/Login.test.tsx.\",\n  \"task_id\": [\"todo-4\"],\n  \"background\": false\n}"),
			prompt.XMLTag("reasoning",
				prompt.Paragraph("The assistant used the planner-then-execute pattern because:"),
				prompt.OrderedList("OAuth2 refactoring is architecturally complex and needs a structured plan",
					"The planner reads the existing code and produces a grounded plan file with dependencies",
					"Independent tasks (callback handler and UI) can run in parallel",
					"The todo dependency graph ensures tests run only after implementation is complete"))).
		XMLTag("example",
			prompt.Paragraph("User: We need to migrate our REST API from v1 to v2. I want the explorer to research the current API surface first, then plan and execute."),
			prompt.Paragraph("Assistant: I'll start with an explorer to map the full API surface, then hand findings to the planner."),
			prompt.Paragraph("*Creates initial todo and spawns explorer:*"),
			prompt.Code("json", "{\"id\": \"todo-1\", \"title\": \"Research current API v1 endpoints and usage\", \"status\": \"in_progress\", \"priority\": \"high\"}"),
			prompt.Code("json", "{\n  \"name\": \"explore-api-v1\",\n  \"subagent_type\": \"explorer\",\n  \"prompt\": \"Map the complete REST API v1 surface. Find all route definitions, handler functions, request/response types, and middleware. Check for any API documentation or OpenAPI specs. Look for external consumers or SDK clients. Return a structured report with: list of all endpoints (method, path, handler file:line), request/response types, middleware applied, and any breaking change considerations.\",\n  \"task_id\": [\"todo-1\"],\n  \"background\": false\n}"),
			prompt.Paragraph("*Explorer completes with a detailed findings report. Assistant marks todo-1 done, creates plan todos, and dispatches planner:*"),
			prompt.Code("json", "{\"id\": \"todo-1\", \"status\": \"completed\"}"),
			prompt.Paragraph("*Creates the plan and execution todos based on explorer findings:*"),
			prompt.Code("json", "{\"id\": \"todo-2\", \"title\": \"Create API v2 migration plan\", \"status\": \"in_progress\", \"priority\": \"high\", \"dependsOn\": [\"todo-1\"]}"),
			prompt.Code("json", "{\n  \"name\": \"plan-api-v2-migration\",\n  \"subagent_type\": \"planner\",\n  \"prompt\": \"Based on the explorer's findings (23 endpoints across 8 handlers, 3 middleware layers), design a migration plan from REST API v1 to v2. The plan should: introduce versioned URL prefix /api/v2/, add response envelope wrapping, standardize error formats, and maintain backward compatibility during transition. Write the plan to .pegg/plans/ and create the persistent todo hierarchy.\",\n  \"task_id\": [\"todo-2\"],\n  \"background\": false\n}"),
			prompt.Paragraph("*Planner creates the plan and a todo hierarchy with 6 tasks across 3 phases. Assistant dispatches the first batch of independent developer subagents:*"),
			prompt.Code("json", "{\n  \"name\": \"implement-v2-router\",\n  \"subagent_type\": \"developer\",\n  \"prompt\": \"Implement the v2 router with versioned URL prefix /api/v2/ in internal/server/router.go. Add the response envelope wrapper middleware. Keep v1 routes functional during transition.\",\n  \"task_id\": [\"todo-3\"],\n  \"background\": true\n}"),
			prompt.Code("json", "{\n  \"name\": \"migrate-auth-handlers\",\n  \"subagent_type\": \"developer\",\n  \"prompt\": \"Migrate authentication endpoints (login, register, refresh) to v2 format in internal/server/handlers/auth.go. Wrap responses in the new envelope format. Update request/response types in internal/server/types/.\",\n  \"task_id\": [\"todo-4\"],\n  \"background\": true\n}"),
			prompt.Paragraph("*When both finish, the assistant dispatches the remaining handlers and finally the tests:*"),
			prompt.XMLTag("reasoning",
				prompt.Paragraph("The assistant used the explorer → planner → developer pipeline because:"),
				prompt.OrderedList("The API surface is too large to plan without first mapping what exists",
					"The explorer provides grounded findings so the planner works from real code, not assumptions",
					"The plan produces a dependency graph so independent handler migrations run in parallel",
					"Each developer subagent gets the exact files and types from the explorer's report"))).
		XMLTag("example",
			prompt.Paragraph("User: There's a performance issue in the database queries. Can you investigate and fix it?"),
			prompt.Paragraph("Assistant: Let me first research the issue with an explorer, then act on the findings."),
			prompt.Paragraph("*Spawns an explorer to investigate:*"),
			prompt.Code("json", "{\n  \"name\": \"investigate-db-performance\",\n  \"subagent_type\": \"explorer\",\n  \"prompt\": \"Investigate the database query performance issue. Check internal/db/ for slow queries, look for missing indexes, N+1 query patterns, and connection pool configuration. Check if there's a query logger or slow query log. Return: specific slow queries with file:line references, missing indexes, and N+1 patterns found.\",\n  \"background\": false\n}"),
			prompt.Paragraph("*Explorer reports: 3 N+1 queries in internal/db/user.go:45-82, missing index on orders.created_at, connection pool too small. Assistant creates todos and spawns developer:*"),
			prompt.Code("json", "{\"id\": \"todo-1\", \"title\": \"Fix N+1 queries in user repository\", \"status\": \"in_progress\", \"priority\": \"high\"}\n{\"id\": \"todo-2\", \"title\": \"Add missing index on orders.created_at\", \"status\": \"pending\", \"priority\": \"high\"}\n{\"id\": \"todo-3\", \"title\": \"Tune connection pool settings\", \"status\": \"pending\", \"priority\": \"medium\"}\n{\"id\": \"todo-4\", \"title\": \"Write performance regression tests\", \"status\": \"pending\", \"priority\": \"high\", \"dependsOn\": [\"todo-1\", \"todo-2\", \"todo-3\"]}"),
			prompt.Code("json", "{\n  \"name\": \"fix-db-performance\",\n  \"subagent_type\": \"developer\",\n  \"prompt\": \"Fix the database performance issues found by the explorer:\\n1. Replace N+1 queries in internal/db/user.go:45-82 with JOINs or batch loading\\n2. Add a migration for the missing index on orders.created_at in migrations/\\n3. Increase the connection pool max open connections from 5 to 25 in internal/db/db.go\\nRun benchmarks before and after to verify improvement.\",\n  \"task_id\": [\"todo-1\", \"todo-2\", \"todo-3\"],\n  \"background\": true\n}"),
			prompt.Paragraph("*Developer finishes, system reminder arrives. Assistant marks todos 1-3 done, then dispatches the test subagent:*"),
			prompt.Code("json", "{\"id\": \"todo-1\", \"status\": \"completed\"}\n{\"id\": \"todo-2\", \"status\": \"completed\"}\n{\"id\": \"todo-3\", \"status\": \"completed\"}"),
			prompt.Code("json", "{\n  \"name\": \"db-performance-tests\",\n  \"subagent_type\": \"developer\",\n  \"prompt\": \"Write performance regression tests to prevent the N+1 and indexing issues from recurring. Create benchmarks in internal/db/bench_test.go that measure query time for the user repository operations. Assert that the fixes maintain sub-10ms query times.\",\n  \"task_id\": [\"todo-4\"],\n  \"background\": false\n}"),
			prompt.XMLTag("reasoning",
				prompt.Paragraph("The assistant used the explorer → developer pipeline because:"),
				prompt.OrderedList("Performance issues require investigation before fixing — the explorer maps the exact problems",
					"Fixing without research risks addressing symptoms rather than root causes",
					"Todo dependencies ensure tests run only after all fixes are in place",
					"The explorer's file:line references let the developer target exact code locations"))).
		XMLTag("example",
			prompt.Paragraph("User: Help me rename the function getCwd to getCurrentWorkingDirectory across my project"),
			prompt.Paragraph("Assistant: Let me first search to understand the scope."),
			prompt.Paragraph("*Uses grep to find 15 instances across 8 files*"),
			prompt.Paragraph("Assistant: I found 15 instances across 8 files. Let me delegate this to a subagent."),
			prompt.Code("json", "{\n  \"name\": \"rename-getCwd\",\n  \"subagent_type\": \"developer\",\n  \"prompt\": \"Rename getCwd to getCurrentWorkingDirectory in all 8 files: internal/fs/walk.go, internal/fs/path.go, internal/cmd/root.go, internal/cmd/cwd.go, internal/tool/path.go, internal/shell/exec.go, internal/vfs/vfs.go, pkg/sdk/files.go. Ensure all call sites are updated. Run the build after.\",\n  \"task_id\": [\"todo-1\"],\n  \"background\": false\n}"),
			prompt.Paragraph("*The subagent finishes and returns the result directly since it's foreground.*"),
			prompt.XMLTag("reasoning",
				prompt.Paragraph("The assistant used foreground mode here because the task is straightforward and it needs the result before proceeding."))).
		XMLTag("example",
			prompt.Paragraph("User: There's a bug in the payment processing module. Can you fix it?"),
			prompt.Paragraph("Assistant: I'll investigate the payment module. Let me delegate to a subagent."),
			prompt.Code("json", "{\n  \"name\": \"payment-bug-fix\",\n  \"subagent_type\": \"developer\",\n  \"prompt\": \"Investigate and fix the bug in the payment processing module. Write tests to prevent regression.\",\n  \"task_id\": [\"todo-5\"],\n  \"background\": true\n}"),
			prompt.Paragraph("*The subagent encounters the same issue that was previously fixed in this area. Instead of starting from scratch, you resume it:*"),
			prompt.Code("json", "{\n  \"name\": \"payment-bug-fix\",\n  \"subagent_type\": \"developer\",\n  \"prompt\": \"The previous fix didn't fully address the issue. The root cause is in the currency conversion logic — check the rounding behavior for international transactions.\",\n  \"background\": false\n}"),
			prompt.Paragraph("The subagent resumes with its existing context and conversation history, allowing it to understand what was already tried and provide a more accurate fix."),
			prompt.XMLTag("reasoning",
				prompt.Paragraph("The assistant reused the same subagent name because:"),
				prompt.OrderedList("The subagent had already explored the payment module and built context",
					"Starting fresh would waste time re-exploring the same code",
					"Resuming preserves the conversation history including what was already tried",
					"The follow-up instruction builds on the previous investigation"))).
		XMLTag("example",
			prompt.Paragraph("User: Can you add a comment to the calculateTotal function?"),
			prompt.Paragraph("Assistant: Sure, let me add a comment."),
			prompt.Paragraph("*Uses the Edit tool directly to add a comment*"),
			prompt.XMLTag("reasoning",
				prompt.Paragraph("The assistant did not use the subagent tool because this is a single, straightforward task that can be completed in one step. There's no need to spawn a subagent for such a simple operation."))).
		Add(prompt.IfFn(
			func(vars prompt.Vars) bool { return len(agents) > 0 },
			prompt.Paragraph("Available Agents:"),
			prompt.OrderedList(agentsAny...),
		))
}
