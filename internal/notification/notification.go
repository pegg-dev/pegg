package notification

import (
	"strings"
	"sync"

	"github.com/peggco/pegg/internal/agent"
	"github.com/peggco/pegg/internal/core/event"
	"github.com/peggco/pegg/internal/core/logger"
)

const maxMessageRunes = 200

type Notification struct {
	Title   string
	Message string
}

type Driver interface {
	Notify(n Notification) error
	Close() error
}

type Notifier struct {
	mu       sync.RWMutex
	drivers  []Driver
	bus      event.Bus
	log      *logger.Logger
	enabled  bool
	focused  bool
	subs     map[string]bool
	handlers []handlerRef
}

type handlerRef struct {
	topic string
	fn    any
}

func New(bus event.Bus, log *logger.Logger, enabled bool, drivers ...Driver) *Notifier {
	return &Notifier{
		drivers: drivers,
		bus:     bus,
		log:     log,
		enabled: enabled,
		focused: true,
		subs:    make(map[string]bool),
	}
}

func (n *Notifier) Start() error {
	if n.bus == nil {
		return nil
	}

	syncSubs := []handlerRef{
		{event.TopicAppFocused, func() { n.SetFocused(true) }},
		{event.TopicAppBlurred, func() { n.SetFocused(false) }},
		{agent.TopicAgentStarted, n.onAgentStarted},
	}
	for _, s := range syncSubs {
		if err := n.bus.Subscribe(s.topic, s.fn); err != nil {
			return err
		}
		n.handlers = append(n.handlers, s)
	}

	asyncSubs := []handlerRef{
		{agent.TopicAgentAsk, n.onAgentAsk},
		{agent.TopicAgentFinished, n.onAgentFinished},
		{agent.TopicAgentError, n.onAgentError},
	}
	for _, s := range asyncSubs {
		if err := n.bus.SubscribeAsync(s.topic, s.fn, false); err != nil {
			return err
		}
		n.handlers = append(n.handlers, s)
	}
	return nil
}

func (n *Notifier) SetFocused(v bool) {
	n.mu.Lock()
	n.focused = v
	n.mu.Unlock()
}

func (n *Notifier) Focused() bool {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.focused
}

func (n *Notifier) Enabled() bool {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.enabled
}

func (n *Notifier) Close() error {
	if n.bus != nil {
		for _, h := range n.handlers {
			_ = n.bus.Unsubscribe(h.topic, h.fn)
		}
	}
	n.handlers = nil
	var firstErr error
	for _, d := range n.drivers {
		if err := d.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func (n *Notifier) onAgentStarted(e agent.AgentStarted) {
	if e.ParentAgentID == "" {
		return
	}
	n.mu.Lock()
	n.subs[e.AgentID] = true
	n.mu.Unlock()
}

func (n *Notifier) onAgentAsk(e agent.AgentAsk) {
	if n.skip(e.AgentID) {
		return
	}
	title := "Input required"
	msg := "Your input is required"
	if len(e.Questions) > 0 {
		q := e.Questions[0].Question
		if strings.Contains(q, "requested permission") {
			title = "Permission required"
		}
		msg = truncateRunes(oneLine(q), maxMessageRunes)
	}
	if e.AgentName != "" {
		title += ": " + e.AgentName
	}
	n.notify(title, msg)
}

func (n *Notifier) onAgentFinished(e agent.AgentFinished) {
	if n.skip(e.AgentID) {
		return
	}
	name := e.AgentName
	if name == "" {
		name = "agent"
	}
	msg := truncateRunes(oneLine(e.Output), maxMessageRunes)
	if msg == "" {
		msg = "Task completed"
	}
	n.notify("Agent finished: "+name, msg)
}

func (n *Notifier) onAgentError(e agent.AgentError) {
	if n.skip(e.AgentID) {
		return
	}
	name := e.AgentName
	if name == "" {
		name = "agent"
	}
	msg := ""
	if e.Err != nil {
		msg = e.Err.Error()
	}
	n.notify("Agent failed: "+name, truncateRunes(oneLine(msg), maxMessageRunes))
}

func (n *Notifier) skip(agentID string) bool {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.subs[agentID]
}

func (n *Notifier) notify(title, message string) {
	if !n.enabled || len(n.drivers) == 0 {
		return
	}
	if n.Focused() {
		return
	}
	notif := Notification{Title: title, Message: message}
	for _, d := range n.drivers {
		if err := d.Notify(notif); err != nil && n.log != nil {
			n.log.Fwarn("notification: %v", err)
		}
	}
}

func oneLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

func truncateRunes(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max]) + "..."
}
