package components

import (
	"fmt"
	"strings"
	"time"

	json "github.com/goccy/go-json"

	"github.com/gdamore/tcell/v2"
	"github.com/peggco/pegg/internal/llm"
	"github.com/peggco/pegg/internal/tui/layout"
	"github.com/peggco/pegg/internal/tui/styles"
)

type ItemKind int

const (
	ItemUser ItemKind = iota
	ItemAssistant
	ItemThinking
	ItemTool
	ItemSubagent
	ItemFinished
	ItemError
	ItemCompaction
	ItemCompactionDivider
)

type ChatItem struct {
	Kind ItemKind
	ID   string

	MsgID        string
	MsgSessionID string

	Text      string
	Reasoning string

	ToolName     string
	ToolArgs     string
	ToolOutput   string
	ToolErr      string
	Diff         []DiffHunk
	WriteContent string

	AgentID          string
	SubagentName     string
	SubagentStatus   string
	SubagentTask     string
	SubagentOutput   string
	SubagentUsage    llm.Usage
	SubagentActivity string

	Attachments []llm.Attachment

	Duration time.Duration
	Expanded bool
}

type flatItem struct {
	ItemIdx int
	Start   int
	End     int
}

type Chat struct {
	items       []*ChatItem
	scroll      int
	autoScroll  bool
	wasAtBottom bool
	back        bool
	hasMore     bool

	onActivate        func(*ChatItem)
	onBack            func()
	onLoadMore        func()
	onSubagentHistory func(agentID string)

	flat       []Line
	flatItems  []flatItem
	flatRev    int
	itemCursor int
	follow     bool
	now        time.Duration

	indicatorX0, indicatorX1, indicatorY int
	indicatorVisible                     bool
	backX0, backX1, backY                int
	backVisible                          bool

	historyButtons map[int]string

	lastWidth   int
	lastVisible int

	cache      [][]Line
	cacheWidth int
	dirty      bool

	selActive  bool
	selStart   selPos
	selEnd     selPos
	mouseDown  bool
	mouseStart selPos
	dragged    bool
}

type selPos struct {
	line int
	cell int
}

func NewChat() *Chat {
	return &Chat{autoScroll: true, lastWidth: -1, follow: true, itemCursor: -1}
}

func (c *Chat) SetItems(items []*ChatItem) {
	c.items = items
	c.cache = make([][]Line, len(items))
	c.sel()
	c.autoScroll = true
	c.follow = true
	c.lastWidth = -1
	c.flatRev = -1
	c.dirty = true
	c.itemCursor = -1
	c.selActive = false
}

func (c *Chat) sel() int {
	return len(c.items) - 1
}

func (c *Chat) AppendItem(it *ChatItem) {
	c.items = append(c.items, it)
	c.cache = append(c.cache, nil)
	if c.wasAtBottom {
		c.autoScroll = true
		c.follow = true
	}
	c.dirty = true
}

func (c *Chat) PrependItems(items []*ChatItem) {
	if len(items) == 0 {
		return
	}
	added := 0
	for _, it := range items {
		added += len(c.itemLines(it, c.lastWidth))
	}
	c.ensureCache()
	c.items = append(items, c.items...)
	c.cache = append(make([][]Line, len(items)), c.cache...)
	c.scroll += added
	c.autoScroll = false
	c.follow = false
	c.dirty = true
}

func (c *Chat) Clear() {
	c.items = nil
	c.cache = nil
	c.cacheWidth = 0
	c.scroll = 0
	c.autoScroll = true
	c.follow = true
	c.lastWidth = -1
	c.flatRev = -1
	c.dirty = true
	c.itemCursor = -1
	c.flat = nil
	c.flatItems = nil
}

func (c *Chat) HasItems() bool { return len(c.items) > 0 }

func (c *Chat) Items() []*ChatItem { return c.items }

func (c *Chat) SetBack(b bool) { c.back = b }

func (c *Chat) HasBack() bool { return c.back }

func (c *Chat) SetHasMore(b bool) { c.hasMore = b }

func (c *Chat) HasMore() bool { return c.hasMore }

func (c *Chat) SetOnActivate(fn func(*ChatItem)) { c.onActivate = fn }

func (c *Chat) SetOnBack(fn func()) { c.onBack = fn }

func (c *Chat) SetOnLoadMore(fn func())              { c.onLoadMore = fn }
func (c *Chat) SetOnSubagentHistory(fn func(string)) { c.onSubagentHistory = fn }

func (c *Chat) Invalidate() {
	c.cache = nil
	c.cacheWidth = 0
	c.dirty = true
	c.lastWidth = -1
	c.flatRev = -1
}

func (c *Chat) ensureCache() {
	if c.cache == nil || len(c.cache) != len(c.items) {
		c.cache = make([][]Line, len(c.items))
	}
}

func (c *Chat) MarkLastDirty() {
	if len(c.items) == 0 {
		return
	}
	c.ensureCache()
	c.cache[len(c.items)-1] = nil
	c.dirty = true
}

func (c *Chat) InvalidateItem(idx int) {
	if idx < 0 || idx >= len(c.items) {
		return
	}
	c.ensureCache()
	c.cache[idx] = nil
	c.dirty = true
}

func (c *Chat) InvalidateItemPtr(it *ChatItem) {
	for i, x := range c.items {
		if x == it {
			c.InvalidateItem(i)
			return
		}
	}
}

func (c *Chat) HandleKey(ev *tcell.EventKey) bool {
	switch ev.Key() {
	case tcell.KeyUp:
		c.scrollBy(-1)
		return true
	case tcell.KeyDown:
		c.scrollBy(1)
		return true
	case tcell.KeyPgUp:
		c.scrollBy(-c.lastVisible)
		return true
	case tcell.KeyPgDn:
		c.scrollBy(c.lastVisible)
		return true
	case tcell.KeyHome:
		c.follow = false
		c.scroll = 0
		return true
	case tcell.KeyEnd:
		c.follow = true
		return true
	case tcell.KeyEnter:
		c.activateCursor()
		return true
	case tcell.KeyEsc:
		if c.back && c.onBack != nil {
			c.onBack()
			return true
		}
		return false
	case tcell.KeyRune:
		switch ev.Rune() {
		case ' ':
			c.activateCursor()
			return true
		case ']':
			c.nextItem()
			return true
		case '[':
			c.prevItem()
			return true
		}
	}
	return false
}

func (c *Chat) scrollBy(delta int) {
	if c.flat == nil || len(c.items) == 0 {
		return
	}
	if c.follow {
		if delta >= 0 {
			return
		}
		off := c.maxScroll() + delta
		c.follow = false
		c.setScroll(off)
		return
	}
	off := c.scroll + delta
	if off >= c.maxScroll() {
		c.follow = true
		return
	}
	if off < 0 && c.hasMore && c.onLoadMore != nil {
		c.onLoadMore()
		return
	}
	c.setScroll(off)
}

func (c *Chat) ScrollBy(delta int) {
	c.scrollBy(delta)
}

func (c *Chat) ScrollUp(page int) {
	c.scrollBy(-page)
}

func (c *Chat) ScrollDown(page int) {
	c.scrollBy(page)
}

func (c *Chat) ScrollToBottom() {
	c.follow = true
	c.autoScroll = true
	c.wasAtBottom = true
}

func (c *Chat) setScroll(off int) {
	c.scroll = ClampScroll(off, c.maxScroll())
}

func (c *Chat) maxScroll() int {
	total := len(c.flat)
	vis := c.lastVisible
	if vis <= 0 {
		return 0
	}
	m := total - vis
	if m < 0 {
		return 0
	}
	return m
}

