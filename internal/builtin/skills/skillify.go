package skills

import (
	"github.com/peggco/pegg/internal/agent/prompt"
)

func SkillifySkill() *prompt.Prompt {
	return prompt.New().
		Hr(3).
		Paragraph("name: skillify").
		Paragraph("description: Turns the current session into a reusable skill by analyzing the repeatable process and interviewing the user, then writing a SKILL.md.").
		Paragraph("when_to_use: Use when the user wants to capture the current session or a repeatable workflow as a reusable skill — for example \"skillify this\", \"turn this into a skill\", \"save this workflow as a skill\", \"make this repeatable\", or \"capture this process as a skill\".").
		Paragraph("allowed-tools: read write edit glob grep list askuserquestion").
		Paragraph("context: inline").
		Hr(3).
		Heading(1, "Skillify").
		Paragraph("You are capturing this session's repeatable process as a reusable skill.").
		Paragraph("Review the conversation above — it is your source material. Pay particular attention to the user's messages (how they steered and corrected the process) and to the tools and commands that were actually used.").
		Heading(2, "Step 1: Analyze the session").
		Paragraph("Before asking any questions, analyze the session to identify:").
		List(
			"What repeatable process was performed.",
			"What the inputs and parameters were.",
			"The distinct steps, in order.",
			"The success artifact or criteria for each step (e.g. not just \"writing code\", but \"an open PR with CI fully passing\").",
			"Where the user corrected or steered you.",
			"What tools and permissions were needed.",
			"What subagents were used (via the `task` tool).",
			"The goals and the success artifacts.",
		).
		Heading(2, "Step 2: Interview the user").
		Paragraph("Use the `askuserquestion` tool for ALL questions — never ask questions via plain text. Iterate as many rounds as needed until the user is happy. The user always has a freeform \"Other\" option to type edits or feedback, so do NOT add your own \"Needs tweaking\" or \"I'll provide edits\" option — just offer the substantive choices.").
		Heading(3, "Round 1: High-level confirmation").
		List(
			"Suggest a name and a one-line description for the skill based on your analysis. Ask the user to confirm or rename.",
			"Suggest the high-level goal(s) and the specific success criteria for the skill.",
		).
		Heading(3, "Round 2: More details").
		List(
			"Present the high-level steps you identified as a numbered list. Tell the user you will dig into the detail in the next round.",
			"If the skill will require arguments, suggest them based on what you observed. Make sure you understand what someone would need to provide.",
			"If it is not clear, ask whether the skill should run inline (in the current conversation) or forked (as a subagent with its own context). Forked is better for self-contained tasks that do not need mid-process user input; inline is better when the user wants to steer mid-process.",
			prompt.ListItem("Ask where the skill should be saved. Suggest a default based on context (repo-specific workflows -> repo, cross-repo personal workflows -> user):",
				prompt.List(
					"This repo — `<project>/.pegg/skills/<name>/SKILL.md` — for workflows specific to this project.",
					"Personal — `~/.pegg/skills/<name>/SKILL.md` — follows you across all repos.",
				)),
		).
		Heading(3, "Round 3: Break down each step").
		Paragraph("For each major step, if it is not glaringly obvious, ask:").
		List(
			"What does this step produce that later steps need? (data, artifacts, IDs)",
			"What proves this step succeeded and that we can move on?",
			"Should the user confirm before proceeding? (especially for irreversible actions like merging, sending messages, or destructive operations)",
			"Are any steps independent and could run in parallel? (e.g. posting to Slack while monitoring CI)",
			"How should the skill be executed? (e.g. always use the `task` tool for a code review, or launch subagents for concurrent steps)",
			"What are the hard constraints or hard preferences — things that must or must not happen?",
		).
		Paragraph("You may do multiple rounds here, one per step, especially if there are more than 3 steps or many clarifications. Iterate as much as needed.").
		Paragraph("Pay special attention to where the user corrected you during the session — those corrections often become the rules of a step.").
		Heading(3, "Round 4: Final questions").
		List(
			"Confirm when this skill should be invoked, and suggest or confirm trigger phrases (e.g. for a cherry-pick workflow: \"Use when the user wants to cherry-pick a PR to a release branch. Examples: 'cherry-pick to release', 'CP this PR', 'hotfix'.\").",
			"Ask for any other gotchas or things to watch out for, if it is still unclear.",
		).
		Paragraph("Stop interviewing once you have enough information. Do not over-ask for simple processes.").
		Heading(2, "Step 3: Write the SKILL.md").
		Paragraph("Create the skill directory and file at the location the user chose in Round 2, using the `write` tool. Use this format:").
		Code("markdown", skillifyTemplate).
		Paragraph("Per-step annotations (all optional except success criteria):").
		List(
			"Success criteria (REQUIRED on every step): what the user expects from the step and when to move on.",
			"Execution: Direct (default), subagent (use the `task` tool), or [human] (the user does it). Only specify when it is not Direct.",
			"Artifacts: data this step produces that later steps need (e.g. PR number, commit SHA).",
			"Human checkpoint: when to pause and ask the user before proceeding. Include for irreversible actions (merging, sending messages), error judgment (merge conflicts), or output review.",
			"Rules: hard rules for the workflow. The user's corrections during the reference session are especially useful here.",
		).
		Paragraph("Step structure tips:").
		List(
			"Steps that can run concurrently use sub-numbers: 3a, 3b.",
			"Steps requiring the user to act get `[human]` in the title.",
			"Keep simple skills simple — a 2-step skill does not need annotations on every step.",
		).
		Paragraph("Frontmatter rules:").
		List(
			"allowed-tools: the minimum permissions needed, as space-separated tool names (e.g. `read write edit bash`).",
			"context: only set `context: fork` for self-contained skills that do not need mid-process user input.",
			"when_to_use is critical — it tells Pegg when to auto-invoke. Start with \"Use when...\" and include trigger phrases.",
			"arguments and argument-hint: only include if the skill takes parameters; reference them in the body with `$` placeholders (e.g. `$topic`).",
		).
		Heading(2, "Step 4: Confirm and save").
		Paragraph("Before writing the file, output the complete SKILL.md content as a fenced `markdown` code block in your response so the user can review it with syntax highlighting. Then ask for confirmation with the `askuserquestion` tool — a simple \"Does this SKILL.md look good to save?\" — and keep the question concise.").
		Paragraph("After writing, tell the user:").
		List(
			"Where the skill was saved.",
			"How to invoke it: `/<skill-name> [arguments]`.",
			"That they can edit the SKILL.md directly to refine it.",
		)
}

const skillifyTemplate = `---
name: {{skill-name}}
description: {{one-line description}}
allowed-tools: {{space-separated tool names observed in the session, e.g. read write edit bash}}
when_to_use: {{when Pegg should automatically invoke this skill, including trigger phrases and example user messages}}
argument-hint: "{{hint showing argument placeholders}}"
arguments:
  - {{argument name}}
context: {{inline or fork -- omit this line for inline}}
---

# {{Skill Title}}
{{Short description of the skill.}}

## Inputs
- $arg_name: {{Description of this input}}

## Goal
{{Clearly stated goal. Best if there are clearly defined artifacts or criteria for completion.}}

## Steps

### 1. {{Step Name}}
{{What to do in this step. Be specific and actionable. Include commands when appropriate.}}

**Success criteria**: {{What shows this step is done and we can move on. Can be a list.}}

...`
