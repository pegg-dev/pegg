package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/peggco/pegg/internal/agent/reminder"
	"github.com/peggco/pegg/internal/agent/tool"
	"github.com/peggco/pegg/internal/llm"
)

type runState struct {
	agent        *Agent
	history      []llm.Message
	iterations   int
	usage        llm.Usage
	output       string
	finishReason llm.FinishReason
	modelID      string
	provider     string
	stream       StreamHandler
}

func (a *Agent) run(ctx context.Context, input string, stream StreamHandler) (*RunResult, error) {
	if strings.TrimSpace(input) == "" {
		return nil, a.fail(ctx, ErrEmptyInput)
	}
	mi := expandInput(input)
	input = mi.Text
	if err := a.resolveToolNames(); err != nil {
		return nil, a.fail(ctx, err)
	}
	if err := a.resolveMiddlewareNames(); err != nil {
		return nil, a.fail(ctx, err)
	}
	resolveModel(ctx, a, input)
	if a.Provider == nil {
		return nil, a.fail(ctx, ErrNoProvider)
	}

	ctx = WithAgent(ctx, a)
	state := &runState{agent: a, stream: stream}
	ctx = WithHistory(ctx, &state.history)
	ctx = WithStream(ctx, stream)
	a.debugf("agent %q started run", a.Name)
	if err := a.chain.BeforeRun(ctx, a.Name, input); err != nil {
		return nil, a.fail(ctx, err)
	}
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
	a.publish(TopicAgentInput, AgentInput{
		AgentID:     a.ID,
		AgentName:   a.Name,
		Model:       a.Model,
		Input:       input,
		Attachments: a.Attachments,
	})

	prov := a.Provider
	state.modelID = a.Model.ID
	state.provider = prov.Name()

	if a.SystemPrompt != "" {
		state.history = append(state.history, llm.SystemMessage(a.SystemPrompt))
	}
	state.history = appendSystemReminders(state.history, mi.SystemReminders)
	if strings.TrimSpace(input) != "" {
		state.history = append(state.history, a.userMessage(input))
	}
	if len(mi.Calls) > 0 {
		if err := a.resolveInputCalls(ctx, state, mi.Calls); err != nil {
			return nil, a.fail(ctx, err)
		}
	}

	return a.loop(ctx, state, prov)
}

func (a *Agent) resume(ctx context.Context, input string, history []llm.Message, stream StreamHandler) (*RunResult, error) {
	if strings.TrimSpace(input) == "" {
		return nil, a.fail(ctx, ErrEmptyInput)
	}
	mi := expandInput(input)
	input = mi.Text
	if err := a.resolveToolNames(); err != nil {
		return nil, a.fail(ctx, err)
	}
	if err := a.resolveMiddlewareNames(); err != nil {
		return nil, a.fail(ctx, err)
	}
	resolveModel(ctx, a, input)
	if a.Provider == nil {
		return nil, a.fail(ctx, ErrNoProvider)
	}

	ctx = WithAgent(ctx, a)
	state := &runState{agent: a, stream: stream}
	ctx = WithHistory(ctx, &state.history)
	ctx = WithStream(ctx, stream)
	a.debugf("agent %q resumed run", a.Name)
	if err := a.chain.BeforeRun(ctx, a.Name, input); err != nil {
		return nil, a.fail(ctx, err)
	}
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
	a.publish(TopicAgentInput, AgentInput{
		AgentID:     a.ID,
		AgentName:   a.Name,
		Model:       a.Model,
		Input:       input,
		Attachments: a.Attachments,
	})

	prov := a.Provider
	state.modelID = a.Model.ID
	state.provider = prov.Name()

	state.history = append(state.history, history...)
	state.history = appendSystemReminders(state.history, mi.SystemReminders)
	if strings.TrimSpace(input) != "" {
		state.history = append(state.history, a.userMessage(input))
	}
	if len(mi.Calls) > 0 {
		if err := a.resolveInputCalls(ctx, state, mi.Calls); err != nil {
			return nil, a.fail(ctx, err)
		}
	}

	return a.loop(ctx, state, prov)
}

func appendSystemReminders(history []llm.Message, reminders []string) []llm.Message {
	for _, r := range reminders {
		r = strings.TrimSpace(r)
		if r == "" {
			continue
		}
		history = append(history, llm.SystemMessage("<system-reminder>\n"+r+"\n</system-reminder>"))
	}
	return history
}