func (c *Chat) HandleClick(x, y, top int) bool {
	if len(c.items) == 0 || y < top {
		return false
	}

	if c.indicatorVisible && y == c.indicatorY && x >= c.indicatorX0 && x <= c.indicatorX1 {
		c.follow = true
		return true
	}
	if c.backVisible && y == c.backY && x >= c.backX0 && x <= c.backX1 {
		if c.onBack != nil {
			c.onBack()
		}
		return true
	}

	rel := y - top + c.scroll
	if rel < 0 || rel >= len(c.flat) {
		return false
	}
	if c.historyButtons != nil {
		if agentID, ok := c.historyButtons[rel]; ok {
			if c.onSubagentHistory != nil {
				c.onSubagentHistory(agentID)
			}
			return true
		}
	}
	for _, fi := range c.flatItems {
		if rel >= fi.Start && rel < fi.End {
			c.itemCursor = fi.ItemIdx
			if c.onActivate != nil {
				c.onActivate(c.items[fi.ItemIdx])
			}
			return true
		}
	}
	return true
}

func (c *Chat) posAt(x, y, top, left int) (selPos, bool) {
	contentTop := top
	if c.backVisible {
		contentTop++
	}
	line := y - contentTop + c.scroll
	if line < 0 || line >= len(c.flat) {
		return selPos{}, false
	}
	cell := x - (left + 1)
	if cell < 0 {
		cell = 0
	}
	return selPos{line: line, cell: cell}, true
}

func (c *Chat) selectionBounds() (selPos, selPos, bool) {
	if !c.selActive {
		return selPos{}, selPos{}, false
	}
	a, b := c.selStart, c.selEnd
	if a.line > b.line || (a.line == b.line && a.cell > b.cell) {
		a, b = b, a
	}
	return a, b, true
}

func (c *Chat) HasSelection() bool { return c.selActive }

func (c *Chat) ClearSelection() {
	if c.selActive {
		c.selActive = false
		c.dirty = true
	}
}

func (c *Chat) setSelection(a, b selPos) {
	c.selStart, c.selEnd = a, b
	c.selActive = true
	c.dirty = true
}

func (c *Chat) HandleMouse(x, y, top, left int, buttons tcell.ButtonMask) bool {
	pos, ok := c.posAt(x, y, top, left)
	pressed := buttons&tcell.ButtonPrimary != 0

	if pressed {
		if !c.mouseDown {
			c.mouseDown = true
			c.dragged = false
			if ok {
				c.mouseStart = pos
			}
		} else if ok && c.isDrag(pos) {
			c.dragged = true
			c.setSelection(c.mouseStart, pos)
		}
		return true
	}

	if c.mouseDown {
		c.mouseDown = false
		if c.dragged && c.selActive {
			c.dragged = false
			return true
		}
		c.dragged = false
		c.ClearSelection()
		return c.HandleClick(x, y, top)
	}
	return false
}

func (c *Chat) isDrag(pos selPos) bool {
	if pos.line != c.mouseStart.line {
		return true
	}
	d := pos.cell - c.mouseStart.cell
	if d < 0 {
		d = -d
	}
	return d > 1
}

var frameRunes = map[rune]bool{
	'┌': true, '┐': true, '└': true, '┘': true, '─': true, '│': true, '▍': true,
}

func isFrameOnly(cells []Cell) bool {
	for _, c := range cells {
		if !frameRunes[c.R] && c.R != ' ' {
			return false
		}
	}
	return true
}

func sanitizeSelectedCells(cells []Cell) string {
	var b strings.Builder
	for _, c := range cells {
		if frameRunes[c.R] {
			continue
		}
		b.WriteRune(c.R)
	}
	return strings.TrimRight(b.String(), " \t")
}

func (c *Chat) SelectedText() string {
	a, b, ok := c.selectionBounds()
	if !ok || len(c.flat) == 0 {
		return ""
	}
	if a.line >= len(c.flat) {
		return ""
	}
	if b.line >= len(c.flat) {
		b.line = len(c.flat) - 1
		b.cell = len(c.flat[b.line])
	}
	var out []string
	for line := a.line; line <= b.line; line++ {
		cells := c.flat[line]
		from, to := 0, len(cells)
		if line == a.line {
			from = a.cell
		}
		if line == b.line {
			to = b.cell
		}
		if from > to {
			from = to
		}
		if from > len(cells) {
			from = len(cells)
		}
		if to > len(cells) {
			to = len(cells)
		}
		seg := cells[from:to]
		if isFrameOnly(seg) {
			continue
		}
		out = append(out, sanitizeSelectedCells(seg))
	}
	return strings.Join(out, "\n")
}

func (c *Chat) applySelection(idx int, line Line, s, e selPos, th styles.Theme) Line {
	if idx < s.line || idx > e.line {
		return line
	}
	from, to := 0, len(line)
	if idx == s.line {
		from = s.cell
	}
	if idx == e.line {
		to = e.cell
	}
	if from < 0 {
		from = 0
	}
	if from > len(line) {
		from = len(line)
	}
	if to > len(line) {
		to = len(line)
	}
	if from >= to {
		return line
	}
	selStyle := th.Base().Background(th.Selection)
	out := make(Line, len(line))
	copy(out, line)
	for i := from; i < to; i++ {
		out[i].S = selStyle
	}
	return out
}

func (c *Chat) Draw(s tcell.Screen, bounds layout.Region, focused bool) {
	th := styles.Current()
	bg := th.Base().Background(th.Background)
	FillRegion(s, bounds, ' ', bg)

	innerW := bounds.Width - 2
	if innerW < 4 {
		return
	}
	c.lastWidth = innerW

	if c.dirty || c.flatRev != c.lastWidth || c.flat == nil {
		c.rebuildFlat(innerW)
	}

	total := len(c.flat)
	if total == 0 {
		return
	}

	if c.back && bounds.Height > 2 {
		headerStyle := th.Base().Foreground(th.Accent).Bold(true).Background(th.Background)
		backText := "← back to main chat (Esc)"
		DrawText(s, bounds.Left+1, bounds.Top, backText, headerStyle)
		c.backVisible = true
		c.backX0 = bounds.Left + 1
		c.backX1 = bounds.Left + 1 + len([]rune(backText)) - 1
		c.backY = bounds.Top
		bounds.Top++
		bounds.Height--
	} else {
		c.backVisible = false
	}

	c.lastVisible = bounds.Height
	visible := bounds.Height
	if c.follow {
		c.scroll = ClampScroll(total-visible, total)
	}
	if c.scroll < 0 {
		c.scroll = 0
	}
	maxS := c.maxScroll()
	if c.scroll > maxS {
		c.scroll = maxS
	}
	c.wasAtBottom = c.scroll >= maxS-2

	offset := c.scroll

	markerLine := -1
	if focused && c.itemCursor >= 0 && c.itemCursor < len(c.flatItems) {
		markerLine = c.flatItems[c.itemCursor].Start
	}

	selS, selE, hasSel := c.selectionBounds()
	for row := 0; row < visible; row++ {
		idx := offset + row
		if idx >= total {
			break
		}
		x := bounds.Left
		y := bounds.Top + row
		if idx == markerLine {
			s.SetContent(x, y, '▍', nil, th.Base().Foreground(th.Accent).Background(th.Background))
		} else {
			s.SetContent(x, y, ' ', nil, bg)
		}
		line := c.flat[idx]
		if hasSel {
			line = c.applySelection(idx, line, selS, selE, th)
		}
		DrawLineBounded(s, x+1, y, bounds.Left+bounds.Width, line)
	}

	c.indicatorVisible = false
	below := total - (offset + visible)
	if below > 0 {
		label := fmt.Sprintf(" ↓ %d ", below)
		labelCells := LineFromSegments([]Segment{
			{Text: label, Style: th.Base().Foreground(th.Accent).Background(th.Surface)},
		}, len(label))
		lx := bounds.Right() - len(label) - 1
		ly := bounds.Bottom() - 1
		DrawLine(s, lx, ly, labelCells)
		c.indicatorX0, c.indicatorX1, c.indicatorY = lx, lx+len(label)-1, ly
		c.indicatorVisible = true
	}
}

