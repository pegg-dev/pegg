package home

import (
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"

	"github.com/peggco/pegg/internal/llm"
	"github.com/peggco/pegg/internal/tui/components"
	"github.com/peggco/pegg/internal/tui/layout"
	"github.com/peggco/pegg/internal/tui/styles"
)

func drawFrame(t *testing.T, w, h int) *Page {
	t.Helper()
	styles.RegisterDefaults()
	styles.Set("dark")
	s := tcell.NewSimulationScreen("UTF-8")
	if err := s.Init(); err != nil {
		t.Fatalf("init simulation screen: %v", err)
	}
	t.Cleanup(s.Fini)
	s.SetSize(w, h)

	p := New()
	p.Draw(s, layout.Region{Left: 0, Top: 0, Width: w, Height: h}, true)
	s.Show()
	return p
}

func TestHomeDrawSmoke(t *testing.T) {
	for _, size := range [][2]int{
		{80, 24},
		{120, 40},
		{40, 12},
		{200, 50},
	} {
		drawFrame(t, size[0], size[1])
	}
}

func TestHomeDrawWithContent(t *testing.T) {
	p := drawFrame(t, 100, 30)
	for _, r := range "fix the bug now, please" {
		p.Input().InsertRune(r)
	}
	p.Draw(drawScreen(t, 100, 30), layout.Region{Left: 0, Top: 0, Width: 100, Height: 30}, true)
}

func TestHomeDrawManyLines(t *testing.T) {
	p := drawFrame(t, 100, 30)
	for i := 0; i < 12; i++ {
		p.Input().Newline()
	}
	p.Draw(drawScreen(t, 100, 30), layout.Region{Left: 0, Top: 0, Width: 100, Height: 30}, true)
	if p.Input().VisibleRows() != 6 {
		t.Errorf("VisibleRows = %d, want 6", p.Input().VisibleRows())
	}
}

func TestHomeEscLeavesSubagentViewRegardlessOfFocus(t *testing.T) {
	p := New()
	p.Chat().SetItems([]*components.ChatItem{{Kind: components.ItemUser, Text: "task"}})
	p.Chat().SetBack(true)
	back := false
	p.Chat().SetOnBack(func() { back = true })

	if !p.HandleKey(tcell.NewEventKey(tcell.KeyEsc, 0, 0)) {
		t.Fatal("Esc should be handled")
	}
	if !back {
		t.Fatal("Esc should navigate back from the subagent view")
	}
}

func drawScreen(t *testing.T, w, h int) tcell.Screen {
	t.Helper()
	s := tcell.NewSimulationScreen("UTF-8")
	if err := s.Init(); err != nil {
		t.Fatalf("init simulation screen: %v", err)
	}
	t.Cleanup(s.Fini)
	s.SetSize(w, h)
	return s
}

func screenText(s tcell.Screen, w, h int) string {
	var b strings.Builder
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			r, _, _, _ := s.GetContent(x, y)
			b.WriteRune(r)
		}
		b.WriteByte('\n')
	}
	return b.String()
}

func TestHomeShowsSingleHintAboveInput(t *testing.T) {
	styles.RegisterDefaults()
	styles.Set("dark")
	p := New()
	p.hint = "use '@' to mention files, folders, or agents"
	s := drawScreen(t, 100, 30)
	p.Draw(s, layout.Region{Left: 0, Top: 0, Width: 100, Height: 30}, true)

	rows := strings.Split(screenText(s, 100, 30), "\n")
	hintRow, inputTop := -1, -1
	for i, r := range rows {
		if strings.Contains(r, "• use '@' to mention files, folders, or agents") {
			hintRow = i
		}
		if inputTop < 0 && strings.Contains(r, "┌") {
			inputTop = i
		}
	}
	if hintRow < 0 {
		t.Fatalf("hint not found in:\n%s", strings.Join(rows, "\n"))
	}
	if inputTop < 0 {
		t.Fatal("input box not found")
	}
	if hintRow != inputTop-1 {
		t.Fatalf("hint row = %d, want %d (directly above the input)", hintRow, inputTop-1)
	}
}

func TestHomeHidesHintWithAttachments(t *testing.T) {
	styles.RegisterDefaults()
	styles.Set("dark")
	p := New()
	p.hint = "use '@' to mention files, folders, or agents"
	p.AttachmentBar().Add(llm.Attachment{Type: llm.AttachmentTypeFile, FileName: "main.go"})
	s := drawScreen(t, 100, 30)
	p.Draw(s, layout.Region{Left: 0, Top: 0, Width: 100, Height: 30}, true)

	if strings.Contains(screenText(s, 100, 30), "to mention files") {
		t.Fatal("hint should be hidden when attachments are present")
	}
}

