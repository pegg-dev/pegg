package components

import (
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"

	"github.com/peggco/pegg/internal/llm"
	"github.com/peggco/pegg/internal/tui/layout"
	"github.com/peggco/pegg/internal/tui/styles"
)

func TestChatAppendAndSelect(t *testing.T) {
	c := NewChat()
	c.AppendItem(&ChatItem{Kind: ItemUser, Text: "hello"})
	c.AppendItem(&ChatItem{Kind: ItemAssistant, Text: "hi"})
	if !c.HasItems() {
		t.Fatal("expected items")
	}
	if len(c.items) != 2 {
		t.Errorf("items = %d, want 2", len(c.items))
	}
	if c.sel() != 1 {
		t.Errorf("selection = %d, want last item", c.sel())
	}
}

func TestChatActivateThinking(t *testing.T) {
	c := NewChat()
	it := &ChatItem{Kind: ItemThinking, Reasoning: "secret reasoning"}
	c.AppendItem(it)
	c.SetOnActivate(func(item *ChatItem) { item.Expanded = !item.Expanded })
	c.rebuildFlat(80)
	if len(c.flatItems) == 0 {
		t.Fatal("no flat items")
	}
	c.itemCursor = 0
	c.HandleKey(tcell.NewEventKey(tcell.KeyEnter, 0, 0))
	if !it.Expanded {
		t.Error("Enter should expand the thinking item")
	}
}

func TestChatPrepend(t *testing.T) {
	c := NewChat()
	c.AppendItem(&ChatItem{Kind: ItemUser, Text: "b"})
	old := len(c.items)
	c.PrependItems([]*ChatItem{{Kind: ItemUser, Text: "a"}})
	if len(c.items) != old+1 {
		t.Errorf("items = %d, want %d", len(c.items), old+1)
	}
	if c.items[0].Text != "a" {
		t.Errorf("first item = %q, want a", c.items[0].Text)
	}
}

func TestChatClickSelects(t *testing.T) {
	c := NewChat()
	var activated *ChatItem
	c.AppendItem(&ChatItem{Kind: ItemUser, Text: "first"})
	thinking := &ChatItem{Kind: ItemThinking}
	c.AppendItem(thinking)
	c.SetOnActivate(func(item *ChatItem) { activated = item })
	c.rebuildFlat(80)
	c.HandleClick(5, 4, 0)
	if activated != thinking {
		t.Errorf("click should activate the thinking item, got %v", activated)
	}
}

func TestChatSetItemsResets(t *testing.T) {
	c := NewChat()
	c.AppendItem(&ChatItem{Kind: ItemUser, Text: "x"})
	c.SetItems([]*ChatItem{{Kind: ItemAssistant, Text: "y"}})
	if len(c.items) != 1 || c.items[0].Text != "y" {
		t.Errorf("items after SetItems = %+v", c.items)
	}
}

func TestChatDrawSmoke(t *testing.T) {
	styles.RegisterDefaults()
	styles.Set("dark")
	s := tcell.NewSimulationScreen("UTF-8")
	if err := s.Init(); err != nil {
		t.Fatal(err)
	}
	defer s.Fini()
	s.SetSize(100, 30)

	c := NewChat()
	c.AppendItem(&ChatItem{Kind: ItemUser, Text: "Please fix the bug in #main"})
	c.AppendItem(&ChatItem{Kind: ItemThinking, Reasoning: "Let me think about this\nstep by step."})
	c.AppendItem(&ChatItem{Kind: ItemAssistant, Text: "I'll look at the code first.\n\n```go\npackage main\n```"})
	c.AppendItem(&ChatItem{Kind: ItemTool, ToolName: "edit", ToolArgs: `{"filePath":"a.go","oldString":"x","newString":"y"}`, Diff: ComputeDiff("x", "y"), ToolOutput: "edited"})
	c.AppendItem(&ChatItem{Kind: ItemSubagent, SubagentName: "developer", SubagentStatus: "running"})
	c.AppendItem(&ChatItem{Kind: ItemFinished})

	c.Draw(s, layout.Region{Left: 0, Top: 0, Width: 100, Height: 28}, true)
	s.Show()
}