func (c *Chat) setFollow(f bool) {
	if c.follow != f {
		c.follow = f
		if !f {
			c.scroll = c.maxScroll()
		}
	}
}

func (c *Chat) nextItem() {
	if len(c.flatItems) == 0 {
		return
	}
	c.itemCursor++
	if c.itemCursor >= len(c.flatItems) {
		c.itemCursor = 0
	}
	c.revealCursor()
}

func (c *Chat) prevItem() {
	if len(c.flatItems) == 0 {
		return
	}
	c.itemCursor--
	if c.itemCursor < 0 {
		c.itemCursor = len(c.flatItems) - 1
	}
	c.revealCursor()
}

func (c *Chat) revealCursor() {
	if c.flat == nil || c.itemCursor < 0 || c.itemCursor >= len(c.flatItems) {
		return
	}
	item := c.flatItems[c.itemCursor]
	if c.follow {
		c.follow = false
	}
	off := c.scroll
	if item.Start < off {
		c.setScroll(item.Start)
	} else if item.Start >= off+c.lastVisible {
		c.setScroll(item.Start - c.lastVisible + 1)
	}
}

func (c *Chat) activateCursor() {
	if c.itemCursor < 0 || c.itemCursor >= len(c.flatItems) {
		return
	}
	fi := c.flatItems[c.itemCursor]
	if fi.ItemIdx >= 0 && fi.ItemIdx < len(c.items) {
		if c.onActivate != nil {
			c.onActivate(c.items[fi.ItemIdx])
		}
	}
}

func (c *Chat) rebuildFlat(width int) {
	if c.cache == nil || len(c.cache) != len(c.items) || c.cacheWidth != width {
		c.cache = make([][]Line, len(c.items))
		c.cacheWidth = width
	}
	c.flat = nil
	c.flatItems = nil
	c.historyButtons = nil
	total := 0
	for i, it := range c.items {
		live := c.isLiveItem(it)
		var lines []Line
		if cached := c.cache[i]; cached != nil && !live {
			lines = cached
		} else {
			lines = c.itemLines(it, width)
			if !live {
				c.cache[i] = lines
			}
		}
		if len(lines) > 0 {
			c.flatItems = append(c.flatItems, flatItem{
				ItemIdx: i,
				Start:   total,
				End:     total + len(lines),
			})
			if it.Kind == ItemSubagent && it.AgentID != "" {
				if c.historyButtons == nil {
					c.historyButtons = make(map[int]string)
				}
				c.historyButtons[total] = it.AgentID
			}
		}
		c.flat = append(c.flat, lines...)
		c.flat = append(c.flat, nil)
		total += len(lines) + 1
	}
	c.flatRev = width
	c.dirty = false
	if c.itemCursor >= len(c.flatItems) {
		c.itemCursor = -1
	}
}

func (c *Chat) isLiveItem(it *ChatItem) bool {
	switch it.Kind {
	case ItemTool:
		return it.ToolErr == "" && it.ToolOutput == ""
	case ItemSubagent:
		return it.SubagentStatus == "" || it.SubagentStatus == "running"
	case ItemThinking:
		return c.isActiveThinking(it)
	}
	return false
}

func (c *Chat) itemLines(it *ChatItem, width int) []Line {
	if width < 1 {
		width = 80
	}
	var lines []Line
	switch it.Kind {
	case ItemUser:
		lines = c.userCardLines(it, width)
	case ItemAssistant:
		lines = MdToLines(it.Text, width, styles.Current())
	case ItemThinking:
		lines = c.thinkingLines(it, width)
	case ItemTool:
		lines = c.toolLines(it, width)
	case ItemSubagent:
		lines = c.subagentLines(it, width)
	case ItemFinished:
		return nil
	case ItemError:
		th := styles.Current()
		lines = WrapSegments([]Segment{
			{Text: "✖ error: " + it.Text, Style: th.Base().Foreground(th.Error).Bold(true).Background(th.Background)},
		}, width)
	case ItemCompaction:
		th := styles.Current()
		lines = []Line{LineFromSegments([]Segment{
			{Text: "  " + it.Text, Style: th.Base().Foreground(th.Muted).Background(th.Background)},
		}, width)}
	case ItemCompactionDivider:
		th := styles.Current()
		lines = []Line{LineFromSegments([]Segment{
			{Text: "─ " + it.Text + " ─", Style: th.Base().Foreground(th.Muted).Background(th.Background)},
		}, width)}
	}
	return clipLinesToWidth(lines, width)
}

func clipLinesToWidth(lines []Line, width int) []Line {
	for i, l := range lines {
		if l.Width() > width {
			lines[i] = clipLineToWidth(l, width)
		}
	}
	return lines
}

func clipLineToWidth(l Line, width int) Line {
	w := 0
	for i, c := range l {
		cw := cellWidth(c.R)
		if w+cw > width {
			return l[:i]
		}
		w += cw
	}
	return l
}

func (c *Chat) userCardLines(it *ChatItem, width int) []Line {
	th := styles.Current()
	if width < 8 {
		return WrapText(it.Text, th.Base().Foreground(th.InputText).Background(th.UserBg), width)
	}
	border := th.Base().Foreground(th.Border).Background(th.UserBg)

	var lines []Line
	lines = append(lines, cardEdge('┌', '┐', width, border))

	content := WrapText(it.Text, th.Base().Foreground(th.Foreground).Background(th.UserBg), width-4)
	if len(content) == 0 {
		content = []Line{{}}
	}
	for _, ln := range content {
		lines = append(lines, cardRow(ln, width, border, th))
	}
	for _, a := range it.Attachments {
		lines = append(lines, cardRow(attachmentChipLine(a, width-4, th), width, border, th))
	}

	lines = append(lines, cardEdge('└', '┘', width, border))
	return lines
}

func cardEdge(left, right rune, width int, border tcell.Style) Line {
	line := Line{{R: left, S: border}}
	for line.Width() < width-1 {
		line = append(line, Cell{R: '─', S: border})
	}
	line = append(line, Cell{R: right, S: border})
	return line
}

func cardRow(ln Line, width int, border tcell.Style, th styles.Theme) Line {
	bg := th.Base().Foreground(th.Foreground).Background(th.UserBg)
	row := Line{{R: '│', S: border}, {R: ' ', S: bg}}
	row = append(row, ln...)
	for row.Width() < width-1 {
		row = append(row, Cell{R: ' ', S: bg})
	}
	row = append(row, Cell{R: '│', S: border})
	return row
}

func attachmentChipLine(a llm.Attachment, width int, th styles.Theme) Line {
	var icon rune
	switch a.Type {
	case llm.AttachmentTypeImage:
		icon = '🖼'
	case llm.AttachmentTypeAudio:
		icon = '🎵'
	default:
		icon = '📎'
	}
	bg := th.Base().Foreground(th.Foreground).Background(th.UserBg)
	nameStyle := th.Base().Foreground(th.TextDim).Background(th.UserBg)
	line := Line{
		{R: ' ', S: bg},
		{R: icon, S: bg},
		{R: ' ', S: bg},
	}
	name := a.FileName
	for line.Width()+DisplayWidth(name)+4 > width {
		if len(name) <= 1 {
			break
		}
		name = name[:len(name)-1]
	}
	if a.FileName != name {
		name += "…"
	}
	for _, r := range name {
		line = append(line, Cell{R: r, S: nameStyle})
	}
	return line
}

