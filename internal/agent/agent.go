package agent

import (
	"context"
	"fmt"
	"sync"

	"github.com/google/uuid"
	"github.com/peggco/pegg/internal/agent/middleware"
	"github.com/peggco/pegg/internal/agent/middlewares"
	"github.com/peggco/pegg/internal/agent/reminder"
	"github.com/peggco/pegg/internal/agent/tool"
	"github.com/peggco/pegg/internal/agent/tools"
	"github.com/peggco/pegg/internal/core/event"
	"github.com/peggco/pegg/internal/core/logger"
	"github.com/peggco/pegg/internal/llm"
)

const DefaultMaxIterations = 150

type Option func(*Agent)

type Agent struct {
	ID              string
	Name            string
	Description     string
	ParentAgentID   string
	RouterManaged   bool
	DisplayName     string
	Model           llm.Model
	Provider        llm.Provider
	SystemPrompt    string
	SystemPromptFn  func(providerID, modelID string) string
	Temperature     float64
	TopP            float64
	MaxTokens       int
	MaxIterations   int
	Attachments     []llm.Attachment
	Tools           *tool.Registry
	ToolNames       []string
	ReasoningEffort string

	structuredName   string
	structuredSchema any

	chain           *middleware.Chain
	middlewareNames []string
	namesResolved   bool
	Bus             event.Bus
	log             *logger.Logger

	pendingNotifications []reminder.Reminder
	attachedReminder     *reminder.Reminder
	notificationsMu      sync.Mutex
}

func New(name string, opts ...Option) *Agent {
	if name == "" {
		name = "agent"
	}
	a := &Agent{
		ID:            uuid.NewString(),
		Name:          name,
		Tools:         tool.NewRegistry(),
		MaxIterations: DefaultMaxIterations,
		chain:         middleware.NewChain(),
	}
	for _, opt := range opts {
		opt(a)
	}
	return a
}

func (a *Agent) Clone(name string) *Agent {
	if name == "" {
		name = a.Name
	}
	c := New(name,
		WithDescription(a.Description),
		WithModel(a.Model),
		WithProvider(a.Provider),
		WithSystemPrompt(a.SystemPrompt),
		WithSystemPromptFn(a.SystemPromptFn),
		WithTemperature(a.Temperature),
		WithTopP(a.TopP),
		WithMaxTokens(a.MaxTokens),
		WithMaxIterations(a.MaxIterations),
		WithReasoningEffort(a.ReasoningEffort),
		WithBus(a.Bus),
	)
	c.log = a.log
	c.RouterManaged = a.RouterManaged
	c.chain = a.chain.Clone()
	for _, t := range a.Tools.List() {
		_ = c.Tools.Register(t)
	}
	return c
}

func WithDescription(desc string) Option {
	return func(a *Agent) { a.Description = desc }
}

func WithModel(model llm.Model) Option {
	return func(a *Agent) { a.Model = model }
}

func WithProvider(p llm.Provider) Option {
	return func(a *Agent) { a.Provider = p }
}

func WithSystemPrompt(prompt string) Option {
	return func(a *Agent) { a.SystemPrompt = prompt }
}

func WithSystemPromptFn(fn func(providerID, modelID string) string) Option {
	return func(a *Agent) { a.SystemPromptFn = fn }
}

func (a *Agent) SetModelProvider(model llm.Model, provider llm.Provider) {
	a.Model = model
	a.Provider = provider
	if a.SystemPromptFn != nil && provider != nil {
		a.SystemPrompt = a.SystemPromptFn(provider.Name(), model.ID)
	}
}

func WithTemperature(temp float64) Option {
	return func(a *Agent) { a.Temperature = temp }
}

func WithTopP(topP float64) Option {
	return func(a *Agent) { a.TopP = topP }
}

func WithMaxTokens(maxTokens int) Option {
	return func(a *Agent) { a.MaxTokens = maxTokens }
}

func WithAttachments(atts ...llm.Attachment) Option {
	return func(a *Agent) {
		if len(atts) > 0 {
			a.Attachments = append(a.Attachments, atts...)
		}
	}
}

func WithMaxIterations(maxIterations int) Option {
	return func(a *Agent) {
		if maxIterations > 0 {
			a.MaxIterations = maxIterations
		}
	}
}

func WithTool(t tool.Tool) Option {
	return func(a *Agent) {
		if t != nil {
			_ = a.Tools.Register(t)
		}
	}
}

func WithTools(ts ...tool.Tool) Option {
	return func(a *Agent) {
		for _, t := range ts {
			if t != nil {
				_ = a.Tools.Register(t)
			}
		}
	}
}

func WithToolNames(names ...string) Option {
	return func(a *Agent) {
		for _, n := range names {
			if n != "" {
				a.ToolNames = append(a.ToolNames, n)
			}
		}
	}
}

func WithMiddleware(mws ...middleware.Middleware) Option {
	return func(a *Agent) { a.chain.Append(mws...) }
}

