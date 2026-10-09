package settings

import (
	"testing"

	"github.com/gdamore/tcell/v2"
)

func TestMemoryTabFocusReachesStatsAndClear(t *testing.T) {
	s := New(Deps{})
	m := s.memory
	if m.focus != memoryFocusEnabled {
		t.Fatalf("initial focus = %v, want Enabled", m.focus)
	}
	for i := 0; i < 9; i++ {
		if !m.HandleKey(tcell.NewEventKey(tcell.KeyDown, 0, 0)) {
			t.Fatalf("Down not handled at step %d", i)
		}
	}
	if m.focus != memoryFocusClear {
		t.Fatalf("focus = %v, want Clear", m.focus)
	}
	for i := 0; i < 9; i++ {
		if !m.HandleKey(tcell.NewEventKey(tcell.KeyUp, 0, 0)) {
			t.Fatalf("Up not handled at step %d", i)
		}
	}
	if m.focus != memoryFocusEnabled {
		t.Fatalf("focus = %v, want Enabled", m.focus)
	}
}

func TestMemoryTabGateCycling(t *testing.T) {
	s := New(Deps{})
	m := s.memory
	m.HandleKey(tcell.NewEventKey(tcell.KeyDown, 0, 0))
	m.HandleKey(tcell.NewEventKey(tcell.KeyDown, 0, 0))
	if m.focus != memoryFocusGate {
		t.Fatalf("focus = %v, want Gate", m.focus)
	}
	start := m.gate
	if !m.HandleKey(tcell.NewEventKey(tcell.KeyRight, 0, 0)) {
		t.Fatal("Right should cycle gate")
	}
	next := m.gate
	if next == start {
		t.Fatal("gate did not change")
	}
	if !m.HandleKey(tcell.NewEventKey(tcell.KeyLeft, 0, 0)) {
		t.Fatal("Left should cycle gate")
	}
	if m.gate != start {
		t.Fatalf("gate = %q, want back to %q", m.gate, start)
	}
	if !m.HandleKey(tcell.NewEventKey(tcell.KeyEnter, 0, 0)) {
		t.Fatal("Enter should cycle gate")
	}
	if m.gate == start {
		t.Fatal("gate did not change on Enter")
	}
}

func TestMemoryTabStatsRowsReadOnly(t *testing.T) {
	s := New(Deps{})
	m := s.memory
	for i := 0; i < 7; i++ {
		m.HandleKey(tcell.NewEventKey(tcell.KeyDown, 0, 0))
	}
	if m.focus != memoryFocusEntries {
		t.Fatalf("focus = %v, want Entries", m.focus)
	}
	if !m.HandleKey(tcell.NewEventKey(tcell.KeyLeft, 0, 0)) {
		t.Fatal("Left should be consumed on read-only row")
	}
	if !m.HandleKey(tcell.NewEventKey(tcell.KeyRight, 0, 0)) {
		t.Fatal("Right should be consumed on read-only row")
	}
	if !m.HandleKey(tcell.NewEventKey(tcell.KeyEnter, 0, 0)) {
		t.Fatal("Enter should be consumed on read-only row")
	}
	if m.focus != memoryFocusEntries {
		t.Fatalf("focus changed on read-only row: %v", m.focus)
	}
	m.HandleKey(tcell.NewEventKey(tcell.KeyDown, 0, 0))
	if m.focus != memoryFocusLastEntry {
		t.Fatalf("focus = %v, want LastEntry", m.focus)
	}
	if !m.HandleKey(tcell.NewEventKey(tcell.KeyEnter, 0, 0)) {
		t.Fatal("Enter should be consumed on read-only row")
	}
	if m.focus != memoryFocusLastEntry {
		t.Fatalf("focus changed on read-only row: %v", m.focus)
	}
}