func (c *Chat) thinkingLines(it *ChatItem, width int) []Line {
	th := styles.Current()
	dim := th.Base().Foreground(th.TextDim).Background(th.Background)

	active := c.isActiveThinking(it)
	var mark rune
	var markStyle tcell.Style
	if active {
		mark = '◌'
		markStyle = th.Base().Foreground(th.ThinkingGlow).Background(th.Background)
	} else {
		mark = '▸'
		if it.Expanded {
			mark = '▾'
		}
		markStyle = th.Base().Foreground(th.Accent).Background(th.Background)
	}

	label := "Thinking"
	labelStyle := th.Base().Foreground(th.TextDim).Background(th.Background)
	if active {
		labelStyle = th.Base().Foreground(lerpColor(th.ThinkingDim, th.ThinkingGlow, glowT(c.now))).Background(th.Background)
	}

	line := Line{
		{R: mark, S: markStyle},
		{R: ' ', S: dim},
	}
	for _, r := range label {
		line = append(line, Cell{R: r, S: labelStyle})
	}

	if active {
		dots := int(c.now / (300 * time.Millisecond) % 4)
		for i := 0; i < 4; i++ {
			r := '·'
			if i >= dots {
				r = ' '
			}
			line = append(line, Cell{R: r, S: dim})
		}
	}

	var lines []Line
	lines = append(lines, line)

	if it.Expanded && it.Reasoning != "" {
		reasonStyle := th.Base().Foreground(th.Reasoning).Background(th.Background)
		body := WrapText(it.Reasoning, reasonStyle, width)
		lines = append(lines, body...)
	}
	return lines
}

func (c *Chat) isActiveThinking(it *ChatItem) bool {
	for i, item := range c.items {
		if item == it {
			for j := i + 1; j < len(c.items); j++ {
				if c.items[j].Kind != ItemThinking {
					return false
				}
			}
			return true
		}
	}
	return false
}

func (c *Chat) toolLines(it *ChatItem, width int) []Line {
	th := styles.Current()

	isEdit := strings.HasPrefix(it.ToolName, "edit:") || it.ToolName == "edit"
	isWrite := strings.HasPrefix(it.ToolName, "write:") || it.ToolName == "write"
	isList := strings.Contains(it.ToolName, "todoread") || strings.Contains(it.ToolName, "todowrite")
	isBash := strings.HasPrefix(it.ToolName, "bash:") || it.ToolName == "bash"
	isAsk := it.ToolName == "askuserquestion"
	isRunning := it.ToolErr == "" && it.ToolOutput == ""

	if isRunning {
		left := c.toolStatusLine(it, width, '◌', th.Running, fmt.Sprintf(" %c running", spinnerAt(c.now)))
		return []Line{left}
	}

	if it.ToolErr != "" {
		if it.Expanded {
			left := c.toolStatusLine(it, width, '✖', th.Error, " ✖ "+formatDuration(it.Duration))
			var lines []Line
			lines = append(lines, left)
			bodyStyle := th.Base().Foreground(th.Error).Background(th.Background)
			wrapped := WrapText(it.ToolErr, bodyStyle, width-4)
			for _, ln := range wrapped {
				row := Line{{R: ' ', S: th.Base()}, {R: ' ', S: th.Base()}}
				row = append(row, ln...)
				lines = append(lines, row)
			}
			lines = append(lines, LineFromSegments([]Segment{
				{Text: "  [Enter] Collapse", Style: th.Base().Foreground(th.Muted).Background(th.Background)},
			}, width))
			return lines
		}
		left := c.toolStatusLine(it, width, '✖', th.Error, " ✖ "+formatDuration(it.Duration)+"  [Enter] Expand")
		return []Line{left}
	}

	if isList {
		return c.todoCardLines(it, width)
	}

	if isBash {
		return c.bashCardLines(it, width)
	}

	if isEdit && len(it.Diff) > 0 {
		return c.editCardLines(it, width)
	}
	if isWrite {
		return c.writeCardLines(it, width)
	}
	if isAsk {
		return c.askCardLines(it, width)
	}

	hasOutput := it.ToolOutput != ""
	if it.Expanded && hasOutput {
		left := c.toolStatusLine(it, width, '✔', th.Success, " ✔ "+formatDuration(it.Duration))
		var lines []Line
		lines = append(lines, left)
		bodyStyle := th.Base().Foreground(th.TextDim).Background(th.Background)
		wrapped := WrapText(it.ToolOutput, bodyStyle, width-4)
		const maxOutputLines = 100
		if len(wrapped) > maxOutputLines {
			wrapped = wrapped[:maxOutputLines]
		}
		for _, ln := range wrapped {
			row := Line{{R: ' ', S: th.Base()}, {R: ' ', S: th.Base()}}
			row = append(row, ln...)
			lines = append(lines, row)
		}
		if len(wrapped) == maxOutputLines {
			lines = append(lines, LineFromSegments([]Segment{
				{Text: "  … output truncated", Style: th.Base().Foreground(th.Muted).Background(th.Background)},
			}, width))
		}
		lines = append(lines, LineFromSegments([]Segment{
			{Text: "  [Enter] Collapse", Style: th.Base().Foreground(th.Muted).Background(th.Background)},
		}, width))
		return lines
	}

	if hasOutput {
		lines := []Line{c.toolStatusLine(it, width, '✔', th.Success, " ✔ "+formatDuration(it.Duration)+"  [Enter] Expand")}
		return lines
	}

	return []Line{c.toolStatusLine(it, width, '✔', th.Success, " ✔ "+formatDuration(it.Duration))}
}

func (c *Chat) toolStatusLine(it *ChatItem, width int, mark rune, color tcell.Color, statusText string) Line {
	th := styles.Current()
	left := Line{
		{R: mark, S: th.Base().Foreground(color).Background(th.Background)},
		{R: ' ', S: th.Base()},
	}
	displayName := it.ToolName
	toolType := ""
	if idx := strings.Index(displayName, ":"); idx > 0 {
		toolType = displayName[:idx]
		displayName = displayName[idx+1:]
	}
	if toolType != "" {
		capType := strings.ToUpper(toolType[:1]) + toolType[1:]
		for _, r := range capType {
			left = append(left, Cell{R: r, S: th.Base().Foreground(color).Bold(true).Background(th.Background)})
		}
		left = append(left, Cell{R: ':', S: th.Base().Foreground(color).Background(th.Background)})
		left = append(left, Cell{R: ' ', S: th.Base()})
	}
	statusCells := LineFromSegments([]Segment{
		{Text: statusText, Style: th.Base().Foreground(color).Background(th.Background)},
	}, width)
	statusW := statusCells.Width()
	for _, r := range displayName {
		if left.Width()+1+statusW >= width {
			left = append(left, Cell{R: '…', S: th.Base().Foreground(th.Muted).Background(th.Background)})
			break
		}
		left = append(left, Cell{R: r, S: th.Base().Foreground(th.Foreground).Bold(true).Background(th.Background)})
	}
	left = append(left, Cell{R: ' ', S: th.Base()})
	maxLeft := width - statusW
	if maxLeft < 0 {
		maxLeft = 0
	}
	if left.Width() > maxLeft {
		left = TruncateLine(left, maxLeft)
	}
	for left.Width() < maxLeft {
		left = append(left, Cell{R: ' ', S: th.Base()})
	}
	left = append(left, statusCells...)
	return left
}