func TestChatDrawExpandedSmoke(t *testing.T) {
	styles.RegisterDefaults()
	styles.Set("dark")
	s := tcell.NewSimulationScreen("UTF-8")
	if err := s.Init(); err != nil {
		t.Fatal(err)
	}
	defer s.Fini()
	s.SetSize(100, 30)

	c := NewChat()
	tool := &ChatItem{Kind: ItemTool, ToolName: "edit", ToolArgs: `{"oldString":"a","newString":"b"}`, Diff: ComputeDiff("a\nb", "a\nc"), ToolOutput: "done"}
	tool.Expanded = true
	c.AppendItem(tool)
	think := &ChatItem{Kind: ItemThinking, Reasoning: "thinking text\nmore"}
	think.Expanded = true
	c.AppendItem(think)
	c.Draw(s, layout.Region{Left: 0, Top: 0, Width: 100, Height: 28}, true)
	s.Show()
}

func chatStreamingFixture(t *testing.T, n int) (*Chat, *ChatItem, tcell.SimulationScreen) {
	t.Helper()
	styles.RegisterDefaults()
	styles.Set("dark")
	s := tcell.NewSimulationScreen("UTF-8")
	if err := s.Init(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Fini)
	s.SetSize(100, 30)

	c := NewChat()
	for i := 0; i < n; i++ {
		c.AppendItem(&ChatItem{Kind: ItemUser, Text: "question number " + string(rune('a'+i%26))})
		c.AppendItem(&ChatItem{Kind: ItemAssistant, Text: "answer with **markdown** and `code` and a long line that wraps several times across the available width of the terminal viewport."})
		c.AppendItem(&ChatItem{Kind: ItemTool, ToolName: "bash:echo hi", ToolOutput: "ok"})
	}
	live := &ChatItem{Kind: ItemAssistant, Text: "streaming "}
	c.AppendItem(live)
	c.Draw(s, layout.Region{Left: 0, Top: 0, Width: 100, Height: 28}, true)
	return c, live, s
}

func TestChatStreamingCacheStaysCorrect(t *testing.T) {
	c, live, s := chatStreamingFixture(t, 3)

	for i := 0; i < 10; i++ {
		live.Text += " token"
		c.MarkLastDirty()
		c.Draw(s, layout.Region{Left: 0, Top: 0, Width: 100, Height: 28}, true)
	}
	fi := c.flatItems[len(c.flatItems)-1]
	if fi.End-fi.Start == 0 {
		t.Fatal("live item has no flat lines after streaming draws")
	}

	text := ""
	for _, ln := range c.flat[fi.Start:fi.End] {
		for _, cell := range ln {
			text += string(cell.R)
		}
	}
	if !strings.Contains(text, "streaming token token") {
		t.Errorf("live item content missing from flat: %q", text)
	}

	for i := 0; i < 3; i++ {
		if c.cache[i*3] == nil {
			t.Fatalf("user item %d should be cached after first draw", i)
		}
	}

	c.AppendItem(&ChatItem{Kind: ItemTool, ToolName: "bash:go test ./..."})
	c.Draw(s, layout.Region{Left: 0, Top: 0, Width: 100, Height: 28}, true)
	if c.cache[len(c.cache)-1] != nil {
		t.Fatal("running tool must not be cached")
	}

	thinking := &ChatItem{Kind: ItemThinking, Reasoning: "hidden reasoning body"}
	c.AppendItem(thinking)
	c.Draw(s, layout.Region{Left: 0, Top: 0, Width: 100, Height: 28}, true)
	if len(c.flatItems) == 0 || c.flatItems[len(c.flatItems)-1].End-c.flatItems[len(c.flatItems)-1].Start != 1 {
		t.Fatalf("collapsed thinking should be 1 line, got %d", c.flatItems[len(c.flatItems)-1].End-c.flatItems[len(c.flatItems)-1].Start)
	}
	thinking.Expanded = true
	c.InvalidateItemPtr(thinking)
	c.Draw(s, layout.Region{Left: 0, Top: 0, Width: 100, Height: 28}, true)
	span := c.flatItems[len(c.flatItems)-1].End - c.flatItems[len(c.flatItems)-1].Start
	if span <= 1 {
		t.Fatalf("expanded thinking should wrap to multiple lines, got %d", span)
	}
}