func (a *Agent) loop(ctx context.Context, state *runState, prov llm.Provider) (*RunResult, error) {
	finished := false
	for state.iterations < a.MaxIterations {
		select {
		case <-ctx.Done():
			return a.partialResult(state), a.fail(ctx, ctx.Err())
		default:
		}
		state.iterations++
		msg, calls, err := a.iterate(ctx, state, prov)
		if err != nil {
			if msg.Role == llm.RoleAssistant && !emptyAssistantMessage(msg) {
				state.history = append(state.history, msg)
			}
			return a.partialResult(state), a.fail(ctx, err)
		}
		if !emptyAssistantMessage(msg) {
			state.history = append(state.history, msg)
		}
		a.publish(TopicAgentMessage, AgentMessage{
			AgentID:   a.ID,
			AgentName: a.Name,
			Model:     a.Model,
			Message:   msg,
		})
		if len(calls) > 0 {
			for _, call := range calls {
				if err := a.executeTool(ctx, state, call); err != nil {
					return a.partialResult(state), a.fail(ctx, err)
				}
			}
			continue
		}
		if a.HasPendingNotifications() {
			continue
		}
		finished = true
		break
	}

	result := &RunResult{
		AgentName:    a.Name,
		Model:        a.Model,
		Provider:     state.provider,
		Output:       state.output,
		History:      state.history,
		Usage:        state.usage,
		Iterations:   state.iterations,
		FinishReason: state.finishReason,
	}

	if !finished {
		err := fmt.Errorf("%w (%d iterations)", ErrMaxIterations, state.iterations)
		a.debugf("agent %q hit max iterations", a.Name)
		a.publish(TopicAgentError, AgentError{AgentID: a.ID, AgentName: a.Name, Model: a.Model, Err: err})
		_ = a.chain.OnError(ctx, err)
		return a.partialResult(state), err
	}

	if err := a.chain.AfterRun(ctx, a.Name, result, nil); err != nil {
		return nil, a.fail(ctx, err)
	}
	a.publish(TopicAgentFinished, AgentFinished{
		AgentID:      a.ID,
		AgentName:    a.Name,
		Model:        a.Model,
		Output:       result.Output,
		Usage:        result.Usage,
		Iterations:   result.Iterations,
		FinishReason: result.FinishReason,
	})
	a.debugf("agent %q finished run in %d iterations", a.Name, state.iterations)
	return result, nil
}

func (a *Agent) userMessage(input string) llm.Message {
	if len(a.Attachments) == 0 {
		return llm.UserMessage(input)
	}
	return llm.UserMessage(llm.ContentWithAttachments(input, a.Attachments))
}

func (a *Agent) resolveInputCalls(ctx context.Context, state *runState, calls []llm.ToolCall) error {
	msg := llm.AssistantMessage("")
	msg.ToolCalls = calls
	state.history = append(state.history, msg)
	for _, call := range calls {
		if err := a.executeTool(ctx, state, call); err != nil {
			return err
		}
	}
	return nil
}

func emptyAssistantMessage(m llm.Message) bool {
	return m.Role == llm.RoleAssistant && len(m.ToolCalls) == 0 && llm.MessageText(m) == ""
}

func appendReminder(msgs []llm.Message, text string) []llm.Message {
	if len(msgs) == 0 {
		return append(msgs, llm.SystemMessage(text))
	}
	last := -1
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == llm.RoleUser || msgs[i].Role == llm.RoleTool {
			last = i
			break
		}
	}
	if last < 0 {
		return append(msgs, llm.SystemMessage(text))
	}
	out := append([]llm.Message(nil), msgs...)
	m := out[last]
	switch c := m.Content.(type) {
	case string:
		m.Content = c + "\n\n" + text
	case llm.Content:
		m.Content = llm.Content{Text: c.Text + "\n\n" + text, Attachments: c.Attachments}
	case []any:
		parts := append([]any(nil), c...)
		parts = append(parts, map[string]any{"type": "text", "text": text})
		m.Content = parts
	}
	out[last] = m
	return out
}

func (a *Agent) partialResult(state *runState) *RunResult {
	return &RunResult{
		AgentName:  a.Name,
		Model:      a.Model,
		Provider:   state.provider,
		History:    state.history,
		Usage:      state.usage,
		Iterations: state.iterations,
	}
}

