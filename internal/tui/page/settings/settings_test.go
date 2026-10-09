package settings

import (
	"testing"

	"github.com/gdamore/tcell/v2"

	"github.com/peggco/pegg/internal/core/config"
	"github.com/peggco/pegg/internal/llm"
	"github.com/peggco/pegg/internal/llm/credentials"
	"github.com/peggco/pegg/internal/llm/subscription"
	"github.com/peggco/pegg/internal/tui/components"
	"github.com/peggco/pegg/internal/tui/layout"
	"github.com/peggco/pegg/internal/tui/styles"
)

func drawSettings(t *testing.T, w, h int, s *Settings) {
	t.Helper()
	screen := newSim(t, w, h)
	s.Draw(screen, layout.Region{Left: 0, Top: 0, Width: w, Height: h}, true)
	screen.Show()
}

func newSim(t *testing.T, w, h int) tcell.Screen {
	t.Helper()
	s := tcell.NewSimulationScreen("UTF-8")
	if err := s.Init(); err != nil {
		t.Fatalf("init simulation screen: %v", err)
	}
	t.Cleanup(s.Fini)
	s.SetSize(w, h)
	return s
}

func TestSettingsDrawSmoke(t *testing.T) {
	styles.RegisterDefaults()
	styles.Set("dark")
	for _, size := range [][2]int{{80, 24}, {120, 40}, {50, 15}, {200, 50}} {
		drawSettings(t, size[0], size[1], New(Deps{}))
	}
}

func TestSettingsTabs(t *testing.T) {
	s := New(Deps{})
	if s.tab != tabGeneral {
		t.Fatalf("initial tab = %v, want general", s.tab)
	}
	s.HandleKey(tcell.NewEventKey(tcell.KeyTab, 0, 0))
	if s.tab != tabSession {
		t.Errorf("after Tab tab = %v, want Session", s.tab)
	}
	s.HandleKey(tcell.NewEventKey(tcell.KeyRight, 0, 0))
	if s.tab != tabMCP {
		t.Errorf("after Right tab = %v, want MCP", s.tab)
	}
	s.HandleKey(tcell.NewEventKey(tcell.KeyRight, 0, 0))
	if s.tab != tabSkills {
		t.Errorf("after Right tab = %v, want Skills", s.tab)
	}
	s.HandleKey(tcell.NewEventKey(tcell.KeyRight, 0, 0))
	if s.tab != tabRules {
		t.Errorf("after Right tab = %v, want Rules", s.tab)
	}
	s.HandleKey(tcell.NewEventKey(tcell.KeyRight, 0, 0))
	if s.tab != tabPlugins {
		t.Errorf("after Right tab = %v, want Plugins", s.tab)
	}
	s.HandleKey(tcell.NewEventKey(tcell.KeyRight, 0, 0))
	if s.tab != tabPermissions {
		t.Errorf("after Right tab = %v, want Permissions", s.tab)
	}
	s.HandleKey(tcell.NewEventKey(tcell.KeyRight, 0, 0))
	if s.tab != tabMemory {
		t.Errorf("after Right tab = %v, want Memory", s.tab)
	}
	s.HandleKey(tcell.NewEventKey(tcell.KeyRight, 0, 0))
	if s.tab != tabSystem {
		t.Errorf("after Right tab = %v, want System", s.tab)
	}
	s.HandleKey(tcell.NewEventKey(tcell.KeyUp, 0, 0))
	s.HandleKey(tcell.NewEventKey(tcell.KeyUp, 0, 0))
	s.HandleKey(tcell.NewEventKey(tcell.KeyRight, 0, 0))
	if s.tab != tabGeneral {
		t.Errorf("after wrap tab = %v, want General", s.tab)
	}
	s.HandleKey(tcell.NewEventKey(tcell.KeyLeft, 0, 0))
	if s.tab != tabSystem {
		t.Errorf("after Left tab = %v, want System", s.tab)
	}
}

func TestSettingsEscCloses(t *testing.T) {
	s := New(Deps{})
	closed := false
	s.SetOnClose(func() { closed = true })
	s.HandleKey(tcell.NewEventKey(tcell.KeyEsc, 0, 0))
	if !closed {
		t.Error("Esc should fire onClose")
	}
}

func TestSettingsGeneralEnterOpensProviderSub(t *testing.T) {
	s := New(Deps{})
	s.HandleKey(tcell.NewEventKey(tcell.KeyEnter, 0, 0))
	if s.sub == nil {
		t.Fatal("Enter on Provider row should open a sub-modal")
	}
	s.HandleKey(tcell.NewEventKey(tcell.KeyEsc, 0, 0))
	if s.sub != nil {
		t.Error("Esc should close the sub-modal")
	}
}