func TestChatLazyLoadStreamingInterplay(t *testing.T) {
	styles.RegisterDefaults()
	styles.Set("dark")
	s := tcell.NewSimulationScreen("UTF-8")
	if err := s.Init(); err != nil {
		t.Fatal(err)
	}
	defer s.Fini()
	s.SetSize(100, 30)
	bounds := layout.Region{Left: 0, Top: 0, Width: 100, Height: 28}

	c := NewChat()
	for i := 0; i < 20; i++ {
		c.AppendItem(&ChatItem{Kind: ItemUser, Text: "recent-" + itoa(i)})
		c.AppendItem(&ChatItem{Kind: ItemAssistant, Text: "answer " + itoa(i)})
	}
	live := &ChatItem{Kind: ItemAssistant, Text: "live "}
	c.AppendItem(live)
	c.Draw(s, bounds, true)
	before := len(c.flat)

	c.PrependItems([]*ChatItem{
		{Kind: ItemUser, Text: "older-a"},
		{Kind: ItemUser, Text: "older-b"},
	})
	if len(c.cache) != len(c.items) {
		t.Fatalf("cache length %d != items length %d after prepend", len(c.cache), len(c.items))
	}
	c.Draw(s, bounds, true)
	if len(c.flat) <= before {
		t.Fatalf("flat did not grow after prepend: %d -> %d", before, len(c.flat))
	}
	if c.flat[0].Width() == 0 {
		t.Fatal("prepended item missing from flat")
	}
	first := c.flatItems[0]
	if c.flatItems[0].ItemIdx != 0 {
		t.Fatalf("first flatItem points at item %d, want 0", first.ItemIdx)
	}

	for i := 0; i < 5; i++ {
		live.Text += " tok"
		c.MarkLastDirty()
		c.Draw(s, bounds, true)
	}
	if len(c.cache) != len(c.items) {
		t.Fatalf("cache length %d != items length %d after streaming", len(c.cache), len(c.items))
	}
	fi := c.flatItems[len(c.flatItems)-1]
	text := ""
	for _, ln := range c.flat[fi.Start:fi.End] {
		for _, cell := range ln {
			text += string(cell.R)
		}
	}
	if !strings.Contains(text, "live tok tok") {
		t.Errorf("streamed content missing after lazy load: %q", text)
	}
	oldIdx := 2
	if c.flatItems[oldIdx].ItemIdx != 2 {
		t.Fatalf("flatItem %d points at item %d, want 2", oldIdx, c.flatItems[oldIdx].ItemIdx)
	}
	oldText := ""
	for _, ln := range c.flat[c.flatItems[oldIdx].Start:c.flatItems[oldIdx].End] {
		for _, cell := range ln {
			oldText += string(cell.R)
		}
	}
	if !strings.Contains(oldText, "recent-0") {
		t.Errorf("cached item content changed after lazy load: %q", oldText)
	}
}