func (c *Chat) todoCardLines(it *ChatItem, width int) []Line {
	th := styles.Current()
	if width < 12 {
		return nil
	}

	border := th.Base().Foreground(th.Border).Background(th.Background)
	accent := th.Base().Foreground(th.Accent).Background(th.Background)
	dim := th.Base().Foreground(th.TextDim).Background(th.Background)
	success := th.Base().Foreground(th.Success).Bold(true).Background(th.Background)
	errStyle := th.Base().Foreground(th.Error).Background(th.Background)
	running := th.Base().Foreground(th.Running).Bold(true).Background(th.Background)

	var lines []Line

	top := Line{{R: '╭', S: border}}
	top = append(top, Cell{R: '─', S: border})
	for _, r := range " TODO " {
		top = append(top, Cell{R: r, S: accent})
	}
	for top.Width() < width-1 {
		top = append(top, Cell{R: '─', S: border})
	}
	top = append(top, Cell{R: '╮', S: border})
	lines = append(lines, top)

	type todoItem struct {
		status string
		title  string
		desc   string
		deps   string
	}
	var todos []todoItem

	textLines := strings.Split(it.ToolOutput, "\n")
	for _, l := range textLines {
		trimmed := strings.TrimSpace(l)
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "Todos:") {
			continue
		}
		if strings.HasPrefix(trimmed, "No todos.") {
			row := Line{{R: '│', S: border}, {R: ' ', S: th.Base()}}
			for _, r := range "No todos." {
				row = append(row, Cell{R: r, S: dim})
			}
			for row.Width() < width-1 {
				row = append(row, Cell{R: ' ', S: th.Base()})
			}
			row = append(row, Cell{R: '│', S: border})
			lines = append(lines, row)
			sep := cardSep(border, width)
			lines = append(lines, sep)
			lines = append(lines, cardFooter(border, dim, th.Base(), width, fmt.Sprintf("0 / 0 completed")))
			return lines
		}

		status := ""
		rest := trimmed
		switch {
		case strings.HasPrefix(trimmed, "[x]"):
			status = "completed"
			rest = strings.TrimSpace(trimmed[3:])
		case strings.HasPrefix(trimmed, "[~]"):
			status = "in_progress"
			rest = strings.TrimSpace(trimmed[3:])
		case strings.HasPrefix(trimmed, "[-]"):
			status = "cancelled"
			rest = strings.TrimSpace(trimmed[3:])
		case strings.HasPrefix(trimmed, "[ ]"):
			status = "pending"
			rest = strings.TrimSpace(trimmed[3:])
		default:
			if strings.HasPrefix(trimmed, "   ") && len(todos) > 0 {
				cur := &todos[len(todos)-1]
				if strings.Contains(trimmed, "depends on:") {
					cur.deps = strings.TrimSpace(strings.TrimPrefix(trimmed, "  depends on:"))
				} else {
					cur.desc = strings.TrimSpace(trimmed)
				}
			}
			continue
		}

		var ti todoItem
		ti.status = status

		if idx := strings.Index(rest, "]"); idx >= 0 {
			ti.title = strings.TrimSpace(rest[idx+1:])
		} else {
			ti.title = rest
		}
		if pIdx := strings.LastIndex(ti.title, "(priority:"); pIdx >= 0 {
			ti.title = strings.TrimSpace(ti.title[:pIdx])
		}
		todos = append(todos, ti)
	}

	for _, t := range todos {
		row := Line{{R: '│', S: border}, {R: ' ', S: th.Base()}}

		var icon rune
		var iconStyle tcell.Style
		var titleStyle tcell.Style

		switch t.status {
		case "completed":
			icon = '✓'
			iconStyle = success
			titleStyle = success
		case "in_progress":
			icon = '◉'
			iconStyle = running
			titleStyle = th.Base().Foreground(th.Foreground).Bold(true).Background(th.Background)
		case "cancelled":
			icon = '✗'
			iconStyle = errStyle
			titleStyle = errStyle
		default:
			icon = '○'
			iconStyle = th.Base().Foreground(th.Muted).Background(th.Background)
			titleStyle = dim
		}

		row = append(row, Cell{R: icon, S: iconStyle}, Cell{R: ' ', S: th.Base()})
		for _, r := range t.title {
			row = append(row, Cell{R: r, S: titleStyle})
		}
		if t.status == "cancelled" {
			for i := len(row) - 2; i >= 0; i-- {
				if row[i].R != ' ' && row[i].R != '│' && row[i].S != th.Base() && row[i].S != border {
					row[i].S = row[i].S.StrikeThrough(true)
				}
			}
		}

		for row.Width() < width-1 {
			row = append(row, Cell{R: ' ', S: th.Base()})
		}
		row = append(row, Cell{R: '│', S: border})
		lines = append(lines, row)

		if t.desc != "" {
			drow := Line{{R: '│', S: border}, Cell{R: ' ', S: th.Base()}, Cell{R: ' ', S: th.Base()}, Cell{R: ' ', S: th.Base()}}
			for _, r := range "    " + t.desc {
				if drow.Width() >= width-2 {
					break
				}
				drow = append(drow, Cell{R: r, S: dim})
			}
			for drow.Width() < width-1 {
				drow = append(drow, Cell{R: ' ', S: th.Base()})
			}
			drow = append(drow, Cell{R: '│', S: border})
			lines = append(lines, drow)
		}
		if t.deps != "" {
			drow := Line{{R: '│', S: border}, Cell{R: ' ', S: th.Base()}, Cell{R: ' ', S: th.Base()}, Cell{R: ' ', S: th.Base()}}
			for _, r := range "    depends on: " + t.deps {
				if drow.Width() >= width-2 {
					break
				}
				drow = append(drow, Cell{R: r, S: dim})
			}
			for drow.Width() < width-1 {
				drow = append(drow, Cell{R: ' ', S: th.Base()})
			}
			drow = append(drow, Cell{R: '│', S: border})
			lines = append(lines, drow)
		}
	}

	total := len(todos)
	completed := 0
	for _, t := range todos {
		if t.status == "completed" {
			completed++
		}
	}

	sep := cardSep(border, width)
	lines = append(lines, sep)
	lines = append(lines, cardFooter(border, dim, th.Base(), width, fmt.Sprintf("%d / %d completed", completed, total)))
	lines = append(lines, bottomEdge(border, width))

	return lines
}

func cardSep(border tcell.Style, width int) Line {
	sep := Line{{R: '├', S: border}}
	for i := 0; i < width-2; i++ {
		sep = append(sep, Cell{R: '─', S: border})
	}
	sep = append(sep, Cell{R: '┤', S: border})
	return sep
}

func cardFooter(border tcell.Style, dim tcell.Style, bg tcell.Style, width int, text string) Line {
	footer := Line{{R: '│', S: border}, {R: ' ', S: bg}}
	for _, r := range text {
		footer = append(footer, Cell{R: r, S: dim})
	}
	for footer.Width() < width-1 {
		footer = append(footer, Cell{R: ' ', S: bg})
	}
	footer = append(footer, Cell{R: '│', S: border})
	return footer
}

func bottomEdge(border tcell.Style, width int) Line {
	bottom := Line{{R: '╰', S: border}}
	for i := 0; i < width-2; i++ {
		bottom = append(bottom, Cell{R: '─', S: border})
	}
	bottom = append(bottom, Cell{R: '╯', S: border})
	return bottom
}

