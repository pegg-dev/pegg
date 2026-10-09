package middlewares

import (
	"context"
	"fmt"
	"strings"
	"sync"

	agentmw "github.com/peggco/pegg/internal/agent/middleware"
	"github.com/peggco/pegg/internal/llm"
)

type responseRecord struct {
	content  string
	toolSigs []string
}

type loopDetector struct {
	agentmw.BaseMiddleware

	mu sync.Mutex

	recentTexts     []string
	recentReasoning []string
	recentResponses []responseRecord

	loopDetected bool
	loopReason   string
}

const (
	loopWindow      = 6
	repeatThreshold = 3
	maxCompareLen   = 300
)

func NewLoopDetector() *loopDetector {
	return &loopDetector{}
}

func (l *loopDetector) BeforeRun(_ context.Context, _, _ string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.recentTexts = nil
	l.recentReasoning = nil
	l.recentResponses = nil
	l.loopDetected = false
	l.loopReason = ""
	return nil
}

func (l *loopDetector) BeforeLLM(_ context.Context, req *llm.Request) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if !l.loopDetected {
		return nil
	}

	msg := fmt.Sprintf("An execution loop was detected: %s. Attempt an alternative reasoning path, change your approach, or use different tool arguments. Do not repeat the same pattern.", l.loopReason)
	req.Messages = append(req.Messages, llm.SystemMessage(msg))

	l.loopDetected = false
	l.loopReason = ""
	return nil
}

func (l *loopDetector) AfterLLM(_ context.Context, _ *llm.Request, resp *llm.Response) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	content := normalizeText(resp.GetContent())
	reasoning := normalizeText(resp.GetReasoning())
	toolCalls := resp.GetToolCalls()

	if content != "" {
		l.recentTexts = append(l.recentTexts, content)
		if len(l.recentTexts) > loopWindow {
			l.recentTexts = l.recentTexts[1:]
		}
	}

	if reasoning != "" {
		l.recentReasoning = append(l.recentReasoning, reasoning)
		if len(l.recentReasoning) > loopWindow {
			l.recentReasoning = l.recentReasoning[1:]
		}
	}

	var sigs []string
	for _, tc := range toolCalls {
		sigs = append(sigs, toolSig(tc))
	}
	rec := responseRecord{content: content, toolSigs: sigs}
	l.recentResponses = append(l.recentResponses, rec)
	if len(l.recentResponses) > loopWindow {
		l.recentResponses = l.recentResponses[1:]
	}

	if len(sigs) > 0 {
		if reason := detectResponseLoop(l.recentResponses); reason != "" {
			l.loopDetected = true
			l.loopReason = reason
			return nil
		}
	}
	if reason := detectLoop(l.recentTexts, "text"); reason != "" {
		l.loopDetected = true
		l.loopReason = reason
		return nil
	}
	if reason := detectLoop(l.recentReasoning, "thinking"); reason != "" {
		l.loopDetected = true
		l.loopReason = reason
		return nil
	}

	return nil
}

func detectLoop(window []string, label string) string {
	if len(window) < repeatThreshold {
		return ""
	}
	last := window[len(window)-1]
	count := 0
	for i := len(window) - 1; i >= 0; i-- {
		if window[i] == last {
			count++
		} else {
			break
		}
	}
	if count >= repeatThreshold {
		return fmt.Sprintf("repetitive %s output detected: same content produced %d consecutive times", label, count)
	}
	return ""
}

func detectResponseLoop(responses []responseRecord) string {
	if len(responses) < 3 {
		return ""
	}

	last := responses[len(responses)-1]
	count := 0
	for i := len(responses) - 1; i >= 0; i-- {
		if responsesEqual(responses[i], last) {
			count++
		} else {
			break
		}
	}
	if count >= repeatThreshold {
		label := "response"
		if len(last.toolSigs) > 0 {
			label = "text-tool"
		}
		return fmt.Sprintf("repetitive %s pattern detected: same content+tools produced %d consecutive times", label, count)
	}

	return ""
}

func responsesEqual(a, b responseRecord) bool {
	if a.content != b.content {
		return false
	}
	if len(a.toolSigs) != len(b.toolSigs) {
		return false
	}
	for i := range a.toolSigs {
		if a.toolSigs[i] != b.toolSigs[i] {
			return false
		}
	}
	return true
}

func toolSig(tc llm.ToolCall) string {
	args := strings.TrimSpace(tc.Function.Arguments)
	if len(args) > maxCompareLen {
		args = args[:maxCompareLen]
	}
	args = strings.Join(strings.Fields(args), " ")
	return tc.Function.Name + "(" + args + ")"
}

func normalizeText(s string) string {
	s = strings.TrimSpace(s)
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > maxCompareLen {
		s = s[:maxCompareLen]
	}
	return s
}

func (l *loopDetector) OnError(_ context.Context, _ error) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.recentTexts = nil
	l.recentReasoning = nil
	l.recentResponses = nil
	l.loopDetected = false
	l.loopReason = ""
	return nil
}