func TestHomeHidesHintWhileTyping(t *testing.T) {
	styles.RegisterDefaults()
	styles.Set("dark")
	p := New()
	p.hint = "use '@' to mention files, folders, or agents"
	p.Input().InsertRune('h')
	s := drawScreen(t, 100, 30)
	p.Draw(s, layout.Region{Left: 0, Top: 0, Width: 100, Height: 30}, true)

	if strings.Contains(screenText(s, 100, 30), "to mention files") {
		t.Fatal("hint should be hidden while the input is not empty")
	}
}

func TestHomeHidesHintWhenChatHasContent(t *testing.T) {
	styles.RegisterDefaults()
	styles.Set("dark")
	p := New()
	p.hint = "use '@' to mention files, folders, or agents"
	p.Chat().SetItems([]*components.ChatItem{{Kind: components.ItemUser, Text: "hi"}})
	s := drawScreen(t, 100, 30)
	p.Draw(s, layout.Region{Left: 0, Top: 0, Width: 100, Height: 30}, true)

	if strings.Contains(screenText(s, 100, 30), "to mention files") {
		t.Fatal("hint should not be shown once the chat has content")
	}
}

func TestRandomHintIsFromPool(t *testing.T) {
	for i := 0; i < 50; i++ {
		h := randomHint()
		found := false
		for _, want := range defaultHints {
			if h == want {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("randomHint returned %q which is not in the pool", h)
		}
	}
}

func TestHomeDrawWithSelection(t *testing.T) {
	p := drawFrame(t, 100, 30)
	for _, r := range "select me" {
		p.Input().InsertRune(r)
	}
	p.Input().Home()
	p.Input().ShiftWordRight()
	if !p.Input().HasSelection() {
		t.Fatal("expected a selection")
	}
	p.Draw(drawScreen(t, 100, 30), layout.Region{Left: 0, Top: 0, Width: 100, Height: 30}, true)
}

func TestHomeBlinkTick(t *testing.T) {
	p := drawFrame(t, 100, 30)
	p.OnTick(false)
	p.OnTick(true)
	p.Draw(drawScreen(t, 100, 30), layout.Region{Left: 0, Top: 0, Width: 100, Height: 30}, true)
}

func TestHomeSkillPickerFlow(t *testing.T) {
	p := New()
	p.SetSkills([]components.ListItem{
		{Label: "go-development", Detail: "write Go"},
		{Label: "websearch", Detail: "search"},
	})
	p.HandleKey(tcell.NewEventKey(tcell.KeyRune, '/', 0))
	if active, query := p.Input().SlashQuery(); !active || query != "" {
		t.Fatalf("SlashQuery = (%v,%q), want (true,\"\")", active, query)
	}
	for _, r := range []rune("go") {
		p.HandleKey(tcell.NewEventKey(tcell.KeyRune, r, 0))
	}
	if !p.PickOpen() {
		t.Fatal("picker should be open in slash mode")
	}
	p.HandleKey(tcell.NewEventKey(tcell.KeyDown, 0, 0))
	p.HandleKey(tcell.NewEventKey(tcell.KeyEnter, 0, 0))
	if p.PickOpen() {
		t.Error("picker should close after selection")
	}
	if got := p.Input().Value(); got != "/go-development " {
		t.Errorf("Value = %q, want '/go-development ' (chip + auto space)", got)
	}
	if p.Input().Col() != 2 {
		t.Errorf("col = %d, want 2 (after chip + space)", p.Input().Col())
	}
}

func TestHomeSkillPickerEscCloses(t *testing.T) {
	p := New()
	p.SetSkills([]components.ListItem{{Label: "go-development", Detail: ""}})
	p.HandleKey(tcell.NewEventKey(tcell.KeyRune, '/', 0))
	p.HandleKey(tcell.NewEventKey(tcell.KeyRune, 'g', 0))
	if !p.PickOpen() {
		t.Fatal("picker should be open")
	}
	p.HandleKey(tcell.NewEventKey(tcell.KeyEsc, 0, 0))
	if p.PickOpen() {
		t.Error("Esc should close the picker")
	}
	if got := p.Input().Value(); got != "/g" {
		t.Errorf("Value = %q, want /g (query text kept)", got)
	}
}

func TestHomeSkillPickerDraw(t *testing.T) {
	styles.RegisterDefaults()
	styles.Set("dark")
	p := New()
	p.SetSkills([]components.ListItem{
		{Label: "go-development", Detail: "write Go"},
		{Label: "websearch", Detail: "search"},
	})
	p.HandleKey(tcell.NewEventKey(tcell.KeyRune, '/', 0))
	p.HandleKey(tcell.NewEventKey(tcell.KeyRune, 'g', 0))
	s := drawScreen(t, 100, 30)
	p.Draw(s, layout.Region{Left: 0, Top: 0, Width: 100, Height: 30}, true)
	s.Show()
}

func TestHomeChipDraw(t *testing.T) {
	styles.RegisterDefaults()
	styles.Set("dark")
	p := New()
	p.Input().InsertChip("go-development")
	p.Input().InsertRune('x')
	s := drawScreen(t, 100, 30)
	p.Draw(s, layout.Region{Left: 0, Top: 0, Width: 100, Height: 30}, true)
	s.Show()
}

func TestHomeSkillPickerClosesAfterDeletingSlash(t *testing.T) {
	styles.RegisterDefaults()
	styles.Set("dark")
	p := New()
	p.SetSkills([]components.ListItem{{Label: "go-development", Detail: ""}})

	p.HandleKey(tcell.NewEventKey(tcell.KeyRune, '/', 0))
	p.HandleKey(tcell.NewEventKey(tcell.KeyRune, 'g', 0))
	if !p.PickOpen() {
		t.Fatal("picker should be open in slash mode")
	}
	p.HandleKey(tcell.NewEventKey(tcell.KeyBackspace, 0, 0))
	if !p.PickOpen() {
		t.Fatal("picker should remain open while '/' remains")
	}
	p.HandleKey(tcell.NewEventKey(tcell.KeyBackspace, 0, 0))
	p.Draw(drawScreen(t, 100, 30), layout.Region{Left: 0, Top: 0, Width: 100, Height: 30}, true)
	if p.PickOpen() {
		t.Error("picker should close after deleting '/'")
	}
	if p.Input().Value() != "" {
		t.Errorf("input = %q, want empty", p.Input().Value())
	}
}

func TestHomeSkillPickerMidSentence(t *testing.T) {
	p := New()
	p.SetSkills([]components.ListItem{{Label: "go-development", Detail: ""}})
	for _, r := range []rune("fix it /go") {
		p.HandleKey(tcell.NewEventKey(tcell.KeyRune, r, 0))
	}
	if !p.PickOpen() {
		t.Fatal("picker should open mid-sentence after a space + '/'")
	}
	p.HandleKey(tcell.NewEventKey(tcell.KeyEnter, 0, 0))
	if p.PickOpen() {
		t.Error("picker should close after selection")
	}
	if got := p.Input().Value(); got != "fix it /go-development " {
		t.Errorf("Value = %q, want 'fix it /go-development ' (auto space at EOL)", got)
	}
}

func TestHomeSkillPickerNotTriggeredByWordSlash(t *testing.T) {
	p := New()
	p.SetSkills([]components.ListItem{{Label: "go-development", Detail: ""}})
	for _, r := range []rune("a/b path") {
		p.HandleKey(tcell.NewEventKey(tcell.KeyRune, r, 0))
	}
	if p.PickOpen() {
		t.Error("picker should not open for a slash inside a word/path")
	}
}

func TestHomeSkillPickerSpaceClosesAndContinues(t *testing.T) {
	styles.RegisterDefaults()
	styles.Set("dark")
	p := New()
	p.SetSkills([]components.ListItem{{Label: "go-development", Detail: ""}})
	for _, r := range []rune("/go") {
		p.HandleKey(tcell.NewEventKey(tcell.KeyRune, r, 0))
	}
	if !p.PickOpen() {
		t.Fatal("picker should be open")
	}
	p.HandleKey(tcell.NewEventKey(tcell.KeyRune, ' ', 0))
	p.Draw(drawScreen(t, 100, 30), layout.Region{Left: 0, Top: 0, Width: 100, Height: 30}, true)
	if p.PickOpen() {
		t.Error("picker should close after space")
	}
	for _, r := range []rune("and more") {
		p.HandleKey(tcell.NewEventKey(tcell.KeyRune, r, 0))
	}
	p.Draw(drawScreen(t, 100, 30), layout.Region{Left: 0, Top: 0, Width: 100, Height: 30}, true)
	if p.PickOpen() {
		t.Error("picker should stay closed after dismissal")
	}
	if got := p.Input().Value(); got != "/go and more" {
		t.Errorf("Value = %q, want '/go and more'", got)
	}
}

func TestHomeSkillPickerReopensWithNewToken(t *testing.T) {
	p := New()
	p.SetSkills([]components.ListItem{{Label: "go-development", Detail: ""}})
	for _, r := range []rune("/go ") {
		p.HandleKey(tcell.NewEventKey(tcell.KeyRune, r, 0))
	}
	p.Draw(drawScreen(t, 100, 30), layout.Region{Left: 0, Top: 0, Width: 100, Height: 30}, true)
	if p.PickOpen() {
		t.Fatal("picker should be dismissed after space")
	}
	for _, r := range []rune("/web") {
		p.HandleKey(tcell.NewEventKey(tcell.KeyRune, r, 0))
	}
	p.Draw(drawScreen(t, 100, 30), layout.Region{Left: 0, Top: 0, Width: 100, Height: 30}, true)
	if !p.PickOpen() {
		t.Error("a new '/' token should reopen the picker")
	}
}

func TestHomeSkillPickerRendersSkills(t *testing.T) {
	styles.RegisterDefaults()
	styles.Set("dark")
	p := New()
	p.SetSkills([]components.ListItem{
		{Label: "go-development", Detail: "write go"},
		{Label: "websearch", Detail: ""},
	})
	p.HandleKey(tcell.NewEventKey(tcell.KeyRune, '/', 0))
	p.HandleKey(tcell.NewEventKey(tcell.KeyRune, 'g', 0))
	s := drawScreen(t, 100, 30)
	p.Draw(s, layout.Region{Left: 0, Top: 0, Width: 100, Height: 30}, true)
	s.Show()
	if !cellContains(s, 100, 30, "go-development") {
		t.Error("skill item not rendered in the picker")
	}
	if !cellContains(s, 100, 30, "Skills") {
		t.Error("picker title not rendered")
	}
}

func cellContains(s tcell.Screen, w, h int, needle string) bool {
	for y := 0; y < h; y++ {
		var row []rune
		for x := 0; x < w; x++ {
			ch, _, _, _ := s.GetContent(x, y)
			row = append(row, ch)
		}
		if strings.Contains(string(row), needle) {
			return true
		}
	}
	return false
}

func TestHomeMentionPickerFlow(t *testing.T) {
	p := New()
	p.SetMentionItems([]components.ListItem{
		{Label: "developer", Detail: "software engineer"},
		{Label: "explorer", Detail: "codebase search"},
	})
	p.HandleKey(tcell.NewEventKey(tcell.KeyRune, '@', 0))
	if active, query := p.Input().AtQuery(); !active || query != "" {
		t.Fatalf("AtQuery = (%v,%q), want (true,\"\")", active, query)
	}
	for _, r := range []rune("dev") {
		p.HandleKey(tcell.NewEventKey(tcell.KeyRune, r, 0))
	}
	if !p.PickOpen() {
		t.Fatal("mention picker should be open in @ mode")
	}
	p.HandleKey(tcell.NewEventKey(tcell.KeyDown, 0, 0))
	p.HandleKey(tcell.NewEventKey(tcell.KeyEnter, 0, 0))
	if p.PickOpen() {
		t.Error("picker should close after selection")
	}
	if got := p.Input().Value(); got != "@developer " {
		t.Errorf("Value = %q, want '@developer ' (mention chip + auto space)", got)
	}
	if p.Input().Col() != 2 {
		t.Errorf("col = %d, want 2 (after chip + space)", p.Input().Col())
	}
}

func TestHomeMentionPickerEscCloses(t *testing.T) {
	p := New()
	p.SetMentionItems([]components.ListItem{{Label: "developer", Detail: ""}})
	p.HandleKey(tcell.NewEventKey(tcell.KeyRune, '@', 0))
	p.HandleKey(tcell.NewEventKey(tcell.KeyRune, 'd', 0))
	if !p.PickOpen() {
		t.Fatal("mention picker should be open")
	}
	p.HandleKey(tcell.NewEventKey(tcell.KeyEsc, 0, 0))
	if p.PickOpen() {
		t.Error("Esc should close the mention picker")
	}
	if got := p.Input().Value(); got != "@d" {
		t.Errorf("Value = %q, want @d (query text kept)", got)
	}
}

func TestHomeMentionPickerNotTriggeredByEmail(t *testing.T) {
	p := New()
	p.SetMentionItems([]components.ListItem{{Label: "developer", Detail: ""}})
	for _, r := range []rune("user@example.com") {
		p.HandleKey(tcell.NewEventKey(tcell.KeyRune, r, 0))
	}
	if p.PickOpen() {
		t.Error("picker should not open for an @ inside a word/email")
	}
}

func TestHomeMentionChipDraw(t *testing.T) {
	styles.RegisterDefaults()
	styles.Set("dark")
	p := New()
	p.Input().InsertMention("developer")
	p.Input().InsertRune('x')
	s := drawScreen(t, 100, 30)
	p.Draw(s, layout.Region{Left: 0, Top: 0, Width: 100, Height: 30}, true)
	s.Show()
}

func TestHomeMentionPickerSpaceClosesAndContinues(t *testing.T) {
	styles.RegisterDefaults()
	styles.Set("dark")
	p := New()
	p.SetMentionItems([]components.ListItem{{Label: "developer", Detail: ""}})
	for _, r := range []rune("@dev") {
		p.HandleKey(tcell.NewEventKey(tcell.KeyRune, r, 0))
	}
	if !p.PickOpen() {
		t.Fatal("mention picker should be open")
	}
	p.HandleKey(tcell.NewEventKey(tcell.KeyRune, ' ', 0))
	p.Draw(drawScreen(t, 100, 30), layout.Region{Left: 0, Top: 0, Width: 100, Height: 30}, true)
	if p.PickOpen() {
		t.Error("picker should close after space")
	}
	for _, r := range []rune("and more") {
		p.HandleKey(tcell.NewEventKey(tcell.KeyRune, r, 0))
	}
	p.Draw(drawScreen(t, 100, 30), layout.Region{Left: 0, Top: 0, Width: 100, Height: 30}, true)
	if p.PickOpen() {
		t.Error("picker should stay closed after dismissal")
	}
	if got := p.Input().Value(); got != "@dev and more" {
		t.Errorf("Value = %q, want '@dev and more'", got)
	}
}

func TestHomeMentionPickerReopensWithNewToken(t *testing.T) {
	p := New()
	p.SetMentionItems([]components.ListItem{
		{Label: "developer", Detail: ""},
		{Label: "explorer", Detail: ""},
	})
	for _, r := range []rune("@dev ") {
		p.HandleKey(tcell.NewEventKey(tcell.KeyRune, r, 0))
	}
	p.Draw(drawScreen(t, 100, 30), layout.Region{Left: 0, Top: 0, Width: 100, Height: 30}, true)
	if p.PickOpen() {
		t.Fatal("picker should be dismissed after space")
	}
	for _, r := range []rune("@exp") {
		p.HandleKey(tcell.NewEventKey(tcell.KeyRune, r, 0))
	}
	p.Draw(drawScreen(t, 100, 30), layout.Region{Left: 0, Top: 0, Width: 100, Height: 30}, true)
	if !p.PickOpen() {
		t.Error("a new '@' token should reopen the mention picker")
	}
}

func TestHomeMentionPickerRendersItems(t *testing.T) {
	styles.RegisterDefaults()
	styles.Set("dark")
	p := New()
	p.SetMentionItems([]components.ListItem{
		{Label: "developer", Detail: "write code"},
		{Label: "explorer", Detail: ""},
	})
	p.HandleKey(tcell.NewEventKey(tcell.KeyRune, '@', 0))
	p.HandleKey(tcell.NewEventKey(tcell.KeyRune, 'd', 0))
	s := drawScreen(t, 100, 30)
	p.Draw(s, layout.Region{Left: 0, Top: 0, Width: 100, Height: 30}, true)
	s.Show()
	if !cellContains(s, 100, 30, "developer") {
		t.Error("mention item not rendered in the picker")
	}
	if !cellContains(s, 100, 30, "Mentions") {
		t.Error("mention picker title not rendered")
	}
}

func TestHomeMentionPickerMidSentence(t *testing.T) {
	p := New()
	p.SetMentionItems([]components.ListItem{{Label: "developer", Detail: ""}})
	for _, r := range []rune("ask @dev") {
		p.HandleKey(tcell.NewEventKey(tcell.KeyRune, r, 0))
	}
	if !p.PickOpen() {
		t.Fatal("picker should open mid-sentence after a space + '@'")
	}
	p.HandleKey(tcell.NewEventKey(tcell.KeyEnter, 0, 0))
	if p.PickOpen() {
		t.Error("picker should close after selection")
	}
	if got := p.Input().Value(); got != "ask @developer " {
		t.Errorf("Value = %q, want 'ask @developer ' (auto space at EOL)", got)
	}
}