func (c *Chat) bashCardLines(it *ChatItem, width int) []Line {
	th := styles.Current()
	if width < 12 {
		return nil
	}

	cmd := it.ToolName
	if idx := strings.Index(cmd, ":"); idx > 0 {
		cmd = cmd[idx+1:]
	}

	border := th.Base().Foreground(th.Border).Background(th.Background)
	accent := th.Base().Foreground(th.Accent).Background(th.Background)
	dim := th.Base().Foreground(th.TextDim).Background(th.Background)
	muted := th.Base().Foreground(th.Muted).Background(th.Background)
	success := th.Base().Foreground(th.Success).Background(th.Background)

	var lines []Line

	top := Line{{R: '╭', S: border}}
	top = append(top, Cell{R: '─', S: border})
	for _, r := range " BASH " {
		top = append(top, Cell{R: r, S: accent})
	}

	cmdStyle := th.Base().Foreground(th.Foreground).Background(th.Background)
	statusText := " ✓ " + formatDuration(it.Duration)
	statusStyle := success.Bold(true)

	cmdLabel := "$ " + cmd
	rightSide := statusText
	needed := top.Width() + DisplayWidth(cmdLabel) + 2 + DisplayWidth(rightSide)
	for needed > width-2 {
		if len(cmdLabel) > 4 {
			cmdLabel = cmdLabel[:len(cmdLabel)-1]
		} else {
			break
		}
		needed = top.Width() + DisplayWidth(cmdLabel) + 2 + DisplayWidth(rightSide)
	}

	for _, r := range cmdLabel {
		top = append(top, Cell{R: r, S: cmdStyle})
	}
	topPad := width - 2 - top.Width() - DisplayWidth(rightSide)
	if topPad < 1 {
		topPad = 1
	}
	for i := 0; i < topPad; i++ {
		top = append(top, Cell{R: ' ', S: th.Base()})
	}
	for _, r := range rightSide {
		top = append(top, Cell{R: r, S: statusStyle})
	}
	top = append(top, Cell{R: ' ', S: th.Base()})
	for top.Width() < width-1 {
		top = append(top, Cell{R: '─', S: border})
	}
	top = append(top, Cell{R: '╮', S: border})
	lines = append(lines, top)

	sep := Line{{R: '├', S: border}}
	for i := 0; i < width-2; i++ {
		sep = append(sep, Cell{R: '─', S: border})
	}
	sep = append(sep, Cell{R: '┤', S: border})
	lines = append(lines, sep)

	output := it.ToolOutput
	if output == "" {
		output = "(no output)"
	}

	rawLines := strings.Split(output, "\n")
	const maxBashLines = 30
	truncated := len(rawLines) > maxBashLines && !it.Expanded
	shown := rawLines
	if truncated {
		shown = rawLines[:maxBashLines]
	}

	bodyStyle := th.Base().Foreground(th.Foreground).Background(th.Background)
	for _, ln := range shown {
		row := Line{{R: '│', S: border}, {R: ' ', S: th.Base()}}
		hl := highlightByLangName(ln, "bash", th, bodyStyle)
		for _, c := range hl {
			if row.Width() >= width-2 {
				break
			}
			row = append(row, c)
		}
		for row.Width() < width-1 {
			row = append(row, Cell{R: ' ', S: th.Base()})
		}
		row = append(row, Cell{R: '│', S: border})
		lines = append(lines, row)
	}

	if truncated {
		remaining := len(rawLines) - maxBashLines
		moreLine := Line{{R: '│', S: border}, {R: ' ', S: th.Base()}}
		msg := fmt.Sprintf("··· %d more lines — click to expand", remaining)
		for _, r := range msg {
			moreLine = append(moreLine, Cell{R: r, S: dim})
		}
		for moreLine.Width() < width-1 {
			moreLine = append(moreLine, Cell{R: ' ', S: th.Base()})
		}
		moreLine = append(moreLine, Cell{R: '│', S: border})
		lines = append(lines, moreLine)
	}

	if it.Expanded {
		moreLine := Line{{R: '│', S: border}, {R: ' ', S: th.Base()}}
		msg := fmt.Sprintf("··· showing all %d lines — click to collapse", len(rawLines))
		for _, r := range msg {
			moreLine = append(moreLine, Cell{R: r, S: dim})
		}
		for moreLine.Width() < width-1 {
			moreLine = append(moreLine, Cell{R: ' ', S: th.Base()})
		}
		moreLine = append(moreLine, Cell{R: '│', S: border})
		lines = append(lines, moreLine)
	}

	sep2 := Line{{R: '├', S: border}}
	for i := 0; i < width-2; i++ {
		sep2 = append(sep2, Cell{R: '─', S: border})
	}
	sep2 = append(sep2, Cell{R: '┤', S: border})
	lines = append(lines, sep2)

	footer := Line{{R: '│', S: border}, {R: ' ', S: th.Base()}}
	exitStr := "exit code: 0"
	if it.ToolErr != "" {
		exitStr = "exit code: 1"
	}
	for _, r := range exitStr {
		footer = append(footer, Cell{R: r, S: muted})
	}

	expandHint := "[Enter] Expand"
	if it.Expanded {
		expandHint = "[Enter] Collapse"
	}
	for footer.Width()+DisplayWidth(expandHint)+4 < width {
		footer = append(footer, Cell{R: ' ', S: th.Base()})
	}
	for footer.Width() < width-1-DisplayWidth(expandHint) {
		footer = append(footer, Cell{R: ' ', S: th.Base()})
	}
	for _, r := range expandHint {
		footer = append(footer, Cell{R: r, S: dim})
	}
	for footer.Width() < width-1 {
		footer = append(footer, Cell{R: ' ', S: th.Base()})
	}
	footer = append(footer, Cell{R: '│', S: border})
	lines = append(lines, footer)

	bottom := Line{{R: '╰', S: border}}
	for i := 0; i < width-2; i++ {
		bottom = append(bottom, Cell{R: '─', S: border})
	}
	bottom = append(bottom, Cell{R: '╯', S: border})
	lines = append(lines, bottom)

	return lines
}

func (c *Chat) editCardLines(it *ChatItem, width int) []Line {
	th := styles.Current()
	if width < 12 {
		return nil
	}

	filePath := it.ToolName
	if idx := strings.Index(filePath, ":"); idx > 0 {
		filePath = filePath[idx+1:]
	}

	border := th.Base().Foreground(th.Border).Background(th.CodeBg)
	accent := th.Base().Foreground(th.Accent).Background(th.CodeBg)
	dim := th.Base().Foreground(th.TextDim).Background(th.CodeBg)
	muted := th.Base().Foreground(th.Muted).Background(th.CodeBg)
	addedStyle := th.Base().Foreground(th.Success).Bold(true).Background(th.CodeBg)
	removedStyle := th.Base().Foreground(th.Error).Bold(true).Background(th.CodeBg)
	codeBg := th.Base().Background(th.CodeBg)

	var lines []Line

	top := Line{{R: '┌', S: border}}
	top = append(top, Cell{R: '─', S: border})
	for _, r := range " DIFF " {
		top = append(top, Cell{R: r, S: accent})
	}
	for top.Width() < width-1 {
		top = append(top, Cell{R: '─', S: border})
	}
	top = append(top, Cell{R: '┐', S: border})
	lines = append(lines, top)

	pathLine := Line{{R: '│', S: border}, {R: ' ', S: muted}}
	for _, r := range truncateToDisplayWidth(filePath, width-4) {
		pathLine = append(pathLine, Cell{R: r, S: accent.Bold(true)})
	}
	for pathLine.Width() < width-1 {
		pathLine = append(pathLine, Cell{R: ' ', S: th.Base()})
	}
	pathLine = append(pathLine, Cell{R: '│', S: border})
	lines = append(lines, pathLine)

	allLines := 0
	for _, hunk := range it.Diff {
		allLines += len(hunk.Lines)
	}
	const maxCardRows = 30
	truncated := allLines > maxCardRows && !it.Expanded
	emitted := 0

	for _, hunk := range it.Diff {
		if truncated && emitted >= maxCardRows {
			break
		}

		for _, dl := range hunk.Lines {
			if truncated && emitted >= maxCardRows {
				break
			}
			cl := Line{{R: '│', S: border}, {R: ' ', S: codeBg}}

			diffStyle := dim
			pref := "  "
			switch dl.Kind {
			case '-':
				diffStyle = removedStyle
				pref = "- "
			case '+':
				diffStyle = addedStyle
				pref = "+ "
			default:
				diffStyle = dim
				pref = "  "
			}
			for _, r := range pref {
				cl = append(cl, Cell{R: r, S: diffStyle})
			}

			contentLine := highlightLine(dl.Text, filePath, th, dim)
			for _, cell := range contentLine {
				if cl.Width() >= width-2 {
					break
				}
				cl = append(cl, cell)
			}
			for cl.Width() < width-1 {
				cl = append(cl, Cell{R: ' ', S: th.Base()})
			}
			cl = append(cl, Cell{R: '│', S: border})
			lines = append(lines, cl)
			emitted++
		}
	}

	if truncated {
		remaining := allLines - maxCardRows
		moreLine := Line{{R: '│', S: border}, {R: ' ', S: th.Base()}}
		msg := fmt.Sprintf("··· %d more lines — click to expand", remaining)
		for _, r := range msg {
			moreLine = append(moreLine, Cell{R: r, S: dim})
		}
		for moreLine.Width() < width-1 {
			moreLine = append(moreLine, Cell{R: ' ', S: th.Base()})
		}
		moreLine = append(moreLine, Cell{R: '│', S: border})
		lines = append(lines, moreLine)
	}

	if it.Expanded {
		moreLine := Line{{R: '│', S: border}, {R: ' ', S: th.Base()}}
		msg := fmt.Sprintf("··· showing all %d lines — click to collapse", allLines)
		for _, r := range msg {
			moreLine = append(moreLine, Cell{R: r, S: dim})
		}
		for moreLine.Width() < width-1 {
			moreLine = append(moreLine, Cell{R: ' ', S: th.Base()})
		}
		moreLine = append(moreLine, Cell{R: '│', S: border})
		lines = append(lines, moreLine)
	}

	addedCount, removedCount := 0, 0
	for _, hunk := range it.Diff {
		for _, dl := range hunk.Lines {
			switch dl.Kind {
			case '-':
				removedCount++
			case '+':
				addedCount++
			}
		}
	}
	footer := Line{{R: '└', S: border}}
	for _, r := range fmt.Sprintf(" +%d  -%d ", addedCount, removedCount) {
		footer = append(footer, Cell{R: r, S: addedStyle})
	}
	for footer.Width() < width-1 {
		footer = append(footer, Cell{R: '─', S: border})
	}
	footer = append(footer, Cell{R: '┘', S: border})
	lines = append(lines, footer)

	return lines
}