func WithMiddlewareNames(names ...string) Option {
	return func(a *Agent) {
		for _, n := range names {
			if n != "" {
				a.middlewareNames = append(a.middlewareNames, n)
			}
		}
	}
}

func WithBus(b event.Bus) Option {
	return func(a *Agent) { a.Bus = b }
}

func WithLogger(l *logger.Logger) Option {
	return func(a *Agent) { a.log = l }
}

func WithReasoningEffort(effort string) Option {
	return func(a *Agent) { a.ReasoningEffort = effort }
}

func WithStructuredOutput(name string, schema any) Option {
	return func(a *Agent) {
		a.structuredName = name
		a.structuredSchema = schema
	}
}

func (a *Agent) Run(ctx context.Context, input string) (*RunResult, error) {
	return a.run(ctx, input, nil)
}

func (a *Agent) Resume(ctx context.Context, input string, history []llm.Message) (*RunResult, error) {
	return a.resume(ctx, input, history, nil)
}

func (a *Agent) ResumeStream(ctx context.Context, input string, history []llm.Message, handler StreamHandler) (*RunResult, error) {
	return a.resume(ctx, input, history, handler)
}

func (a *Agent) RunStream(ctx context.Context, input string, handler StreamHandler) (*RunResult, error) {
	return a.run(ctx, input, handler)
}

func (a *Agent) publish(topic string, payload any) {
	if a.Bus == nil {
		return
	}
	a.Bus.Publish(topic, payload)
}

func (a *Agent) QueueNotification(r reminder.Reminder) {
	a.notificationsMu.Lock()
	defer a.notificationsMu.Unlock()
	a.pendingNotifications = append(a.pendingNotifications, r)
}

func (a *Agent) AttachReminder(r reminder.Reminder) {
	a.notificationsMu.Lock()
	defer a.notificationsMu.Unlock()
	a.attachedReminder = &r
}

func (a *Agent) DetachReminder() {
	a.notificationsMu.Lock()
	defer a.notificationsMu.Unlock()
	a.attachedReminder = nil
}

func (a *Agent) StandingReminder() *reminder.Reminder {
	a.notificationsMu.Lock()
	defer a.notificationsMu.Unlock()
	if a.attachedReminder == nil {
		return nil
	}
	r := *a.attachedReminder
	return &r
}

func (a *Agent) HasPendingNotifications() bool {
	a.notificationsMu.Lock()
	defer a.notificationsMu.Unlock()
	return len(a.pendingNotifications) > 0
}

func (a *Agent) drainNotifications() []reminder.Reminder {
	a.notificationsMu.Lock()
	defer a.notificationsMu.Unlock()
	if len(a.pendingNotifications) == 0 {
		return nil
	}
	msgs := a.pendingNotifications
	a.pendingNotifications = nil
	return msgs
}

func (a *Agent) Continue(ctx context.Context, history []llm.Message) (*RunResult, error) {
	if !a.HasPendingNotifications() {
		return nil, nil
	}
	if a.Provider == nil {
		return nil, a.fail(ctx, ErrNoProvider)
	}
	if err := a.resolveToolNames(); err != nil {
		return nil, a.fail(ctx, err)
	}
	if err := a.resolveMiddlewareNames(); err != nil {
		return nil, a.fail(ctx, err)
	}

	ctx = WithAgent(ctx, a)
	state := &runState{agent: a}
	ctx = WithHistory(ctx, &state.history)
	a.debugf("agent %q continued run", a.Name)
	a.publish(TopicAgentStarted, AgentStarted{
		AgentID:         a.ID,
		AgentName:       a.Name,
		DisplayName:     a.DisplayName,
		ParentAgentID:   a.ParentAgentID,
		Model:           a.Model,
		Provider:        a.Provider,
		ReasoningEffort: a.ReasoningEffort,
		Agent:           a,
	})

	if err := a.chain.BeforeRun(ctx, a.Name, ""); err != nil {
		return nil, a.fail(ctx, err)
	}

	prov := a.Provider
	state.modelID = a.Model.ID
	state.provider = prov.Name()
	state.history = append(state.history, history...)
	return a.loop(ctx, state, prov)
}

func (a *Agent) debugf(format string, args ...any) {
	if a.log == nil {
		return
	}
	a.log.Fdebug(format, args...)
}

func (a *Agent) resolveToolNames() error {
	for _, name := range a.ToolNames {
		if _, ok := a.Tools.Get(name); ok {
			continue
		}
		t, ok := tools.Get(name)
		if !ok {
			return fmt.Errorf("%w: %q", tool.ErrNotFound, name)
		}
		_ = a.Tools.Register(t)
	}
	return nil
}

func (a *Agent) resolveMiddlewareNames() error {
	if a.namesResolved {
		return nil
	}
	for _, name := range a.middlewareNames {
		m, ok := middlewares.Get(name)
		if !ok {
			return fmt.Errorf("%w: %q", middleware.ErrNotFound, name)
		}
		a.chain.Append(m)
	}
	a.namesResolved = true
	return nil
}