func TestSettingsSubOpensModelPickerAndBack(t *testing.T) {
	s := New(Deps{})
	s.HandleKey(tcell.NewEventKey(tcell.KeyDown, 0, 0))
	s.HandleKey(tcell.NewEventKey(tcell.KeyEnter, 0, 0))
	if s.sub == nil {
		t.Fatal("Enter on Model row should open the picker")
	}
	s.HandleKey(tcell.NewEventKey(tcell.KeyEsc, 0, 0))
	if s.sub != nil {
		t.Error("Esc should close the picker")
	}
}

func TestSettingsGeneralRowSelection(t *testing.T) {
	s := New(Deps{})
	g := s.general
	if g.index != 0 {
		t.Fatalf("initial index = %d", g.index)
	}
	s.HandleKey(tcell.NewEventKey(tcell.KeyDown, 0, 0))
	s.HandleKey(tcell.NewEventKey(tcell.KeyDown, 0, 0))
	if g.index != 1 {
		t.Errorf("after Down index = %d, want 1", g.index)
	}
	for i := 0; i < 6; i++ {
		s.HandleKey(tcell.NewEventKey(tcell.KeyDown, 0, 0))
	}
	if g.index != 3 {
		t.Errorf("index should clamp at 3, got %d", g.index)
	}
}

func TestListModalHandleKey(t *testing.T) {
	back := false
	m := &listModal{title: "x", list: components.NewList("x"), onBack: func() { back = true }}
	m.HandleKey(tcell.NewEventKey(tcell.KeyEsc, 0, 0))
	if !back {
		t.Error("Esc on listModal should trigger onBack")
	}
}

func TestSettingsAllTabsDraw(t *testing.T) {
	styles.RegisterDefaults()
	styles.Set("dark")
	s := New(Deps{})
	for i := 0; i < len(tabNames); i++ {
		s.tab = tabKind(i)
		drawSettings(t, 80, 24, s)
		drawSettings(t, 110, 32, s)
	}
}

func TestSettingsSubModalsDraw(t *testing.T) {
	styles.RegisterDefaults()
	styles.Set("dark")
	s := New(Deps{})
	s.general.HandleKey(tcell.NewEventKey(tcell.KeyEnter, 0, 0))
	drawSettings(t, 80, 24, s)
	s.HandleKey(tcell.NewEventKey(tcell.KeyEsc, 0, 0))
	s.general.HandleKey(tcell.NewEventKey(tcell.KeyDown, 0, 0))
	s.general.HandleKey(tcell.NewEventKey(tcell.KeyEnter, 0, 0))
	drawSettings(t, 80, 24, s)
	s.HandleKey(tcell.NewEventKey(tcell.KeyEsc, 0, 0))
	s.general.HandleKey(tcell.NewEventKey(tcell.KeyDown, 0, 0))
	s.general.HandleKey(tcell.NewEventKey(tcell.KeyEnter, 0, 0))
	drawSettings(t, 80, 24, s)
}

func TestSettingsSetSelectedModel(t *testing.T) {
	s := New(Deps{})
	s.SetSelectedModel("provider-x", llm.Model{ID: "model-y", Name: "Model Y"})
	if got := s.modelDisplay(); got != "provider-x/Model Y" {
		t.Errorf("modelDisplay = %q, want provider-x/Model Y", got)
	}
}

func TestSettingsSelectedModelEmptyDisplay(t *testing.T) {
	s := New(Deps{})
	if got := s.modelDisplay(); got != "—" {
		t.Errorf("empty modelDisplay = %q, want em dash", got)
	}
}

func TestSettingsAllTabsMouseSmoke(t *testing.T) {
	styles.RegisterDefaults()
	styles.Set("dark")
	for i := 0; i < len(tabNames); i++ {
		s := New(Deps{})
		s.tab = tabKind(i)
		s.tabs.SetActive(i)
		drawSettings(t, 100, 30, s)
		content := s.contentRegion()
		for y := content.Top; y < content.Bottom() && y < content.Top+12; y++ {
			s.HandleMouse(content.Left+2, y, tcell.ButtonPrimary)
			s.HandleMouse(content.Left+2, y, tcell.WheelDown)
			if s.sub != nil {
				s.HandleKey(tcell.NewEventKey(tcell.KeyEsc, 0, 0))
				drawSettings(t, 100, 30, s)
				content = s.contentRegion()
			}
		}
	}
}