func (c *Chat) writeCardLines(it *ChatItem, width int) []Line {
	th := styles.Current()
	if width < 12 {
		return nil
	}

	filePath := it.ToolName
	if idx := strings.Index(filePath, ":"); idx > 0 {
		filePath = filePath[idx+1:]
	}

	border := th.Base().Foreground(th.Border).Background(th.CodeBg)
	accent := th.Base().Foreground(th.Accent).Background(th.CodeBg)
	dim := th.Base().Foreground(th.TextDim).Background(th.CodeBg)
	muted := th.Base().Foreground(th.Muted).Background(th.CodeBg)
	codeBg := th.Base().Background(th.CodeBg)

	var lines []Line

	top := Line{{R: '┌', S: border}}
	top = append(top, Cell{R: '─', S: border})
	for _, r := range " WRITE " {
		top = append(top, Cell{R: r, S: accent})
	}
	for top.Width() < width-1 {
		top = append(top, Cell{R: '─', S: border})
	}
	top = append(top, Cell{R: '┐', S: border})
	lines = append(lines, top)

	pathLine := Line{{R: '│', S: border}, {R: ' ', S: muted}}
	for _, r := range truncateToDisplayWidth(filePath, width-4) {
		pathLine = append(pathLine, Cell{R: r, S: accent.Bold(true)})
	}
	for pathLine.Width() < width-1 {
		pathLine = append(pathLine, Cell{R: ' ', S: th.Base()})
	}
	pathLine = append(pathLine, Cell{R: '│', S: border})
	lines = append(lines, pathLine)

	sep := Line{{R: '├', S: border}}
	for i := 0; i < width-2; i++ {
		sep = append(sep, Cell{R: '─', S: border})
	}
	sep = append(sep, Cell{R: '┤', S: border})
	lines = append(lines, sep)

	content := it.WriteContent
	if content == "" {
		content = "(empty)"
	}

	rawLines := strings.Split(content, "\n")
	const maxWriteLines = 30
	truncated := len(rawLines) > maxWriteLines && !it.Expanded
	shown := rawLines
	if truncated {
		shown = rawLines[:maxWriteLines]
	}

	for _, ln := range shown {
		row := Line{{R: '│', S: border}, {R: ' ', S: codeBg}}
		contentLine := highlightLine(ln, filePath, th, dim)
		for _, cell := range contentLine {
			if row.Width() >= width-2 {
				break
			}
			row = append(row, cell)
		}
		for row.Width() < width-1 {
			row = append(row, Cell{R: ' ', S: th.Base()})
		}
		row = append(row, Cell{R: '│', S: border})
		lines = append(lines, row)
	}

	if truncated {
		remaining := len(rawLines) - maxWriteLines
		moreLine := Line{{R: '│', S: border}, {R: ' ', S: th.Base()}}
		msg := fmt.Sprintf("··· %d more lines — click to expand", remaining)
		for _, r := range msg {
			moreLine = append(moreLine, Cell{R: r, S: dim})
		}
		for moreLine.Width() < width-1 {
			moreLine = append(moreLine, Cell{R: ' ', S: th.Base()})
		}
		moreLine = append(moreLine, Cell{R: '│', S: border})
		lines = append(lines, moreLine)
	}

	if it.Expanded {
		total := len(rawLines)
		moreLine := Line{{R: '│', S: border}, {R: ' ', S: th.Base()}}
		msg := fmt.Sprintf("··· showing all %d lines — click to collapse", total)
		for _, r := range msg {
			moreLine = append(moreLine, Cell{R: r, S: dim})
		}
		for moreLine.Width() < width-1 {
			moreLine = append(moreLine, Cell{R: ' ', S: th.Base()})
		}
		moreLine = append(moreLine, Cell{R: '│', S: border})
		lines = append(lines, moreLine)
	}

	footer := Line{{R: '└', S: border}}
	for i := 0; i < width-2; i++ {
		footer = append(footer, Cell{R: '─', S: border})
	}
	footer = append(footer, Cell{R: '┘', S: border})
	lines = append(lines, footer)

	return lines
}

type askQuestion struct {
	ID       string   `json:"id"`
	Question string   `json:"question"`
	Type     string   `json:"type"`
	Options  []string `json:"options,omitempty"`
	Required bool     `json:"required"`
}

type askArgs struct {
	Questions []askQuestion `json:"questions"`
}

type askOutput struct {
	Answers map[string]string `json:"answers"`
}

func (c *Chat) askCardLines(it *ChatItem, width int) []Line {
	th := styles.Current()
	if width < 12 {
		return nil
	}

	border := th.Base().Foreground(th.Border).Background(th.Background)
	accent := th.Base().Foreground(th.Accent).Background(th.Background)
	dim := th.Base().Foreground(th.TextDim).Background(th.Background)

	var qs []askQuestion
	var args askArgs
	if err := json.Unmarshal([]byte(it.ToolArgs), &args); err == nil {
		qs = args.Questions
	}

	var answers map[string]string
	if it.ToolOutput != "" {
		var out askOutput
		if err := json.Unmarshal([]byte(it.ToolOutput), &out); err == nil {
			answers = out.Answers
		}
	}
	if answers == nil {
		answers = make(map[string]string)
	}

	var lines []Line

	top := Line{{R: '╭', S: border}}
	top = append(top, Cell{R: '─', S: border})
	for _, r := range " ASK " {
		top = append(top, Cell{R: r, S: accent})
	}
	for top.Width() < width-1 {
		top = append(top, Cell{R: '─', S: border})
	}
	top = append(top, Cell{R: '╮', S: border})
	lines = append(lines, top)

	for _, q := range qs {
		qtext := q.Question
		if len(qtext) > width-6 {
			qtext = qtext[:width-7] + "\u2026"
		}
		qLine := Line{{R: '│', S: border}, {R: ' ', S: th.Base()}}
		for _, r := range "Q: " + qtext {
			qLine = append(qLine, Cell{R: r, S: accent})
		}
		for qLine.Width() < width-1 {
			qLine = append(qLine, Cell{R: ' ', S: th.Base()})
		}
		qLine = append(qLine, Cell{R: '│', S: border})
		lines = append(lines, qLine)

		a := answers[q.ID]
		if a == "" {
			a = "(unanswered)"
		}
		atext := a
		if len(atext) > width-6 {
			atext = atext[:width-7] + "\u2026"
		}
		aLine := Line{{R: '│', S: border}, {R: ' ', S: th.Base()}, {R: ' ', S: th.Base()}}
		for _, r := range "A: " + atext {
			aLine = append(aLine, Cell{R: r, S: dim})
		}
		for aLine.Width() < width-1 {
			aLine = append(aLine, Cell{R: ' ', S: th.Base()})
		}
		aLine = append(aLine, Cell{R: '│', S: border})
		lines = append(lines, aLine)
	}

	sep := Line{{R: '├', S: border}}
	for i := 0; i < width-2; i++ {
		sep = append(sep, Cell{R: '─', S: border})
	}
	sep = append(sep, Cell{R: '┤', S: border})
	lines = append(lines, sep)

	nAnswered := 0
	for _, q := range qs {
		if answers[q.ID] != "" {
			nAnswered++
		}
	}
	footer := Line{{R: '│', S: border}, {R: ' ', S: th.Base()}}
	summary := fmt.Sprintf("%d / %d answered", nAnswered, len(qs))
	for _, r := range summary {
		footer = append(footer, Cell{R: r, S: dim})
	}
	for footer.Width() < width-1 {
		footer = append(footer, Cell{R: ' ', S: th.Base()})
	}
	footer = append(footer, Cell{R: '│', S: border})
	lines = append(lines, footer)

	bot := Line{{R: '╰', S: border}}
	for i := 0; i < width-2; i++ {
		bot = append(bot, Cell{R: '─', S: border})
	}
	bot = append(bot, Cell{R: '╯', S: border})
	lines = append(lines, bot)

	return lines
}