func (a *Agent) iterate(ctx context.Context, state *runState, prov llm.Provider) (llm.Message, []llm.ToolCall, error) {
	req := a.buildRequest(state)

	if r := a.StandingReminder(); r != nil {
		if formatted := reminder.FormatAll([]reminder.Reminder{*r}); formatted != "" {
			req.Messages = appendReminder(req.Messages, formatted)
		}
	}

	if notifications := a.drainNotifications(); len(notifications) > 0 {
		if formatted := reminder.FormatAll(notifications); formatted != "" {
			req.Messages = append(req.Messages, llm.SystemMessage(formatted))
		}
	}

	if err := a.chain.BeforeLLM(ctx, req); err != nil {
		return llm.Message{}, nil, err
	}
	if state.stream != nil {
		return a.iterateStream(ctx, state, prov, req)
	}

	resp, err := a.chain.InvokeLLM(ctx, req, prov.Chat)
	if err != nil {
		return llm.Message{}, nil, err
	}
	if err := a.chain.AfterLLM(ctx, req, resp); err != nil {
		return llm.Message{}, nil, err
	}

	state.accumulateUsage(resp.Usage)
	a.publish(TopicAgentUsage, AgentUsage{
		AgentID:   a.ID,
		AgentName: a.Name,
		Model:     a.Model,
		Usage:     state.usage,
	})
	state.output = resp.GetContent()
	state.finishReason = resp.GetFinishReason()

	msg := llm.AssistantMessage(state.output)
	if reasoning := resp.GetReasoning(); reasoning != "" {
		msg.Reasoning = reasoning
	}
	calls := resp.GetToolCalls()
	if len(calls) > 0 {
		msg.ToolCalls = calls
	}
	return msg, calls, nil
}

func (a *Agent) iterateStream(ctx context.Context, state *runState, prov llm.Provider, req *llm.Request) (llm.Message, []llm.ToolCall, error) {
	acc := newStreamAccumulator()
	err := a.chain.InvokeLLMStream(ctx, req, func(chunk llm.StreamChunk) error {
		acc.append(chunk)
		if chunk.Usage != nil {
			state.accumulateUsage(*chunk.Usage)
			a.publish(TopicAgentUsage, AgentUsage{
				AgentID:   a.ID,
				AgentName: a.Name,
				Model:     a.Model,
				Usage:     state.usage,
			})
		}
		if chunk.FinishReason != "" {
			state.finishReason = chunk.FinishReason
		}
		if chunk.Content != "" {
			if err := state.stream(StreamEvent{
				Type:      StreamToken,
				AgentID:   a.ID,
				AgentName: a.Name,
				Content:   chunk.Content,
			}); err != nil {
				return err
			}
			a.publish(TopicAgentToken, AgentToken{
				AgentID:   a.ID,
				AgentName: a.Name,
				Model:     a.Model,
				Content:   chunk.Content,
			})
		}
		if chunk.Reasoning != "" {
			if err := state.stream(StreamEvent{
				Type:      StreamReasoning,
				AgentID:   a.ID,
				AgentName: a.Name,
				Reasoning: chunk.Reasoning,
			}); err != nil {
				return err
			}
			a.publish(TopicAgentToken, AgentToken{
				AgentID:   a.ID,
				AgentName: a.Name,
				Model:     a.Model,
				Reasoning: chunk.Reasoning,
			})
		}
		return nil
	}, prov.ChatStream)
	if err != nil {
		if acc.content == "" && len(acc.order) == 0 && acc.reasoning == "" {
			return llm.Message{}, nil, err
		}
		msg := llm.AssistantMessage(acc.content)
		if acc.reasoning != "" {
			msg.Reasoning = acc.reasoning
		}
		calls := acc.toolCalls()
		if len(calls) > 0 {
			msg.ToolCalls = calls
		}
		return msg, calls, err
	}

	state.output = acc.content
	msg := llm.AssistantMessage(acc.content)
	if acc.reasoning != "" {
		msg.Reasoning = acc.reasoning
	}
	calls := acc.toolCalls()
	if len(calls) > 0 {
		msg.ToolCalls = calls
	}

	resp := &llm.Response{
		Model:   req.Model,
		Choices: []llm.Choice{{Index: 0, Message: &msg, FinishReason: &state.finishReason}},
		Usage:   state.usage,
	}
	if err := a.chain.AfterLLM(ctx, req, resp); err != nil {
		return llm.Message{}, nil, err
	}

	for _, call := range calls {
		callCopy := call
		if err := state.stream(StreamEvent{
			Type:      StreamToolCall,
			AgentID:   a.ID,
			AgentName: a.Name,
			ToolCall:  &callCopy,
		}); err != nil {
			return llm.Message{}, nil, err
		}
	}
	if err := state.stream(StreamEvent{
		Type:      StreamDone,
		AgentID:   a.ID,
		AgentName: a.Name,
		Model:     a.Model,
		Content:   acc.content,
		Usage:     &state.usage,
	}); err != nil {
		return llm.Message{}, nil, err
	}
	return msg, calls, nil
}