func TestSettingsOpenSubscription(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	s := New(Deps{})
	subscription.Register(subscription.Info{
		Provider: "tuisub",
		Hint:     "run tui-cli",
		Status:   func() credentials.Status { return credentials.Status{Provider: "tuisub", LoggedIn: true} },
	})
	s.openSubscription("tuisub", mustSub(t, "tuisub"))
	if s.sub != nil {
		t.Fatal("signed-in subscription should close the modal")
	}
	if s.errMsg != "" {
		t.Fatalf("errMsg = %q", s.errMsg)
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, p := range cfg.Providers {
		if p.Provider == "tuisub" {
			found = true
		}
	}
	if !found {
		t.Fatalf("tuisub provider not saved: %+v", cfg.Providers)
	}
}

func TestSettingsOpenSubscriptionNotSignedIn(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	s := New(Deps{})
	subscription.Register(subscription.Info{
		Provider: "tuisub2",
		Hint:     "run tui-cli",
		Status:   func() credentials.Status { return credentials.Status{Provider: "tuisub2", LoggedIn: false} },
	})
	s.openSubscription("tuisub2", mustSub(t, "tuisub2"))
	if s.sub == nil {
		t.Fatal("not-signed-in subscription should show instructions")
	}
}

func mustSub(t *testing.T, name string) subscription.Info {
	t.Helper()
	info, ok := subscription.Get(name)
	if !ok {
		t.Fatalf("subscription %q not registered", name)
	}
	return info
}

func TestSettingsSetSelectedModelUsesID(t *testing.T) {
	s := New(Deps{})
	s.SetSelectedModel("p", llm.Model{ID: "m1"})
	if got := s.modelDisplay(); got != "p/m1" {
		t.Errorf("modelDisplay = %q, want p/m1", got)
	}
}

func TestSettingsTabClick(t *testing.T) {
	styles.RegisterDefaults()
	styles.Set("dark")
	s := New(Deps{})
	drawSettings(t, 100, 30, s)
	inner := s.innerRegion()

	var targetX int
	for x := inner.Left; x < inner.Right(); x++ {
		if idx, ok := s.tabs.HandleMouse(x, inner.Top); ok && idx == 1 {
			targetX = x
			break
		}
	}
	if targetX == 0 {
		t.Fatal("could not locate the Session tab")
	}
	if !s.HandleMouse(targetX, inner.Top, tcell.ButtonPrimary) {
		t.Fatal("tab click should be handled")
	}
	if s.tab != tabSession {
		t.Fatalf("tab = %v, want Session", s.tab)
	}
}

func TestSettingsGeneralRowClick(t *testing.T) {
	styles.RegisterDefaults()
	styles.Set("dark")
	s := New(Deps{})
	drawSettings(t, 100, 30, s)
	content := s.contentRegion()

	if !s.HandleMouse(content.Left+2, content.Top, tcell.ButtonPrimary) {
		t.Fatal("row click should be handled")
	}
	if s.sub == nil {
		t.Fatal("clicking the Provider row should open a sub-modal")
	}
}

func TestSettingsOutsideClickCloses(t *testing.T) {
	styles.RegisterDefaults()
	styles.Set("dark")
	s := New(Deps{})
	closed := false
	s.SetOnClose(func() { closed = true })
	drawSettings(t, 100, 30, s)

	if !s.HandleMouse(0, 0, tcell.ButtonPrimary) {
		t.Fatal("outside click should be handled")
	}
	if !closed {
		t.Fatal("clicking outside the dialog should close it")
	}
}

func TestListModalMouse(t *testing.T) {
	styles.RegisterDefaults()
	styles.Set("dark")
	l := components.NewList("x")
	l.SetItems([]components.ListItem{{Label: "a"}, {Label: "b"}})
	selected := -1
	l.SetOnSelect(func(i int, _ components.ListItem) { selected = i })
	back := false
	m := &listModal{title: "x", list: l, onBack: func() { back = true }}
	screen := newSim(t, 60, 20)
	m.Draw(screen, layout.Region{Left: 0, Top: 0, Width: 60, Height: 20}, true)

	if !m.HandleMouse(m.region.Left+1, m.region.Top, tcell.ButtonPrimary) {
		t.Fatal("list click should be handled")
	}
	if selected != 0 {
		t.Fatalf("selected = %d, want 0", selected)
	}
	m.HandleMouse(0, 0, tcell.ButtonPrimary)
	if !back {
		t.Fatal("clicking outside the list modal should close it")
	}
}

func TestSettingsListTabClick(t *testing.T) {
	styles.RegisterDefaults()
	styles.Set("dark")
	s := New(Deps{})
	s.tab = tabPlugins
	s.tabs.SetActive(int(tabPlugins))
	drawSettings(t, 100, 30, s)
	content := s.contentRegion()

	if !s.HandleMouse(content.Left+2, content.Top, tcell.ButtonPrimary) {
		t.Fatal("click on a list item should be handled")
	}
}