func (c *Chat) subagentLines(it *ChatItem, width int) []Line {
	th := styles.Current()
	if width < 12 {
		return nil
	}

	border := th.Base().Foreground(th.Border).Background(th.Background)
	accent := th.Base().Foreground(th.Accent).Background(th.Background)
	dim := th.Base().Foreground(th.TextDim).Background(th.Background)
	muted := th.Base().Foreground(th.Muted).Background(th.Background)
	subagentColor := th.Base().Foreground(th.Subagent).Background(th.Background)

	status := it.SubagentStatus
	if status == "" {
		status = "running"
	}
	isRunning := status == "running"

	var mark rune
	if isRunning {
		mark = '◌'
	} else if status == "error" {
		mark = '✖'
	} else {
		mark = '✔'
	}

	var lines []Line

	top := Line{{R: '╭', S: border}}
	top = append(top, Cell{R: '─', S: border})
	top = append(top, Cell{R: ' ', S: th.Base()})
	top = append(top, Cell{R: mark, S: subagentColor})
	top = append(top, Cell{R: ' ', S: th.Base()})
	historyHint := "History"
	maxName := width - 13
	if maxName < 1 {
		maxName = 1
	}
	name := it.SubagentName
	if DisplayWidth(name) > maxName {
		name = truncateToDisplayWidth(name, maxName-1) + "…"
	}
	for _, r := range name {
		top = append(top, Cell{R: r, S: subagentColor})
	}
	historyStart := width - 1 - DisplayWidth(historyHint)
	for top.Width() < historyStart {
		top = append(top, Cell{R: '─', S: border})
	}
	for _, r := range historyHint {
		top = append(top, Cell{R: r, S: muted})
	}
	top = append(top, Cell{R: '╮', S: border})
	lines = append(lines, top)

	task := it.SubagentTask
	if task == "" {
		task = "…"
	}
	taskLine := Line{{R: '│', S: border}, {R: ' ', S: th.Base()}}
	if DisplayWidth(task) > width-6 {
		task = task[:width-6] + "…"
	}
	for _, r := range task {
		taskLine = append(taskLine, Cell{R: r, S: th.Base().Foreground(th.Foreground).Background(th.Background)})
	}
	for taskLine.Width() < width-1 {
		taskLine = append(taskLine, Cell{R: ' ', S: th.Base()})
	}
	taskLine = append(taskLine, Cell{R: '│', S: border})
	lines = append(lines, taskLine)

	sep := Line{{R: '├', S: border}}
	for i := 0; i < width-2; i++ {
		sep = append(sep, Cell{R: '─', S: border})
	}
	sep = append(sep, Cell{R: '┤', S: border})
	lines = append(lines, sep)

	if it.SubagentActivity != "" && isRunning {
		activityLine := Line{{R: '│', S: border}, {R: ' ', S: th.Base()}}
		activityMark := '◌'
		activityLine = append(activityLine, Cell{R: activityMark, S: th.Base().Foreground(lerpColor(th.Subagent, th.Accent, glowT(c.now))).Background(th.Background)})
		activityLine = append(activityLine, Cell{R: ' ', S: th.Base()})
		activity := it.SubagentActivity
		if DisplayWidth(activity) > width-7 {
			activity = activity[:width-7] + "…"
		}
		for _, r := range activity {
			activityLine = append(activityLine, Cell{R: r, S: dim})
		}
		for activityLine.Width() < width-1 {
			activityLine = append(activityLine, Cell{R: ' ', S: th.Base()})
		}
		activityLine = append(activityLine, Cell{R: '│', S: border})
		lines = append(lines, activityLine)
	} else if it.SubagentOutput != "" && !isRunning {
		outputLines := strings.Split(it.SubagentOutput, "\n")
		const maxOutputLines = 15
		truncated := len(outputLines) > maxOutputLines && !it.Expanded
		shown := outputLines
		if truncated {
			shown = outputLines[:maxOutputLines]
		}
		for _, ln := range shown {
			row := Line{{R: '│', S: border}, {R: ' ', S: th.Base()}}
			if DisplayWidth(ln) > width-4 {
				ln = ln[:width-4] + "…"
			}
			for _, r := range ln {
				row = append(row, Cell{R: r, S: dim})
			}
			for row.Width() < width-1 {
				row = append(row, Cell{R: ' ', S: th.Base()})
			}
			row = append(row, Cell{R: '│', S: border})
			lines = append(lines, row)
		}
		if truncated {
			remaining := len(outputLines) - maxOutputLines
			moreLine := Line{{R: '│', S: border}, {R: ' ', S: th.Base()}}
			msg := fmt.Sprintf("··· %d more lines — click to expand", remaining)
			for _, r := range msg {
				moreLine = append(moreLine, Cell{R: r, S: dim})
			}
			for moreLine.Width() < width-1 {
				moreLine = append(moreLine, Cell{R: ' ', S: th.Base()})
			}
			moreLine = append(moreLine, Cell{R: '│', S: border})
			lines = append(lines, moreLine)
		}
		if it.Expanded {
			moreLine := Line{{R: '│', S: border}, {R: ' ', S: th.Base()}}
			msg := fmt.Sprintf("··· showing all %d lines — click to collapse", len(outputLines))
			for _, r := range msg {
				moreLine = append(moreLine, Cell{R: r, S: dim})
			}
			for moreLine.Width() < width-1 {
				moreLine = append(moreLine, Cell{R: ' ', S: th.Base()})
			}
			moreLine = append(moreLine, Cell{R: '│', S: border})
			lines = append(lines, moreLine)
		}
	}

	sep2 := Line{{R: '├', S: border}}
	for i := 0; i < width-2; i++ {
		sep2 = append(sep2, Cell{R: '─', S: border})
	}
	sep2 = append(sep2, Cell{R: '┤', S: border})
	lines = append(lines, sep2)

	footer := Line{{R: '╰', S: border}}
	usageText := ""
	if it.SubagentUsage.TotalTokens > 0 {
		usageText = llm.FormatContextUsage(it.SubagentUsage, 0)
	}
	if usageText != "" {
		footer = append(footer, Cell{R: ' ', S: th.Base()})
		for _, r := range usageText {
			footer = append(footer, Cell{R: r, S: accent})
		}
	}
	for footer.Width() < width-1 {
		footer = append(footer, Cell{R: '─', S: border})
	}
	footer = append(footer, Cell{R: '╯', S: border})
	lines = append(lines, footer)

	return lines
}

func (c *Chat) OnTick(blinkOn bool) {
	c.now += 450 * time.Millisecond
}
