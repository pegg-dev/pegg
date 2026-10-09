package tui

import (
	"sync"

	"github.com/gdamore/tcell/v2"

	"github.com/peggco/pegg/internal/core/event"
	"github.com/peggco/pegg/internal/core/hook"
	"github.com/peggco/pegg/internal/tui/components"
)

type KeyEvent struct {
	Key      tcell.Key
	Rune     rune
	Mod      tcell.ModMask
	Consumed bool
}

var (
	submitHook    = hook.NewHook[string]()
	keyHook       = hook.NewHook[KeyEvent]()
	componentHook = hook.NewHook[[]components.Component]()
	readyHooks    = newCallbacks()
	quitHooks     = newCallbacks()
	busMu         sync.RWMutex
	appBus        event.Bus
)

type callbacks struct {
	mu  sync.Mutex
	fns []func()
}

func newCallbacks() *callbacks { return &callbacks{} }

func (c *callbacks) Add(fn func()) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if fn != nil {
		c.fns = append(c.fns, fn)
	}
}

func (c *callbacks) Run() {
	c.mu.Lock()
	fns := append([]func(){}, c.fns...)
	c.mu.Unlock()
	for _, fn := range fns {
		fn()
	}
}

func setBus(b event.Bus) {
	busMu.Lock()
	appBus = b
	busMu.Unlock()
}

func currentBus() event.Bus {
	busMu.RLock()
	defer busMu.RUnlock()
	return appBus
}

func OnSubmit(fn func(string) string) { submitHook.Add(fn) }

func OnKey(fn func(KeyEvent) KeyEvent) { keyHook.Add(fn) }

func OnRegisterComponent(fn func([]components.Component) []components.Component) {
	componentHook.Add(fn)
}

func OnReady(fn func()) { readyHooks.Add(fn) }

func OnQuit(fn func()) { quitHooks.Add(fn) }

func dispatchSubmit(msg string) string {
	msg = submitHook.Apply(msg)
	if bus := currentBus(); bus != nil {
		bus.Publish(TopicSubmit, SubmitEvent{Message: msg})
	}
	return msg
}

func dispatchKey(ev *tcell.EventKey) KeyEvent {
	ke := KeyEvent{Key: ev.Key(), Rune: ev.Rune(), Mod: ev.Modifiers()}
	return keyHook.Apply(ke)
}

func dispatchReady() { readyHooks.Run() }
func dispatchQuit()  { quitHooks.Run() }
