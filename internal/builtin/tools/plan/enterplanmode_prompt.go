package plan

import "github.com/peggco/pegg/internal/agent/prompt"

func enterplanmodeToolPromptBuilder() *prompt.Prompt {
	return prompt.New().
		Paragraph("Use this tool proactively when you're about to start a non-trivial implementation task. Getting user sign-off on your approach before writing code prevents wasted effort and ensures alignment. This tool transitions you into plan mode where you can explore the codebase and design an implementation approach for user approval.").
		Paragraph("If they explicitly mention wanting to create a plan ALWAYS call this tool first.").
		Heading(2, "When to Use This Tool:").
		Paragraph("**Prefer using EnterPlanMode** for implementation tasks unless they're simple. Use it when ANY of these conditions apply:").
		Raw(`1. **New Feature Implementation**: Adding meaningful new functionality
   - Example: "Add a logout button" - where should it go? What should happen on click?
   - Example: "Add form validation" - what rules? What error messages?

2. **Multiple Valid Approaches**: The task can be solved in several different ways
   - Example: "Add caching to the API" - could use Redis, in-memory, file-based, etc.
   - Example: "Improve performance" - many optimization strategies possible

3. **Code Modifications**: Changes that affect existing behavior or structure
   - Example: "Update the login flow" - what exactly should change?
   - Example: "Refactor this component" - what's the target architecture?

4. **Architectural Decisions**: The task requires choosing between patterns or technologies
   - Example: "Add real-time updates" - WebSockets vs SSE vs polling
   - Example: "Implement state management" - Redux vs Context vs custom solution

5. **Multi-File Changes**: The task will likely touch more than 2-3 files
   - Example: "Refactor the authentication system"
   - Example: "Add a new API endpoint with tests"

6. **Unclear Requirements**: You need to explore before understanding the full scope
   - Example: "Make the app faster" - need to profile and identify bottlenecks
   - Example: "Fix the bug in checkout" - need to investigate root cause

7. **User Preferences Matter**: The implementation could reasonably go multiple ways
   - If you would use askuserquestion to clarify the approach, use EnterPlanMode instead
   - Plan mode lets you explore first, then present options with context`).
		Heading(2, "When NOT to Use This Tool").
		Paragraph("Only skip EnterPlanMode for simple tasks:").
		List("Single-line or few-line fixes (typos, obvious bugs, small tweaks)",
			"Adding a single function with clear requirements",
			"Tasks where the user has given very specific, detailed instruction",
			"Pure research/exploration tasks").
		Heading(2, "Examples").
		Heading(3, "GOOD - Use EnterPlanMode:").
		Paragraph("User: 'Add user authentication to the app'").
		List("Requires architectural decisions (session vs JWT, where to store tokens, middleware structure)").
		Paragraph("User: 'Optimize the database queries'").
		List("Multiple approaches possible, need to profile first, significant impact").
		Paragraph("User: 'Implement dark mode'").
		List("Architectural decision on theme system, affects many components").
		Paragraph("User: 'Add a delete button to the user profile'").
		List("Seems simple but involves: where to place it, confirmation dialog, API call, error handling, state updates").
		Paragraph("User: 'Update the error handling in the API'").
		List("Affects multiple files, user should approve the approach").
		Heading(3, "BAD - Don't use EnterPlanMode:").
		Paragraph("User: 'Fix the typo in the README'").
		List("Straightforward, no planning needed").
		Paragraph("User: 'Add a console.log to debug this function'").
		List("Simple, obvious implementation").
		Paragraph("User: 'What files handle routing?'").
		List("Research task, not implementation planning").
		Heading(2, "Important Notes").
		List("This tool switches you into plan mode immediately - a system reminder enforces the read-only phase on every request",
			"If unsure whether to use it, err on the side of planning - it's better to get alignment upfront than to redo work",
			"Users appreciate being consulted before significant changes are made to their codebase")
}
