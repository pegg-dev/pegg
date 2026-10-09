package plan

import (
	"context"
	"fmt"

	"github.com/peggco/pegg/internal/agent"
	"github.com/peggco/pegg/internal/agent/prompt"
	"github.com/peggco/pegg/internal/agent/reminder"
	"github.com/peggco/pegg/internal/agent/tool"
	"github.com/peggco/pegg/internal/agent/tools"
	"github.com/peggco/pegg/internal/vfs"
)

func PlanTools(fs *vfs.VFS) {
	tools.Register(enterplanmodeTool(fs))
	tools.Register(exitplanmodeTool(fs))
}

func enterplanmodeToolPrompt() (string, error) {
	sys, err := enterplanmodeToolPromptBuilder().
		Build(prompt.FormatMarkdown)
	if err != nil {
		return "", err
	}
	return sys, nil
}

func enterplanmodeTool(fs *vfs.VFS) tool.Tool {
	prompt, err := enterplanmodeToolPrompt()
	if err != nil {
		panic(fmt.Sprintf("failed to generate enterplanmode tool prompt: %v", err))
	}

	return tool.NewSpec(
		"enterplanmode",
		prompt,
		map[string]any{
			"type":       "object",
			"properties": map[string]any{},
			"required":   []string{},
		},
		func(ctx context.Context, args string) (string, error) {
			parent := agent.FromContext(ctx)
			if parent == nil {
				return "", fmt.Errorf("enterplanmode: no parent agent in context")
			}
			parent.AttachReminder(PlanModeReminder())
			if fs != nil {
				if err := fs.SetWriteOnly(vfs.PlansDir); err != nil {
					return "", fmt.Errorf("enterplanmode: restrict writes: %w", err)
				}
			}
			return "Plan mode enabled. You are now in READ-ONLY planning mode and must not make any edits or run any non-readonly tools until the user approves a plan. Call exitplanmode when the plan is ready for approval.", nil
		},
	)
}

func PlanModeReminder() reminder.Reminder {
	content, err := prompt.New().
		Heading(1, "Plan Mode - System Reminder").
		Paragraph("Plan mode is active. The user indicated that they do not want you to execute yet -- you MUST NOT make any edits (with the exception of the plan file mentioned below), run any non-readonly tools (including changing configs or making commits), or otherwise make any changes to the system. This supersedes any other instructions you have received.").
		Hr(3).
		Heading(2, "Plan File Info").
		Paragraph("No plan file exists yet. You should create your plan at `.pegg/plans/plan.md` using the Write tool.").
		Paragraph("You should build your plan incrementally by writing to or editing this file. NOTE that this is the only file you are allowed to edit - other than this you are only allowed to take READ-ONLY actions.").
		Paragraph("**Plan File Guidelines:** The plan file should contain only your final recommended approach, not all alternatives considered. Keep it comprehensive yet concise - detailed enough to execute effectively while avoiding unnecessary verbosity.").
		Hr(3).
		Heading(2, "Enhanced Planning Workflow").
		Heading(3, "Phase 1: Initial Understanding").
		Paragraph("**Goal:** Gain a comprehensive understanding of the user's request by reading through code and asking them questions. Critical: In this phase you should only use the Explore subagent type.").
		OrderedList(
			prompt.ListItem("Understand the user's request thoroughly"),
			prompt.ListItem("**Launch up to 3 Explore agents IN PARALLEL** (single message, multiple tool calls) to efficiently explore the codebase. Each agent can focus on different aspects:",
				prompt.List(
					"Example: One agent searches for existing implementations, another explores related components, a third investigates testing patterns",
					"Provide each agent with a specific search focus or area to explore",
					"Quality over quantity - 3 agents maximum, but you should try to use the minimum number of agents necessary (usually just 1)",
					"Use 1 agent when: the task is isolated to known files, the user provided specific file paths, or you're making a small targeted change. Use multiple agents when: the scope is uncertain, multiple areas of the codebase are involved, or you need to understand existing patterns before planning.",
					"Take into account any context you already have from the user's request or from the conversation so far when deciding how many agents to launch",
				),
			),
			prompt.ListItem("Use AskUserQuestion tool to clarify ambiguities in the user request up front."),
		).
		Heading(3, "Phase 2: Planning").
		Paragraph("**Goal:** Come up with an approach to solve the problem identified in phase 1 by launching a Plan subagent.").
		Paragraph("In the agent prompt:").
		List(
			"Provide any background context that may help the agent with their task without prescribing the exact design itself",
			"Request a detailed plan",
		).
		Heading(3, "Phase 3: Synthesis").
		Paragraph("**Goal:** Synthesize the perspectives from Phase 2, and ensure that it aligns with the user's intentions by asking them questions.").
		OrderedList(
			prompt.ListItem("Collect all agent responses"),
			prompt.ListItem("Each agent will return an implementation plan along with a list of critical files that should be read. You should keep these in mind and read them before you start implementing the plan"),
			prompt.ListItem("Use AskUserQuestion to ask the users questions about trade offs."),
		).
		Heading(3, "Phase 4: Final Plan").
		Paragraph("Once you have all the information you need, ensure that the plan file has been updated with your synthesized recommendation including:").
		List(
			"Recommended approach with rationale",
			"Key insights from different perspectives",
			"Critical files that need modification",
		).
		Heading(3, "Phase 5: Call ExitPlanMode").
		Paragraph("At the very end of your turn, once you have asked the user questions and are happy with your final plan file - you should always call ExitPlanMode to indicate to the user that you are done planning.").
		Paragraph("This is critical - your turn should only end with either asking the user a question or calling ExitPlanMode. Do not stop unless it's for these 2 reasons.").
		Hr(3).
		Paragraph("**NOTE:** At any point in time through this workflow you should feel free to ask the user questions or clarifications. Don't make large assumptions about user intent. The goal is to present a well researched plan to the user, and tie any loose ends before implementation begins.").
		Build(prompt.FormatMarkdown)
	if err != nil {
		content = "CRITICAL: Plan mode ACTIVE - you are in READ-ONLY phase. You may ONLY observe, analyze, and plan. Any modification attempt is a critical violation."
	}
	return reminder.New("plan_mode", content, "mode", "read_only")
}