func TestChatViewSwitchNoResidue(t *testing.T) {
	styles.RegisterDefaults()
	styles.Set("dark")
	s := tcell.NewSimulationScreen("UTF-8")
	if err := s.Init(); err != nil {
		t.Fatal(err)
	}
	defer s.Fini()
	s.SetSize(100, 30)
	bounds := layout.Region{Left: 1, Top: 1, Width: 98, Height: 22}

	main := []*ChatItem{
		{Kind: ItemUser, Text: "Fix the parser bug"},
		{Kind: ItemThinking, Reasoning: "looking at the code"},
		{Kind: ItemTool, ToolName: "todowrite:3 items", ToolOutput: "[ ] fix parser\n[ ] add tests\n[ ] commit"},
		{Kind: ItemSubagent, AgentID: "sub-1", SubagentName: "developer", SubagentStatus: "finished", SubagentTask: "Refactor the parser", SubagentOutput: "Done.\n```\n}\n```"},
		{Kind: ItemAssistant, Text: "Refactored."},
		{Kind: ItemFinished, ID: "main"},
	}
	sub := []*ChatItem{
		{Kind: ItemUser, Text: "Refactor the parser"},
		{Kind: ItemThinking, Reasoning: "analyzing"},
		{Kind: ItemTool, ToolName: "read:parser.go", ToolOutput: "package parser\nfunc parse() {\n}"},
		{Kind: ItemTool, ToolName: "edit:parser.go", ToolOutput: "done"},
		{Kind: ItemAssistant, Text: "Here is the refactor:\n\n```go\nfunc (p *Parser) Run() {\n\tresult, err := p.parse()\n}\n```"},
		{Kind: ItemFinished, ID: "sub-1"},
	}

	c := NewChat()
	c.SetItems(main)
	c.Draw(s, bounds, true)
	before := screenContent(s, 100, 30)

	c.SetItems(sub)
	c.SetBack(true)
	c.Draw(s, bounds, true)

	c.SetItems(main)
	c.SetBack(false)
	c.Draw(s, bounds, true)
	after := screenContent(s, 100, 30)

	if before != after {
		t.Fatalf("residue left on screen after subagent view switch:\n-- before --\n%s\n-- after --\n%s", before, after)
	}
}

func TestChatBackHeaderAccountsForHeaderRow(t *testing.T) {
	styles.RegisterDefaults()
	styles.Set("dark")
	s := tcell.NewSimulationScreen("UTF-8")
	if err := s.Init(); err != nil {
		t.Fatal(err)
	}
	defer s.Fini()
	s.SetSize(100, 30)
	bounds := layout.Region{Left: 0, Top: 0, Width: 100, Height: 28}

	c := NewChat()
	for i := 0; i < 15; i++ {
		c.AppendItem(&ChatItem{Kind: ItemUser, Text: "question " + itoa(i)})
		c.AppendItem(&ChatItem{Kind: ItemAssistant, Text: "answer " + itoa(i)})
	}

	c.Draw(s, bounds, true)
	total := len(c.flat)
	if c.scroll != total-bounds.Height {
		t.Fatalf("scroll without back header = %d, want %d", c.scroll, total-bounds.Height)
	}
	if c.lastVisible != bounds.Height {
		t.Fatalf("lastVisible without header = %d, want %d", c.lastVisible, bounds.Height)
	}

	c.SetBack(true)
	c.Draw(s, bounds, true)
	want := total - (bounds.Height - 1)
	if c.scroll != want {
		t.Fatalf("scroll with back header = %d, want %d (header row must not shrink the viewport)", c.scroll, want)
	}
	if c.lastVisible != bounds.Height-1 {
		t.Fatalf("lastVisible with header = %d, want %d", c.lastVisible, bounds.Height-1)
	}
}

func screenContent(s tcell.Screen, w, h int) string {
	var b strings.Builder
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			mainc, _, _, _ := s.GetContent(x, y)
			b.WriteRune(mainc)
		}
		b.WriteByte('\n')
	}
	return b.String()
}

