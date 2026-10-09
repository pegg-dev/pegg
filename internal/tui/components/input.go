package components

import (
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gdamore/tcell/v2"

	"github.com/peggco/pegg/internal/tui/layout"
	"github.com/peggco/pegg/internal/tui/styles"
)

type inputSnapshot struct {
	lines  []string
	row    int
	col    int
	anchor *textPos
}

type textPos struct {
	row, col int
}

type Input struct {
	lines   []string
	row     int
	col     int
	scroll  int
	hscroll int
	innerW  int
	anchor  *textPos

	minRows int
	maxRows int

	killRegister string
	undo         []inputSnapshot
	redo         []inputSnapshot

	chips     []string
	mentions  []string
	longTexts map[rune]string

	focused      bool
	blinkOn      bool
	lastActivity time.Time
	Placeholder  string
	OnSubmit     func(string)
}

const chipBase = rune(0xE000)
const mentionBase = rune(0xF000)

func NewInput() *Input {
	return &Input{
		lines:     []string{""},
		minRows:   3,
		maxRows:   6,
		longTexts: make(map[rune]string),
	}
}

func (in *Input) SetRows(minRows, maxRows int) {
	if minRows < 1 {
		minRows = 1
	}
	if maxRows < minRows {
		maxRows = minRows
	}
	in.minRows, in.maxRows = minRows, maxRows
}

func (in *Input) Value() string {
	lines := make([]string, len(in.lines))
	for i, line := range in.lines {
		lines[i] = serializeChips(line, in.chips, in.mentions, in.longTexts)
	}
	return strings.Join(lines, "\n")
}

func (in *Input) chipRune(name string) rune {
	in.chips = append(in.chips, name)
	return chipBase + rune(len(in.chips)-1)
}

func chipNameOf(r rune, chips []string) (string, bool) {
	if r < chipBase {
		return "", false
	}
	i := int(r - chipBase)
	if i >= 0 && i < len(chips) {
		return chips[i], true
	}
	return "", false
}

func chipWidth(r rune, chips []string) int {
	if name, ok := chipNameOf(r, chips); ok {
		return len(name) + 1
	}
	return 1
}

func (in *Input) mentionChipRune(name string) rune {
	in.mentions = append(in.mentions, name)
	return mentionBase + rune(len(in.mentions)-1)
}

func mentionNameOf(r rune, mentions []string) (string, bool) {
	if r < mentionBase {
		return "", false
	}
	i := int(r - mentionBase)
	if i >= 0 && i < len(mentions) {
		return mentions[i], true
	}
	return "", false
}

func mentionWidth(r rune, mentions []string) int {
	if name, ok := mentionNameOf(r, mentions); ok {
		return len(name) + 1
	}
	return 1
}

