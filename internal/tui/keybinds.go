package tui

import "github.com/gdamore/tcell/v2"

type keyCombo struct {
	key  tcell.Key
	rune rune
	mod  tcell.ModMask
}

type keyAction int

const (
	ActionNone keyAction = iota
	ActionQuit
	ActionThemeNext
	ActionSettings
	ActionCopy
)

var globalBindings = map[keyCombo]keyAction{
	{key: tcell.KeyCtrlC}: ActionQuit,
	{key: tcell.KeyCtrlQ}: ActionQuit,
	{key: tcell.KeyCtrlT}: ActionThemeNext,
	{key: tcell.KeyCtrlP}: ActionSettings,
	{key: tcell.KeyCtrlC, mod: tcell.ModCtrl | tcell.ModShift}:           ActionCopy,
	{key: tcell.KeyRune, rune: 'c', mod: tcell.ModCtrl | tcell.ModShift}: ActionCopy,
	{key: tcell.KeyRune, rune: 'C', mod: tcell.ModCtrl | tcell.ModShift}: ActionCopy,
}

func resolveGlobal(ev *tcell.EventKey) keyAction {
	combo := keyCombo{key: ev.Key(), mod: ev.Modifiers()}
	if ev.Key() == tcell.KeyRune {
		combo.rune = ev.Rune()
	}
	if a, ok := globalBindings[combo]; ok {
		return a
	}
	combo.mod = 0
	return globalBindings[combo]
}