func TestChatLinesNeverExceedWidth(t *testing.T) {
	styles.RegisterDefaults()
	styles.Set("dark")
	width := 96
	items := []*ChatItem{
		{Kind: ItemUser, Text: "hello"},
		{Kind: ItemUser, Text: "hello", Attachments: []llm.Attachment{{Type: llm.AttachmentTypeFile, FileName: strings.Repeat("verylongfilename", 6)}}},
		{Kind: ItemAssistant, Text: "```go\n" + strings.Repeat("x := someVeryLongFunctionName(argumentOne, argumentTwo) // comment\n", 3) + "```"},
		{Kind: ItemTool, ToolName: "edit:" + strings.Repeat("a/", 80) + "file.go", ToolArgs: `{"oldString":"a","newString":"b"}`, Diff: ComputeDiff("a", "b"), ToolOutput: "ok"},
		{Kind: ItemTool, ToolName: "write:" + strings.Repeat("a/", 80) + "file.go", WriteContent: "package main\nfunc main() {}\n", ToolOutput: "ok"},
		{Kind: ItemTool, ToolName: "bash:" + strings.Repeat("echo hi && ", 30), ToolOutput: strings.Repeat("output line\n", 3)},
		{Kind: ItemTool, ToolName: "todowrite:3 items", ToolOutput: "[ ] task one\n[~] task two\n[x] task three"},
		{Kind: ItemTool, ToolName: "read:" + strings.Repeat("a/", 80) + "file.go", ToolOutput: strings.Repeat("line of output\n", 3)},
		{Kind: ItemSubagent, AgentID: "s1", SubagentName: strings.Repeat("verylongsubagentname", 6), SubagentStatus: "finished", SubagentTask: strings.Repeat("do the thing ", 20), SubagentOutput: strings.Repeat("output\n", 5)},
		{Kind: ItemThinking, Reasoning: strings.Repeat("reasoning words ", 30), Expanded: true},
	}
	c := NewChat()
	for _, it := range items {
		c.AppendItem(it)
	}
	c.rebuildFlat(width)
	for i, ln := range c.flat {
		if ln == nil {
			continue
		}
		if ln.Width() > width {
			t.Errorf("flat line %d width=%d > %d: %q", i, ln.Width(), width, lineText(ln))
		}
	}
}

func lineText(l Line) string {
	var b strings.Builder
	for _, c := range l {
		b.WriteRune(c.R)
	}
	return b.String()
}

func TestChatErrorWrapsLongMessage(t *testing.T) {
	styles.RegisterDefaults()
	styles.Set("dark")
	c := NewChat()
	it := &ChatItem{Kind: ItemError, Text: strings.Repeat("boom ", 10)}
	lines := c.itemLines(it, 20)
	if len(lines) < 2 {
		t.Fatalf("expected the error to wrap, got %d line(s)", len(lines))
	}
	for _, l := range lines {
		if l.Width() > 20 {
			t.Fatalf("error line %q exceeds width 20", lineText(l))
		}
	}
	joined := ""
	for _, l := range lines {
		joined += lineText(l)
	}
	if strings.Count(joined, "boom") != 10 {
		t.Fatalf("error text lost while wrapping: %q", joined)
	}
}

func BenchmarkChatStreamingDraw(b *testing.B) {
	styles.RegisterDefaults()
	styles.Set("dark")
	s := tcell.NewSimulationScreen("UTF-8")
	if err := s.Init(); err != nil {
		b.Fatal(err)
	}
	defer s.Fini()
	s.SetSize(120, 40)

	bounds := layout.Region{Left: 0, Top: 0, Width: 120, Height: 40}
	run := func(b *testing.B, items int) {
		c := NewChat()
		for i := 0; i < items; i++ {
			c.AppendItem(&ChatItem{Kind: ItemUser, Text: "question " + strings.Repeat("x", 40)})
			c.AppendItem(&ChatItem{Kind: ItemAssistant, Text: "answer with **markdown** and `code` and a long line that wraps several times across the available width of the terminal viewport."})
			c.AppendItem(&ChatItem{Kind: ItemTool, ToolName: "bash:echo hi", ToolOutput: "ok"})
		}
		c.Draw(s, bounds, true)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			c.items[len(c.items)-1].Text += " tok"
			c.MarkLastDirty()
			c.Draw(s, bounds, true)
		}
	}
	b.Run("conversation=10items", func(b *testing.B) { run(b, 10) })
	b.Run("conversation=200items", func(b *testing.B) { run(b, 200) })
	b.Run("conversation=1000items", func(b *testing.B) { run(b, 1000) })
}
