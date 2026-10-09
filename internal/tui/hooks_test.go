package tui

import (
	"testing"

	"github.com/gdamore/tcell/v2"

	"github.com/peggco/pegg/internal/core/hook"
	"github.com/peggco/pegg/internal/tui/components"
)

func TestOnSubmitTransform(t *testing.T) {
	submitHook = hook.NewHook[string]()
	OnSubmit(func(s string) string { return "> " + s })
	if got := dispatchSubmit("hello"); got != "> hello" {
		t.Errorf("dispatchSubmit = %q, want > hello", got)
	}
	submitHook = hook.NewHook[string]()
}

func TestOnKeyConsume(t *testing.T) {
	keyHook = hook.NewHook[KeyEvent]()
	OnKey(func(ke KeyEvent) KeyEvent {
		ke.Consumed = true
		return ke
	})
	ke := dispatchKey(tcell.NewEventKey(tcell.KeyCtrlC, 0, tcell.ModCtrl))
	if !ke.Consumed {
		t.Error("OnKey hook should be able to consume events")
	}
	keyHook = hook.NewHook[KeyEvent]()
}

func TestOnRegisterComponent(t *testing.T) {
	componentHook = hook.NewHook[[]components.Component]()
	type fake struct{ components.Component }
	OnRegisterComponent(func(c []components.Component) []components.Component {
		return append(c, fake{})
	})
	got := componentHook.Apply(nil)
	if len(got) != 1 {
		t.Fatalf("expected 1 component after hook, got %d", len(got))
	}
}

func TestResolveGlobal(t *testing.T) {
	cases := []struct {
		key  tcell.Key
		rune rune
		mod  tcell.ModMask
		want keyAction
	}{
		{tcell.KeyCtrlC, 0, tcell.ModCtrl, ActionQuit},
		{tcell.KeyCtrlQ, 0, tcell.ModCtrl, ActionQuit},
		{tcell.KeyCtrlT, 0, tcell.ModCtrl, ActionThemeNext},
		{tcell.KeyEnter, 0, 0, ActionNone},
		{tcell.KeyCtrlC, 0, tcell.ModCtrl | tcell.ModShift, ActionCopy},
		{tcell.KeyRune, 'c', tcell.ModCtrl | tcell.ModShift, ActionCopy},
		{tcell.KeyRune, 'x', tcell.ModCtrl | tcell.ModShift, ActionNone},
	}
	for _, c := range cases {
		ev := tcell.NewEventKey(c.key, c.rune, c.mod)
		if got := resolveGlobal(ev); got != c.want {
			t.Errorf("resolveGlobal(%v, %q) = %v, want %v", c.key, c.rune, got, c.want)
		}
	}
}
