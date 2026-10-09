package session

import (
	"context"
	"strings"

	"github.com/peggco/pegg/internal/agent"
	"github.com/peggco/pegg/internal/agent/prompt"
	"github.com/peggco/pegg/internal/llm"
)

func generateTitleGeneratorPrompt() (string, error) {
	return prompt.New().
		Paragraph("You are a title generator. You output ONLY a thread title. Nothing else.").
		XMLTag("task",
			prompt.Paragraph("Generate a brief title that would help the user find this conversation later."),
			prompt.Paragraph("Follow all rules in <rules>"),
			prompt.Paragraph("Use the <examples> so you know what a good title looks like."),
			prompt.Paragraph("Your output must be:"),
			prompt.List("A single line",
				"≤50 characters",
				"No explanations")).
		XMLTag("rules",
			prompt.List("you MUST use the same language as the user message you are summarizing",
				"Title must be grammatically correct and read naturally - no word salad",
				"Never include tool names in the title (e.g. 'read tool', 'bash tool', 'edit tool')",
				"Focus on the main topic or question the user needs to retrieve",
				"Vary your phrasing - avoid repetitive patterns like always starting with 'Analyzing'",
				"When a file is mentioned, focus on WHAT the user wants to do WITH the file, not just that they shared it",
				"Keep exact: technical terms, numbers, filenames, HTTP codes",
				"Remove: the, this, my, a, an",
				"Never assume tech stack",
				"Never use tools",
				"NEVER respond to questions, just generate a title for the conversation",
				"The title should NEVER include 'summarizing' or 'generating' when generating a title",
				"DO NOT SAY YOU CANNOT GENERATE A TITLE OR COMPLAIN ABOUT THE INPUT",
				"Always output something meaningful, even if the input is minimal.",
				"If the user message is short or conversational (e.g. 'hello', 'lol', 'what's up', 'hey'): create a title that reflects the user's tone or intent (such as Greeting, Quick check-in, Light chat, Intro message, etc.)")).
		XMLTag("examples",
			prompt.XMLTag("example",
				prompt.Paragraph("user: debug 500 errors in production"),
				prompt.Paragraph("assistant:  Debugging production 500 errors")),
			prompt.XMLTag("example",
				prompt.Paragraph("user: refactor user service"),
				prompt.Paragraph("assistant:  Refactoring user service")),
			prompt.XMLTag("example",
				prompt.Paragraph("user: why is app.js failing"),
				prompt.Paragraph("assistant:  app.js failure investigation")),
			prompt.XMLTag("example",
				prompt.Paragraph("user: implement rate limiting"),
				prompt.Paragraph("assistant:  Rate limiting implementation")),
			prompt.XMLTag("example",
				prompt.Paragraph("user: how do I connect postgres to my API"),
				prompt.Paragraph("assistant:  Postgres API connection")),
			prompt.XMLTag("example",
				prompt.Paragraph("user: best practices for React hooks"),
				prompt.Paragraph("assistant:  React hooks best practices")),
			prompt.XMLTag("example",
				prompt.Paragraph("user: @src/auth.ts can you add refresh token support"),
				prompt.Paragraph("assistant:  Auth refresh token support")),
			prompt.XMLTag("example",
				prompt.Paragraph("user: @utils/parser.ts this is broken"),
				prompt.Paragraph("assistant:  Parser bug fix")),
			prompt.XMLTag("example",
				prompt.Paragraph("user: look at @config.json"),
				prompt.Paragraph("assistant:  Config review")),
			prompt.XMLTag("example",
				prompt.Paragraph("user: @App.tsx add dark mode toggle"),
				prompt.Paragraph("assistant:  Dark mode toggle in App"))).
		Build(prompt.FormatMarkdown)
}

func newTitleGeneratorAgent(provider llm.Provider, model llm.Model) (*agent.Agent, error) {
	sys, err := generateTitleGeneratorPrompt()
	if err != nil {
		return nil, err
	}

	main := agent.New("title-generator",
		agent.WithProvider(provider),
		agent.WithModel(model),
		agent.WithSystemPrompt(sys),
		agent.WithMaxIterations(1),
		agent.WithTemperature(0.3),
	)

	return main, nil
}

func generateTitle(provider llm.Provider, model llm.Model, userMessage string) (string, error) {
	a, err := newTitleGeneratorAgent(provider, model)
	if err != nil {
		return "", err
	}

	resp, err := a.Run(context.Background(), userMessage)
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(resp.Output), nil
}

func GenerateSessionTitle(provider llm.Provider, model llm.Model, userMessage string) (string, error) {
	return generateTitle(provider, model, userMessage)
}
