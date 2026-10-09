package todo

import "github.com/peggco/pegg/internal/agent/prompt"

func todowriteToolPromptBuilder() *prompt.Prompt {
	return prompt.New().
		Paragraph("Use this tool to create and manage a structured task list for your current coding session. This helps you track progress, organize complex tasks, and demonstrate thoroughness to the user.").
		Paragraph("It also helps the user understand the progress of the task and overall progress of their requests.").
		Heading(2, "When to Use This Tool").
		Paragraph("Use this tool proactively in these scenarios:").
		OrderedList("Complex multi-step tasks - When a task requires 3 or more distinct steps or actions",
			"Non-trivial and complex tasks - Tasks that require careful planning or multiple operations",
			"User explicitly requests todo list - When the user directly asks you to use the todo list",
			"User provides multiple tasks - When users provide a list of things to be done (numbered or comma-separated)",
			"After receiving new instructions - Immediately capture user requirements as todos. Feel free to edit the todo list based on new information.",
			"After completing a task - Mark it complete and add any new follow-up tasks",
			"When you start working on a new task, mark the todo as in_progress. Ideally you should only have one todo as in_progress at a time. Complete existing tasks before starting new ones.").
		Heading(2, "When NOT to Use This Tool").
		Paragraph("Skip using this tool when:").
		OrderedList("There is only a single, straightforward task",
			"The task is trivial and tracking it provides no organizational benefit",
			"The task can be completed in less than 3 trivial steps",
			"The task is purely conversational or informational").
		Paragraph("NOTE that you should not use this tool if there is only one trivial task to do. In this case you are better off just doing the task directly.").
		Heading(2, "Examples of When to Use the Todo List").
		XMLTag("example",
			prompt.Paragraph("User: I want to add a dark mode toggle to the application settings. Make sure you run the tests and build when you're done!"),
			prompt.Paragraph("Assistant: I'll help add a dark mode toggle to your application settings. Let me create a todo list to track this implementation."),
			prompt.Paragraph("*Creates todo list with the following items:*"),
			prompt.OrderedList("Create dark mode toggle component in Settings page",
				"Add dark mode state management (context/store)",
				"Implement CSS-in-JS styles for dark theme",
				"Update existing components to support theme switching",
				"Run tests and build process, addressing any failures or errors that occur"),
			prompt.Paragraph("*Begins working on the first task*"),
			prompt.XMLTag("reasoning",
				prompt.Paragraph("The assistant used the todo list because:"),
				prompt.OrderedList("Adding dark mode is a multi-step feature requiring UI, state management, and styling changes",
					"The user explicitly requested tests and build be run afterward",
					"The assistant inferred that tests and build need to pass by adding 'Ensure tests and build succeed' as the final task"))).
		XMLTag("example",
			prompt.Paragraph("User: Help me rename the function getCwd to getCurrentWorkingDirectory across my project"),
			prompt.Paragraph("Assistant: Let me first search through your codebase to find all occurrences of 'getCwd'."),
			prompt.Paragraph("*Uses grep or search tools to locate all instances of getCwd in the codebase*"),
			prompt.Paragraph("Assistant: I've found 15 instances of getCwd across 8 different files. Let me create a todo list to track these changes."),
			prompt.Paragraph("*Creates todo list with specific items for each file that needs updating*"),
			prompt.Paragraph("*Begins working on the first task*"),
			prompt.XMLTag("reasoning",
				prompt.Paragraph("The assistant used the todo list because:"),
				prompt.OrderedList("First, the assistant searched to understand the scope of the task",
					"Upon finding multiple occurrences across different files, it determined this was a complex task with multiple steps",
					"The todo list helps ensure every instance is tracked and updated systematically",
					"This approach prevents missing any occurrences and maintains code consistency"))).
		XMLTag("example",
			prompt.Paragraph("User: I need to implement these features for my e-commerce site: user registration, product catalog, shopping cart, and checkout flow."),
			prompt.Paragraph("Assistant: I'll help implement these features. First, let's add all the features to the todo list."),
			prompt.Paragraph("*Creates a todo list breaking down each feature into specific tasks based on the project architecture*"),
			prompt.Paragraph("Assistant: Let's start with user registration. This will involve creating database models, API endpoints, and frontend forms that integrate with your existing authentication system."),
			prompt.Paragraph("*Begins working on the first task*"),
			prompt.XMLTag("reasoning",
				prompt.Paragraph("The assistant used the todo list because:"),
				prompt.OrderedList("The user provided multiple complex features to implement in a comma separated list",
					"The todo list helps organize these large features into manageable tasks",
					"This approach allows for tracking progress across the entire implementation"))).
		XMLTag("example",
			prompt.Paragraph("User: Can you help optimize my React application? It's rendering slowly and has performance issues."),
			prompt.Paragraph("Assistant: I'll help optimize your React application. First, let me examine your codebase to identify potential performance bottlenecks."),
			prompt.Paragraph("*Reviews component structure, render patterns, state management, and data fetching*"),
			prompt.Paragraph("Assistant: After analyzing your codebase, I've identified several performance issues. Let me create a todo list to track our optimization efforts."),
			prompt.Paragraph("*Creates todo list with items like: 1) Implement memoization for expensive calculations in ProductList, 2) Add virtualization for long lists in Dashboard, 3) Optimize image loading in Gallery component, 4) Fix state update loops in ShoppingCart, 5) Review bundle size and implement code splitting*"),
			prompt.Paragraph("Let's start by implementing memoization for the expensive calculations in your ProductList component."),
			prompt.Paragraph("*Begins working on the first task*"),
			prompt.XMLTag("reasoning",
				prompt.Paragraph("The assistant used the todo list because:"),
				prompt.OrderedList("First, the assistant examined the codebase to identify specific performance issues",
					"Based on this analysis, it identified multiple optimization opportunities",
					"Performance optimization is a non-trivial task requiring multiple steps",
					"The todo list helps methodically track improvements across different components",
					"This systematic approach ensures all performance bottlenecks are addressed"))).
		Heading(2, "Examples of When NOT to Use the Todo List").
		XMLTag("example",
			prompt.Paragraph("User: How do I print 'Hello World' in Python?"),
			prompt.Paragraph("Assistant: In Python, you can print \"Hello World\" with this simple code:"),
			prompt.Code("python", "print(\"Hello World\")"),
			prompt.Paragraph("This will output the text \"Hello World\" to the console when executed."),
			prompt.XMLTag("reasoning",
				prompt.Paragraph("The assistant did not use the todo list because this is a single, straightforward task that can be completed in one step. There's no need to track multiple tasks or steps for such a straightforward request."))).
		XMLTag("example",
			prompt.Paragraph("User: What does the git status command do?"),
			prompt.Paragraph("Assistant: The git status command shows the current state of your working directory and staging area. It displays which changes have been staged, which haven't, and which files aren't being tracked by Git. This is a fundamental command for understanding the current state of your repository before making commits."),
			prompt.XMLTag("reasoning",
				prompt.Paragraph("The assistant did not use the todo list because this is an informational request with no actual coding task to complete. The user is simply asking for an explanation, not for the assistant to perform multiple steps or tasks."))).
		XMLTag("example",
			prompt.Paragraph("User: Can you add a comment to the calculateTotal function to explain what it does?"),
			prompt.Paragraph("Assistant: Sure, let me add a comment to the calculateTotal function to explain what it does."),
			prompt.Paragraph("* Uses the Edit tool to add a comment to the calculateTotal function *"),
			prompt.XMLTag("reasoning",
				prompt.Paragraph("The assistant did not use the todo list because this is a single, straightforward task confined to one location in the code. Adding a comment doesn't require tracking multiple steps or systematic organization."))).
		XMLTag("example",
			prompt.Paragraph("User: Run npm install for me and tell me what happens."),
			prompt.Paragraph("Assistant: I'll run the npm install command for you."),
			prompt.Paragraph("Executes: npm install*"),
			prompt.Paragraph("The command completed successfully. Here's the output:"),
			prompt.Paragraph("[Output of npm install command]"),
			prompt.Paragraph("All dependencies have been installed according to your package.json file."),
			prompt.XMLTag("reasoning",
				prompt.Paragraph("The assistant did not use the todo list because this is a single command execution with immediate results. There are no multiple steps to track or organize, making the todo list unnecessary for this straightforward task."))).
		Heading(2, "Task States and Management").
		OrderedList("**Task States**: Use these states to track progress:",
			"pending: Task not yet started",
			"in_progress: Currently working on (limit to ONE task at a time)",
			"completed: Task finished successfully",
			"cancelled: Task no longer needed").
		Paragraph("2. **Task Management**:").
		OrderedList("Update task status in real-time as you work",
			"Mark tasks complete IMMEDIATELY after finishing (don't batch completions)",
			"Only have ONE task in_progress at any time",
			"Complete current tasks before starting new ones",
			"Cancel tasks that become irrelevant").
		Paragraph("3. **Task Breakdown**:").
		OrderedList("Create specific, actionable items",
			"Break complex tasks into smaller, manageable steps",
			"Use clear, descriptive task names").
		Paragraph("When in doubt, use this tool. Being proactive with task management demonstrates attentiveness and ensures you complete all requirements successfully.")
}