func serializeChips(line string, chips []string, mentions []string, longTexts map[rune]string) string {
	hasSpecial := false
	for _, r := range line {
		if r >= chipBase || r >= mentionBase {
			hasSpecial = true
			break
		}
	}
	if !hasSpecial {
		return line
	}
	var b strings.Builder
	for _, r := range line {
		if full, ok := longTexts[r]; ok {
			b.WriteString(full)
		} else if name, ok := chipNameOf(r, chips); ok {
			b.WriteByte('/')
			b.WriteString(name)
		} else if name, ok := mentionNameOf(r, mentions); ok {
			b.WriteByte('@')
			b.WriteString(name)
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func (in *Input) SlashQuery() (bool, string) {
	tok, ok := in.slashToken()
	return ok, tok.query
}

func (in *Input) SlashToken() (at int, query string, ok bool) {
	tok, found := in.slashToken()
	return tok.at, tok.query, found
}

type slashToken struct {
	at, end int
	query   string
}

func (in *Input) slashToken() (slashToken, bool) {
	if len(in.lines) == 0 {
		return slashToken{}, false
	}
	rs := []rune(in.lines[in.row])
	for i := len(rs) - 1; i >= 0; i-- {
		if rs[i] != '/' {
			continue
		}
		if i > 0 && !isSpace(rs[i-1]) {
			continue
		}
		if in.col < i {
			continue
		}
		end := i + 1
		for end < len(rs) && !isSpace(rs[end]) {
			end++
		}
		return slashToken{at: i, end: end, query: string(rs[i+1 : end])}, true
	}
	return slashToken{}, false
}

type atToken struct {
	at, end int
	query   string
}

func (in *Input) atToken() (atToken, bool) {
	if len(in.lines) == 0 {
		return atToken{}, false
	}
	rs := []rune(in.lines[in.row])
	for i := len(rs) - 1; i >= 0; i-- {
		if rs[i] != '@' {
			continue
		}
		if i > 0 && !isSpace(rs[i-1]) {
			continue
		}
		if in.col < i {
			continue
		}
		end := i + 1
		for end < len(rs) && !isSpace(rs[end]) {
			end++
		}
		return atToken{at: i, end: end, query: string(rs[i+1 : end])}, true
	}
	return atToken{}, false
}

func (in *Input) AtQuery() (bool, string) {
	tok, ok := in.atToken()
	return ok, tok.query
}

func (in *Input) AtToken() (at int, query string, ok bool) {
	tok, found := in.atToken()
	return tok.at, tok.query, found
}

func (in *Input) InsertChip(name string) {
	in.saveUndo()
	rs := []rune(in.lines[in.row])
	r := in.chipRune(name)
	rs = append(rs[:in.col], append([]rune{r}, rs[in.col:]...)...)
	in.lines[in.row] = string(rs)
	in.col++
	in.ensureCursorVisible()
}

const LongTextThreshold = 100
const longTextPreviewLen = 10

func (in *Input) InsertLongText(text string) {
	in.saveUndo()
	if in.anchor != nil {
		in.deleteSelection()
	}
	preview := text
	if len(preview) > longTextPreviewLen {
		preview = preview[:longTextPreviewLen]
	}
	r := in.chipRune(preview)
	in.longTexts[r] = text
	rs := []rune(in.lines[in.row])
	rs = append(rs[:in.col], append([]rune{r}, rs[in.col:]...)...)
	in.lines[in.row] = string(rs)
	in.col++
	in.ensureCursorVisible()
}

func (in *Input) ReplaceSkill(name string) {
	tok, ok := in.slashToken()
	if !ok {
		in.InsertChip(name)
		return
	}
	in.saveUndo()
	rs := []rune(in.lines[in.row])
	chip := in.chipRune(name)
	rs = append(rs[:tok.at], append([]rune{chip}, rs[tok.end:]...)...)
	in.col = tok.at + 1
	if in.col >= len(rs) {
		rs = append(rs[:in.col], append([]rune{' '}, rs[in.col:]...)...)
		in.col++
	}
	in.lines[in.row] = string(rs)
	in.ensureCursorVisible()
}

func (in *Input) InsertMention(name string) {
	in.saveUndo()
	rs := []rune(in.lines[in.row])
	r := in.mentionChipRune(name)
	rs = append(rs[:in.col], append([]rune{r}, rs[in.col:]...)...)
	in.lines[in.row] = string(rs)
	in.col++
	in.ensureCursorVisible()
}

func (in *Input) ReplaceMention(name string) {
	tok, ok := in.atToken()
	if !ok {
		in.InsertMention(name)
		return
	}
	in.saveUndo()
	rs := []rune(in.lines[in.row])
	chip := in.mentionChipRune(name)
	rs = append(rs[:tok.at], append([]rune{chip}, rs[tok.end:]...)...)
	in.col = tok.at + 1
	if in.col >= len(rs) {
		rs = append(rs[:in.col], append([]rune{' '}, rs[in.col:]...)...)
		in.col++
	}
	in.lines[in.row] = string(rs)
	in.ensureCursorVisible()
}

func (in *Input) Lines() []string { return in.lines }

func (in *Input) Row() int { return in.row }

func (in *Input) Col() int { return in.col }

func (in *Input) Scroll() int { return in.scroll }

func (in *Input) HScroll() int { return in.hscroll }

func (in *Input) VisibleRows() int {
	if in.innerW > 1 {
		wrapped := in.wrapLines(in.innerW)
		n := len(wrapped)
		switch {
		case n < in.minRows:
			return in.minRows
		case n > in.maxRows:
			return in.maxRows
		default:
			return n
		}
	}
	n := len(in.lines)
	switch {
	case n < in.minRows:
		return in.minRows
	case n > in.maxRows:
		return in.maxRows
	default:
		return n
	}
}

func (in *Input) SetInnerWidth(w int) {
	if w > 0 {
		in.innerW = w
	}
}

func (in *Input) Empty() bool {
	for _, l := range in.lines {
		if l != "" {
			return false
		}
	}
	return true
}

func (in *Input) HasSelection() bool { return in.anchor != nil }

func (in *Input) SelectionBounds() (start, end textPos, ok bool) {
	if in.anchor == nil {
		return textPos{}, textPos{}, false
	}
	start, end = normalizeSelection(*in.anchor, textPos{in.row, in.col})
	return start, end, true
}

func (in *Input) SelectedText() string {
	start, end, ok := in.SelectionBounds()
	if !ok {
		return ""
	}
	parts := make([]string, 0, end.row-start.row+1)
	for r := start.row; r <= end.row; r++ {
		rs := []rune(in.lines[r])
		from, to := 0, len(rs)
		if r == start.row {
			from = start.col
		}
		if r == end.row {
			to = end.col
		}
		if from > to {
			from = to
		}
		parts = append(parts, string(rs[from:to]))
	}
	return strings.Join(parts, "\n")
}

func normalizeSelection(a, b textPos) (textPos, textPos) {
	if a.row < b.row || (a.row == b.row && a.col <= b.col) {
		return a, b
	}
	return b, a
}

func (in *Input) Focus()        { in.focused = true }
func (in *Input) Blur()         { in.focused = false }
func (in *Input) Focused() bool { return in.focused }

func (in *Input) SetBlink(on bool) { in.blinkOn = on }

func (in *Input) OnTick(blinkOn bool) { in.blinkOn = blinkOn }

const CursorIdleDelay = 900 * time.Millisecond

func (in *Input) CursorVisible(t time.Time) bool {
	if t.Sub(in.lastActivity) < CursorIdleDelay {
		return true
	}
	return in.blinkOn
}

func lineWidth(s string) int { return utf8.RuneCountInString(s) }

func isSpace(r rune) bool { return r == ' ' || r == '\t' }

func copyLines(lines []string) []string {
	out := make([]string, len(lines))
	copy(out, lines)
	return out
}

func (in *Input) saveUndo() {
	in.undo = append(in.undo, inputSnapshot{copyLines(in.lines), in.row, in.col, clonePos(in.anchor)})
	if len(in.undo) > 200 {
		in.undo = in.undo[1:]
	}
	in.redo = nil
}

func clonePos(p *textPos) *textPos {
	if p == nil {
		return nil
	}
	c := *p
	return &c
}

func (in *Input) beginExtend() {
	if in.anchor == nil {
		in.anchor = &textPos{in.row, in.col}
	}
}

func (in *Input) clearSelection() { in.anchor = nil }

func (in *Input) deleteSelection() string {
	start, end, ok := in.SelectionBounds()
	if !ok {
		return ""
	}
	in.clearSelection()

	var deleted []string
	for r := start.row; r <= end.row; r++ {
		rs := []rune(in.lines[r])
		from, to := 0, len(rs)
		if r == start.row {
			from = start.col
		}
		if r == end.row {
			to = end.col
		}
		if from > to {
			from = to
		}
		deleted = append(deleted, string(rs[from:to]))
	}
	head := string([]rune(in.lines[start.row])[:start.col])
	tail := string([]rune(in.lines[end.row])[end.col:])
	in.lines[start.row] = head + tail
	if end.row > start.row {
		in.lines = append(in.lines[:start.row+1], in.lines[end.row+1:]...)
	}
	in.row, in.col = start.row, start.col
	return strings.Join(deleted, "\n")
}

func (in *Input) settleExtend() {
	if in.anchor != nil && *in.anchor == (textPos{in.row, in.col}) {
		in.clearSelection()
	}
}

func (in *Input) ensureCursorVisible() {
	visible := in.VisibleRows()
	if in.innerW > 1 {
		cursorVis := in.visualRowOf(in.row, in.col)
		if cursorVis < in.scroll {
			in.scroll = cursorVis
		} else if cursorVis >= in.scroll+visible {
			in.scroll = cursorVis - visible + 1
		}
	} else {
		if in.row < in.scroll {
			in.scroll = in.row
		} else if in.row >= in.scroll+visible {
			in.scroll = in.row - visible + 1
		}
	}
	if in.scroll < 0 {
		in.scroll = 0
	}
}

func (in *Input) runeWidth(r rune) int {
	if full, ok := in.longTexts[r]; ok {
		preview := full
		if len(preview) > longTextPreviewLen {
			preview = preview[:longTextPreviewLen]
		}
		return len(preview) + 2
	}
	if _, ok := chipNameOf(r, in.chips); ok {
		return chipWidth(r, in.chips)
	}
	if _, ok := mentionNameOf(r, in.mentions); ok {
		return mentionWidth(r, in.mentions)
	}
	return 1
}

func (in *Input) visualLineWidth(line string) int {
	width := 0
	for _, r := range line {
		width += in.runeWidth(r)
	}
	return width
}

func (in *Input) visualRowOf(row, col int) int {
	segs := in.wrapLines(in.innerW)
	for i, s := range segs {
		if s.lineIdx != row {
			continue
		}
		if col >= s.start && col <= s.end {
			if col == s.end && i+1 < len(segs) && segs[i+1].lineIdx == row {
				return i + 1
			}
			return i
		}
	}
	return 0
}

type wrapSegment struct {
	lineIdx    int
	start, end int
}

func (in *Input) wrapLines(width int) []wrapSegment {
	if width < 1 {
		width = 1
	}
	var segs []wrapSegment
	for li, line := range in.lines {
		rs := []rune(line)
		if len(rs) == 0 {
			segs = append(segs, wrapSegment{lineIdx: li, start: 0, end: 0})
			continue
		}
		segStart := 0
		for segStart < len(rs) {
			w := 0
			lastSpace := -1
			i := segStart
			for i < len(rs) {
				charW := in.runeWidth(rs[i])
				if w+charW > width {
					break
				}
				w += charW
				if rs[i] == ' ' {
					lastSpace = i
				}
				i++
			}
			if i >= len(rs) {
				segs = append(segs, wrapSegment{lineIdx: li, start: segStart, end: len(rs)})
				break
			}
			end := i
			if lastSpace >= segStart {
				end = lastSpace + 1
			}
			if end <= segStart {
				end = segStart + 1
				if end > len(rs) {
					end = len(rs)
				}
			}
			segs = append(segs, wrapSegment{lineIdx: li, start: segStart, end: end})
			segStart = end
		}
	}
	return segs
}

func (in *Input) visualOffset(s wrapSegment, col int) int {
	rs := []rune(in.lines[s.lineIdx])
	w := 0
	for i := s.start; i < col && i < len(rs); i++ {
		w += in.runeWidth(rs[i])
	}
	return w
}

func (in *Input) cursorVisualPos() ([]wrapSegment, int, int) {
	segs := in.wrapLines(in.innerW)
	for i, s := range segs {
		if s.lineIdx != in.row {
			continue
		}
		if in.col >= s.start && in.col < s.end {
			return segs, i, in.visualOffset(s, in.col)
		}
		if in.col == s.end {
			if i+1 < len(segs) && segs[i+1].lineIdx == in.row {
				return segs, i + 1, 0
			}
			return segs, i, in.visualOffset(s, in.col)
		}
	}
	return segs, 0, 0
}

func (in *Input) colAtVisualOffset(segs []wrapSegment, idx, offset int) (int, int) {
	if idx < 0 {
		idx = 0
	}
	if idx >= len(segs) {
		idx = len(segs) - 1
	}
	s := segs[idx]
	rs := []rune(in.lines[s.lineIdx])
	col := s.start
	w := 0
	for col < s.end && col < len(rs) {
		rw := in.runeWidth(rs[col])
		if w+rw > offset {
			break
		}
		w += rw
		col++
	}
	return s.lineIdx, col
}

func (in *Input) visualRows(width int) int {
	n := len(in.wrapLines(width))
	if n < in.minRows {
		return in.minRows
	}
	if n > in.maxRows {
		return in.maxRows
	}
	return n
}

func (in *Input) InsertRune(ch rune) {
	in.saveUndo()
	if in.anchor != nil {
		in.deleteSelection()
	}
	rs := []rune(in.lines[in.row])
	rs = append(rs[:in.col], append([]rune{ch}, rs[in.col:]...)...)
	in.lines[in.row] = string(rs)
	in.col++
	in.ensureCursorVisible()
}

func (in *Input) Backspace() {
	in.saveUndo()
	if in.anchor != nil {
		in.deleteSelection()
		in.ensureCursorVisible()
		return
	}
	if in.col > 0 {
		rs := []rune(in.lines[in.row])
		rs = append(rs[:in.col-1], rs[in.col:]...)
		in.lines[in.row] = string(rs)
		in.col--
		return
	}
	if in.row > 0 {
		prev := in.lines[in.row-1]
		in.lines[in.row-1] = prev + in.lines[in.row]
		in.lines = append(in.lines[:in.row], in.lines[in.row+1:]...)
		in.row--
		in.col = lineWidth(prev)
	}
	in.ensureCursorVisible()
}

func (in *Input) Delete() {
	in.saveUndo()
	if in.anchor != nil {
		in.deleteSelection()
		in.ensureCursorVisible()
		return
	}
	line := in.lines[in.row]
	if in.col < lineWidth(line) {
		rs := []rune(line)
		rs = append(rs[:in.col], rs[in.col+1:]...)
		in.lines[in.row] = string(rs)
		return
	}
	if in.row < len(in.lines)-1 {
		in.lines[in.row] = line + in.lines[in.row+1]
		in.lines = append(in.lines[:in.row+1], in.lines[in.row+2:]...)
	}
	in.ensureCursorVisible()
}

func (in *Input) Newline() {
	in.saveUndo()
	if in.anchor != nil {
		in.deleteSelection()
	}
	rs := []rune(in.lines[in.row])
	left := string(rs[:in.col])
	right := string(rs[in.col:])
	in.lines[in.row] = left
	in.lines = append(in.lines, "")
	copy(in.lines[in.row+2:], in.lines[in.row+1:])
	in.lines[in.row+1] = right
	in.row++
	in.col = 0
	in.ensureCursorVisible()
}

func (in *Input) MoveLeft()  { in.moveLeft(false) }
func (in *Input) ShiftLeft() { in.moveLeft(true) }

func (in *Input) moveLeft(extend bool) {
	if !extend {
		in.clearSelection()
	} else {
		in.beginExtend()
	}
	switch {
	case in.col > 0:
		in.col--
	case in.row > 0:
		in.row--
		in.col = lineWidth(in.lines[in.row])
	}
	in.ensureCursorVisible()
	in.settleExtend()
}

func (in *Input) MoveRight()  { in.moveRight(false) }
func (in *Input) ShiftRight() { in.moveRight(true) }

func (in *Input) moveRight(extend bool) {
	if !extend {
		in.clearSelection()
	} else {
		in.beginExtend()
	}
	switch {
	case in.col < lineWidth(in.lines[in.row]):
		in.col++
	case in.row < len(in.lines)-1:
		in.row++
		in.col = 0
	}
	in.ensureCursorVisible()
	in.settleExtend()
}

func (in *Input) MoveUp()  { in.moveUp(false) }
func (in *Input) ShiftUp() { in.moveUp(true) }

func (in *Input) moveUp(extend bool) {
	if !extend {
		in.clearSelection()
	} else {
		in.beginExtend()
	}
	if in.innerW > 1 {
		segs, idx, offset := in.cursorVisualPos()
		if idx > 0 {
			in.row, in.col = in.colAtVisualOffset(segs, idx-1, offset)
		} else {
			in.row, in.col = 0, 0
		}
	} else {
		if in.row == 0 {
			in.col = 0
		} else {
			in.row--
			if in.col > lineWidth(in.lines[in.row]) {
				in.col = lineWidth(in.lines[in.row])
			}
		}
	}
	in.ensureCursorVisible()
	in.settleExtend()
}

func (in *Input) MoveDown()  { in.moveDown(false) }
func (in *Input) ShiftDown() { in.moveDown(true) }

func (in *Input) moveDown(extend bool) {
	if !extend {
		in.clearSelection()
	} else {
		in.beginExtend()
	}
	if in.innerW > 1 {
		segs, idx, offset := in.cursorVisualPos()
		if idx+1 < len(segs) {
			in.row, in.col = in.colAtVisualOffset(segs, idx+1, offset)
		} else if len(segs) > 0 {
			last := segs[len(segs)-1]
			in.row = last.lineIdx
			in.col = lineWidth(in.lines[in.row])
		}
	} else {
		if in.row == len(in.lines)-1 {
			in.col = lineWidth(in.lines[in.row])
		} else {
			in.row++
			if in.col > lineWidth(in.lines[in.row]) {
				in.col = lineWidth(in.lines[in.row])
			}
		}
	}
	in.ensureCursorVisible()
	in.settleExtend()
}

func (in *Input) Home()      { in.home(false) }
func (in *Input) ShiftHome() { in.home(true) }

func (in *Input) home(extend bool) {
	if !extend {
		in.clearSelection()
	} else {
		in.beginExtend()
	}
	in.col = 0
	in.settleExtend()
}

func (in *Input) End()      { in.end(false) }
func (in *Input) ShiftEnd() { in.end(true) }

func (in *Input) end(extend bool) {
	if !extend {
		in.clearSelection()
	} else {
		in.beginExtend()
	}
	in.col = lineWidth(in.lines[in.row])
	in.settleExtend()
}

func (in *Input) WordLeft()      { in.wordLeft(false) }
func (in *Input) ShiftWordLeft() { in.wordLeft(true) }

func (in *Input) wordLeft(extend bool) {
	if !extend {
		in.clearSelection()
	} else {
		in.beginExtend()
	}
	rs := []rune(in.lines[in.row])
	i := in.col
	for i > 0 && isSpace(rs[i-1]) {
		i--
	}
	for i > 0 && !isSpace(rs[i-1]) {
		i--
	}
	in.col = i
	in.ensureCursorVisible()
	in.settleExtend()
}

func (in *Input) WordRight()      { in.wordRight(false) }
func (in *Input) ShiftWordRight() { in.wordRight(true) }

func (in *Input) wordRight(extend bool) {
	if !extend {
		in.clearSelection()
	} else {
		in.beginExtend()
	}
	rs := []rune(in.lines[in.row])
	i := in.col
	for i < len(rs) && !isSpace(rs[i]) {
		i++
	}
	for i < len(rs) && isSpace(rs[i]) {
		i++
	}
	in.col = i
	in.ensureCursorVisible()
	in.settleExtend()
}

func (in *Input) SelectAll() {
	in.anchor = &textPos{0, 0}
	in.row = len(in.lines) - 1
	in.col = lineWidth(in.lines[in.row])
	in.ensureCursorVisible()
}

func (in *Input) KillToEnd() {
	in.saveUndo()
	if in.anchor != nil {
		in.killRegister = in.deleteSelection()
		in.ensureCursorVisible()
		return
	}
	rs := []rune(in.lines[in.row])
	in.killRegister = string(rs[in.col:])
	in.lines[in.row] = string(rs[:in.col])
	in.ensureCursorVisible()
}

func (in *Input) KillToStart() {
	in.saveUndo()
	if in.anchor != nil {
		in.killRegister = in.deleteSelection()
		in.ensureCursorVisible()
		return
	}
	rs := []rune(in.lines[in.row])
	in.killRegister = string(rs[:in.col])
	in.lines[in.row] = string(rs[in.col:])
	in.col = 0
	in.ensureCursorVisible()
}

func (in *Input) DeleteWord() {
	in.saveUndo()
	if in.anchor != nil {
		in.killRegister = in.deleteSelection()
		in.ensureCursorVisible()
		return
	}
	rs := []rune(in.lines[in.row])
	i := in.col
	for i > 0 && isSpace(rs[i-1]) {
		i--
	}
	for i > 0 && !isSpace(rs[i-1]) {
		i--
	}
	in.killRegister = string(rs[i:in.col])
	in.lines[in.row] = string(append(rs[:i], rs[in.col:]...))
	in.col = i
	in.ensureCursorVisible()
}

func (in *Input) PasteKill() {
	if in.killRegister == "" {
		return
	}
	in.saveUndo()
	if in.anchor != nil {
		in.deleteSelection()
	}
	rs := []rune(in.lines[in.row])
	ins := []rune(in.killRegister)
	rs = append(rs[:in.col], append(ins, rs[in.col:]...)...)
	in.lines[in.row] = string(rs)
	in.col += len(ins)
	in.ensureCursorVisible()
}

func (in *Input) DeleteLine() {
	in.saveUndo()
	in.clearSelection()
	in.killRegister = in.lines[in.row]
	if len(in.lines) == 1 {
		in.lines[0] = ""
		in.row, in.col = 0, 0
		in.ensureCursorVisible()
		return
	}
	in.lines = append(in.lines[:in.row], in.lines[in.row+1:]...)
	if in.row >= len(in.lines) {
		in.row = len(in.lines) - 1
	}
	in.col = 0
	in.ensureCursorVisible()
}

func (in *Input) MoveLineUp() {
	if in.row == 0 {
		return
	}
	in.saveUndo()
	in.clearSelection()
	in.lines[in.row-1], in.lines[in.row] = in.lines[in.row], in.lines[in.row-1]
	in.row--
	in.ensureCursorVisible()
}

func (in *Input) MoveLineDown() {
	if in.row == len(in.lines)-1 {
		return
	}
	in.saveUndo()
	in.clearSelection()
	in.lines[in.row+1], in.lines[in.row] = in.lines[in.row], in.lines[in.row+1]
	in.row++
	in.ensureCursorVisible()
}

func (in *Input) Undo() {
	if len(in.undo) == 0 {
		return
	}
	s := in.undo[len(in.undo)-1]
	in.undo = in.undo[:len(in.undo)-1]
	in.redo = append(in.redo, inputSnapshot{copyLines(in.lines), in.row, in.col, clonePos(in.anchor)})
	in.lines, in.row, in.col, in.anchor = copyLines(s.lines), s.row, s.col, clonePos(s.anchor)
	in.ensureCursorVisible()
}

func (in *Input) Redo() {
	if len(in.redo) == 0 {
		return
	}
	s := in.redo[len(in.redo)-1]
	in.redo = in.redo[:len(in.redo)-1]
	in.undo = append(in.undo, inputSnapshot{copyLines(in.lines), in.row, in.col, clonePos(in.anchor)})
	in.lines, in.row, in.col, in.anchor = copyLines(s.lines), s.row, s.col, clonePos(s.anchor)
	in.ensureCursorVisible()
}

func (in *Input) Submit() {
	if strings.TrimSpace(in.Value()) == "" {
		return
	}
	if in.OnSubmit != nil {
		in.OnSubmit(in.Value())
	}
	in.Clear()
}

func (in *Input) Clear() {
	in.lines = []string{""}
	in.row, in.col, in.scroll, in.hscroll = 0, 0, 0, 0
	in.anchor = nil
	in.undo, in.redo = nil, nil
	in.chips = nil
	in.mentions = nil
	in.longTexts = make(map[rune]string)
}

func (in *Input) SetValue(text string) {
	in.Clear()
	if text == "" {
		in.lastActivity = time.Now()
		in.ensureCursorVisible()
		return
	}
	in.lines = strings.Split(text, "\n")
	in.row = len(in.lines) - 1
	in.col = len([]rune(in.lines[in.row]))
	in.lastActivity = time.Now()
	in.ensureCursorVisible()
}

func (in *Input) HandleKey(ev *tcell.EventKey) bool {
	if !in.focused {
		return false
	}
	in.lastActivity = time.Now()
	key := ev.Key()
	mod := ev.Modifiers()
	ctrl := mod&tcell.ModCtrl != 0
	shift := mod&tcell.ModShift != 0
	alt := mod&tcell.ModAlt != 0

	if ctrl && shift {
		switch {
		case key == tcell.KeyCtrlK:
			in.DeleteLine()
			return true
		case key == tcell.KeyCtrlZ:
			in.Redo()
			return true
		case key == tcell.KeyUp:
			in.MoveLineUp()
			return true
		case key == tcell.KeyDown:
			in.MoveLineDown()
			return true
		case key == tcell.KeyLeft:
			in.ShiftWordLeft()
			return true
		case key == tcell.KeyRight:
			in.ShiftWordRight()
			return true
		case key == tcell.KeyHome:
			in.ShiftHome()
			return true
		case key == tcell.KeyEnd:
			in.ShiftEnd()
			return true
		}
	}

	if ctrl {
		switch key {
		case tcell.KeyCtrlA:
			in.SelectAll()
			return true
		case tcell.KeyCtrlE:
			in.End()
			return true
		case tcell.KeyCtrlK:
			in.KillToEnd()
			return true
		case tcell.KeyCtrlU:
			in.KillToStart()
			return true
		case tcell.KeyCtrlW:
			in.DeleteWord()
			return true
		case tcell.KeyCtrlD:
			in.Delete()
			return true
		case tcell.KeyCtrlY:
			in.PasteKill()
			return true
		case tcell.KeyCtrlZ:
			in.Undo()
			return true
		case tcell.KeyLeft:
			in.WordLeft()
			return true
		case tcell.KeyRight:
			in.WordRight()
			return true
		case tcell.KeyUp:
			in.Home()
			return true
		case tcell.KeyDown:
			in.End()
			return true
		}
	}

	if alt {
		switch key {
		case tcell.KeyLeft:
			in.WordLeft()
			return true
		case tcell.KeyRight:
			in.WordRight()
			return true
		case tcell.KeyBackspace, tcell.KeyBackspace2:
			in.DeleteWord()
			return true
		}
	}

	switch key {
	case tcell.KeyEnter:
		if shift {
			in.Newline()
		} else {
			in.Submit()
		}
		return true
	case tcell.KeyBackspace, tcell.KeyBackspace2:
		in.Backspace()
		return true
	case tcell.KeyDelete:
		in.Delete()
		return true
	case tcell.KeyLeft:
		if shift {
			in.ShiftLeft()
		} else {
			in.MoveLeft()
		}
		return true
	case tcell.KeyRight:
		if shift {
			in.ShiftRight()
		} else {
			in.MoveRight()
		}
		return true
	case tcell.KeyUp:
		if shift {
			in.ShiftUp()
		} else {
			in.MoveUp()
		}
		return true
	case tcell.KeyDown:
		if shift {
			in.ShiftDown()
		} else {
			in.MoveDown()
		}
		return true
	case tcell.KeyHome:
		if shift {
			in.ShiftHome()
		} else {
			in.Home()
		}
		return true
	case tcell.KeyEnd:
		if shift {
			in.ShiftEnd()
		} else {
			in.End()
		}
		return true
	case tcell.KeyTab:
		in.InsertRune('\t')
		return true
	case tcell.KeyRune:
		if r := ev.Rune(); r != 0 {
			in.InsertRune(r)
			return true
		}
	}
	return false
}

func (in *Input) Draw(s tcell.Screen, bounds layout.Region, focused bool) {
	th := styles.Current()
	in.focused = focused

	textStyle := th.Base().Foreground(th.InputText).Background(th.InputBg)
	if bounds.Width < 2 || bounds.Height < 2 {
		return
	}

	innerW := bounds.Width - 2
	if innerW < 1 {
		innerW = 1
	}
	in.innerW = innerW

	wrapped := in.wrapLines(innerW)
	visible := in.VisibleRows()
	selStart, selEnd, hasSel := in.SelectionBounds()
	cursorOn := in.CursorVisible(time.Now())

	in.ensureCursorVisible()

	FillRegion(s, bounds, ' ', th.Base().Background(th.InputBg))
	DrawBox(s, bounds, th.Base().Foreground(th.Border))

	for i := 0; i < visible; i++ {
		si := in.scroll + i
		if si >= len(wrapped) {
			break
		}
		seg := wrapped[si]
		line := in.lines[seg.lineIdx]
		rs := []rune(line)
		isCursor := seg.lineIdx == in.row

		start := seg.start
		end := seg.end

		x := bounds.Left + 1
		y := bounds.Top + 1 + i
		for j := start; j < end; j++ {
			if x >= bounds.Right()-1 {
				break
			}
			selected := hasSel && cellSelected(seg.lineIdx, j, selStart, selEnd)
			cursorHere := isCursor && focused && cursorOn && j == in.col
			if full, ok := in.longTexts[rs[j]]; ok {
				preview := full
				if len(preview) > longTextPreviewLen {
					preview = preview[:longTextPreviewLen]
				}
				style := th.Base().Foreground(th.InputBg).Background(th.Mention)
				switch {
				case selected:
					style = th.Base().Foreground(th.Mention).Background(th.Selection)
				}
				label := "[" + preview + "]"
				if x+len(label) <= bounds.Right()-1 {
					drawLabel(s, x, y, label, style, cursorHere, th)
				}
				x += len(label)
				continue
			}
			if name, ok := chipNameOf(rs[j], in.chips); ok {
				style := th.Base().Foreground(th.InputBg).Background(th.Chip)
				switch {
				case selected:
					style = th.Base().Foreground(th.Chip).Background(th.Selection)
				}
				label := "/" + name
				if x+len(label) <= bounds.Right()-1 {
					drawLabel(s, x, y, label, style, cursorHere, th)
				}
				x += len(label)
				continue
			}
			if name, ok := mentionNameOf(rs[j], in.mentions); ok {
				style := th.Base().Foreground(th.InputBg).Background(th.Mention)
				switch {
				case selected:
					style = th.Base().Foreground(th.Mention).Background(th.Selection)
				}
				label := "@" + name
				if x+len(label) <= bounds.Right()-1 {
					drawLabel(s, x, y, label, style, cursorHere, th)
				}
				x += len(label)
				continue
			}
			style := textStyle
			switch {
			case cursorHere:
				style = th.Base().Foreground(th.InputBg).Background(th.Cursor)
			case selected:
				style = th.Base().Foreground(th.InputText).Background(th.Selection)
			}
			s.SetContent(x, y, rs[j], nil, style)
			x++
		}
		if isCursor && focused && cursorOn && in.col >= len(rs) && in.col == seg.end {
			cx := bounds.Left + 1
			for k := seg.start; k < in.col && k < len(rs); k++ {
				cx += in.runeWidth(rs[k])
			}
			if cx < bounds.Right()-1 {
				s.SetContent(cx, y, ' ', nil,
					th.Base().Foreground(th.InputBg).Background(th.Cursor))
			}
		}
	}

	if in.Empty() && in.Placeholder != "" {
		x := bounds.Left + 1
		y := bounds.Top + 1 + in.visualRowOf(in.row, in.col) - in.scroll
		if y >= bounds.Top && y < bounds.Bottom()-1 {
			style := textStyle.Foreground(th.Placeholder)
			DrawText(s, x, y, in.Placeholder, style)
		}
	}
}

func drawLabel(s tcell.Screen, x, y int, label string, style tcell.Style, cursorHere bool, th styles.Theme) {
	if cursorHere {
		rs := []rune(label)
		if len(rs) == 0 {
			return
		}
		cursorStyle := th.Base().Foreground(th.InputBg).Background(th.Cursor)
		s.SetContent(x, y, rs[0], nil, cursorStyle)
		if len(rs) > 1 {
			DrawText(s, x+1, y, string(rs[1:]), style)
		}
		return
	}
	DrawText(s, x, y, label, style)
}

func cellSelected(row, col int, s, e textPos) bool {
	if row < s.row || row > e.row {
		return false
	}
	switch {
	case row == s.row && row == e.row:
		return col >= s.col && col < e.col
	case row == s.row:
		return col >= s.col
	case row == e.row:
		return col < e.col
	default:
		return true
	}
}