func (a *Agent) executeTool(ctx context.Context, state *runState, call llm.ToolCall) error {
	a.debugf("agent %q tool call %q (%s)", a.Name, call.Function.Name, call.ID)
	a.publish(TopicAgentToolCall, AgentToolCall{
		AgentID:   a.ID,
		AgentName: a.Name,
		Model:     a.Model,
		Call:      call,
	})

	if err := a.chain.BeforeTool(ctx, call); err != nil {
		output := fmt.Sprintf("Error: tool call aborted by middleware: %v", err)
		state.history = append(state.history, llm.ToolMessage(output, call.ID))
		a.emitToolResult(state, call, output, err)
		return nil
	}

	t, ok := a.Tools.Get(call.Function.Name)
	var output string
	var toolErr error
	if !ok {
		output = fmt.Sprintf("Error: tool %q is not registered", call.Function.Name)
		toolErr = fmt.Errorf("%w: %s", tool.ErrNotFound, call.Function.Name)
	} else {
		output, toolErr = a.chain.InvokeTool(ctx, call, func(ctx context.Context, call llm.ToolCall) (string, error) {
			return t.Execute(ctx, call.Function.Arguments)
		})
		if toolErr != nil {
			output = fmt.Sprintf("Error: %v", toolErr)
			toolErr = fmt.Errorf("%w: %s: %v", ErrToolExecutionFailed, t.Name(), toolErr)
		}
	}

	_ = a.chain.AfterTool(ctx, call, output, toolErr)
	state.history = append(state.history, llm.ToolMessage(output, call.ID))
	a.emitToolResult(state, call, output, toolErr)
	return nil
}

func (a *Agent) emitToolResult(state *runState, call llm.ToolCall, output string, toolErr error) {
	a.publish(TopicAgentToolResult, AgentToolResult{
		AgentID:   a.ID,
		AgentName: a.Name,
		Model:     a.Model,
		CallID:    call.ID,
		ToolName:  call.Function.Name,
		Output:    output,
		Err:       toolErr,
	})
	if state.stream != nil {
		callCopy := call
		_ = state.stream(StreamEvent{
			Type:       StreamToolResult,
			AgentID:    a.ID,
			AgentName:  a.Name,
			ToolCall:   &callCopy,
			ToolOutput: output,
			ToolErr:    toolErr,
		})
	}
}

func (a *Agent) buildRequest(state *runState) *llm.Request {
	req := llm.NewRequest(state.modelID, state.history)
	if a.Temperature != 0 {
		req.Temperature = a.Temperature
	}
	if a.TopP != 0 {
		req.TopP = a.TopP
	}
	if a.MaxTokens != 0 {
		req.MaxTokens = a.MaxTokens
	}
	if a.ReasoningEffort != "" {
		req.ReasoningEffort = a.ReasoningEffort
	}
	if a.structuredName != "" && a.structuredSchema != nil {
		req.WithStructuredOutput(a.structuredName, a.structuredSchema)
	}
	if tools := a.Tools.LLMTools(); len(tools) > 0 {
		req.Tools = tools
	}
	return req
}

func (a *Agent) fail(ctx context.Context, err error) error {
	a.debugf("agent %q failed: %v", a.Name, err)
	a.publish(TopicAgentError, AgentError{AgentID: a.ID, AgentName: a.Name, Model: a.Model, Err: err})
	_ = a.chain.OnError(ctx, err)
	return err
}

func (s *runState) accumulateUsage(u llm.Usage) {
	s.usage.PromptTokens = u.PromptTokens
	s.usage.CompletionTokens = u.CompletionTokens
	s.usage.TotalTokens = u.TotalTokens
	s.usage.Cost += u.Cost
}

type accToolCall struct {
	id        string
	name      string
	typ       string
	arguments string
}

type streamAccumulator struct {
	content   string
	reasoning string
	calls     map[int]*accToolCall
	order     []int
}

func newStreamAccumulator() *streamAccumulator {
	return &streamAccumulator{calls: make(map[int]*accToolCall)}
}

func (a *streamAccumulator) append(chunk llm.StreamChunk) {
	a.content += chunk.Content
	a.reasoning += chunk.Reasoning
	for _, tc := range chunk.ToolCalls {
		acc, ok := a.calls[tc.Index]
		if !ok {
			acc = &accToolCall{}
			a.calls[tc.Index] = acc
			a.order = append(a.order, tc.Index)
		}
		if tc.ID != "" {
			acc.id = string([]byte(tc.ID))
		}
		if tc.Type != "" {
			acc.typ = string([]byte(tc.Type))
		}
		if tc.Function.Name != "" {
			acc.name = string([]byte(tc.Function.Name))
		}
		acc.arguments += tc.Function.Arguments
	}
}

func (a *streamAccumulator) toolCalls() []llm.ToolCall {
	calls := make([]llm.ToolCall, 0, len(a.order))
	for _, idx := range a.order {
		acc := a.calls[idx]
		calls = append(calls, llm.ToolCall{
			Index: idx,
			ID:    acc.id,
			Type:  acc.typ,
			Function: llm.Function{
				Name:      acc.name,
				Arguments: acc.arguments,
			},
		})
	}
	return calls
}
