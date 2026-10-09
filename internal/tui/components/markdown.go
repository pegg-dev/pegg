package components

import (
	"fmt"
	"strings"

	"github.com/alecthomas/chroma/v2"
	"github.com/gdamore/tcell/v2"
	"github.com/peggco/pegg/internal/tui/styles"
	"github.com/yuin/goldmark"
	gast "github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	extast "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
)

type MdSeg struct {
	Text          string
	Bold          bool
	Italic        bool
	Code          bool
	Strikethrough bool
}

type MdLine struct {
	Segs     []MdSeg
	Styled   []Segment
	Heading  int
	Quote    bool
	Bullet   bool
	Hr       bool
	Code     bool
	Lang     string
	Table    bool
	TableRow bool
}

var defaultMarkdown = goldmark.New(
	goldmark.WithExtensions(extension.GFM),
	goldmark.WithParserOptions(parser.WithAutoHeadingID()),
)

func RenderMarkdown(src string) []MdLine {
	return parseMarkdownToMdLines(src)
}

func parseMarkdownToMdLines(src string) []MdLine {
	source := []byte(src)
	reader := text.NewReader(source)
	doc := defaultMarkdown.Parser().Parse(reader)

	var out []MdLine
	for c := doc.FirstChild(); c != nil; c = c.NextSibling() {
		out = append(out, processNode(c, source)...)
	}
	return out
}

func processNode(n gast.Node, source []byte) []MdLine {
	switch v := n.(type) {
	case *gast.Heading:
		return processHeading(v, source)
	case *gast.ThematicBreak:
		return []MdLine{{Hr: true}}
	case *gast.Blockquote:
		return processBlockquote(v, source)
	case *gast.List:
		return processList(v, source)
	case *gast.FencedCodeBlock:
		return processFencedCodeBlock(v, source)
	case *gast.CodeBlock:
		return processCodeBlock(v, source)
	case *gast.Paragraph:
		return processParagraph(v, source)
	case *gast.HTMLBlock:
		return processHTMLBlock(v, source)
	case *extast.Table:
		return processTable(v, source)
	default:
		return processParagraph(v, source)
	}
}

func processHeading(n *gast.Heading, source []byte) []MdLine {
	segs := extractInline(n, source)
	return []MdLine{{Segs: segs, Heading: n.Level}}
}

func processBlockquote(n *gast.Blockquote, source []byte) []MdLine {
	var lines []MdLine
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		childLines := processNode(c, source)
		for i := range childLines {
			childLines[i].Quote = true
		}
		lines = append(lines, childLines...)
	}
	if len(lines) == 0 {
		lines = append(lines, MdLine{Quote: true})
	}
	return lines
}

func processList(n *gast.List, source []byte) []MdLine {
	var lines []MdLine
	itemNum := n.Start
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		if li, ok := c.(*gast.ListItem); ok {
			segs := extractListItemInline(li, source)
			if n.IsOrdered() {
				marker := fmt.Sprintf("%d. ", itemNum)
				segs = append([]MdSeg{{Text: marker}}, segs...)
				itemNum++
			}
			lines = append(lines, MdLine{Segs: segs, Bullet: true})
		}
	}
	return lines
}

func processFencedCodeBlock(n *gast.FencedCodeBlock, source []byte) []MdLine {
	lang := ""
	if n.Info != nil {
		lang = string(n.Language(source))
	}

	var codeLines []string
	segLines := n.Lines()
	for i := 0; i < segLines.Len(); i++ {
		seg := segLines.At(i)
		codeLines = append(codeLines, string(seg.Value(source)))
	}
	code := strings.TrimRight(strings.Join(codeLines, ""), "\n")

	if lang != "" {
		return highlightCodeBlock(code, lang)
	}
	return plainCodeBlock(code, "fenced")
}

func processCodeBlock(n *gast.CodeBlock, source []byte) []MdLine {
	var codeLines []string
	segLines := n.Lines()
	for i := 0; i < segLines.Len(); i++ {
		seg := segLines.At(i)
		codeLines = append(codeLines, string(seg.Value(source)))
	}
	code := strings.TrimRight(strings.Join(codeLines, ""), "\n")
	return plainCodeBlock(code, "indented")
}

func highlightCodeBlock(code string, lang string) []MdLine {
	l := getCachedLexer(lang)
	iterator, err := l.Tokenise(nil, code)
	if err != nil {
		return plainCodeBlock(code, "fenced")
	}

	tokens := iterator.Tokens()
	var lines []MdLine

	lines = append(lines, MdLine{
		Code: true,
		Lang: lang,
		Segs: []MdSeg{{Text: "```" + lang}},
	})

	tokenLines := chroma.SplitTokensIntoLines(tokens)
	for _, lineTokens := range tokenLines {
		var styled []Segment
		for _, tok := range lineTokens {
			styled = append(styled, Segment{Text: tok.Value})
		}
		lines = append(lines, MdLine{
			Code:   true,
			Lang:   lang,
			Styled: styled,
		})
	}

	lines = append(lines, MdLine{
		Code: true,
		Segs: []MdSeg{{Text: "```"}},
	})
	return lines
}

func plainCodeBlock(code string, blockType string) []MdLine {
	firstLine := "```"
	if blockType == "fenced" {
		firstLine = "``` "
	}
	var lines []MdLine
	lines = append(lines, MdLine{Code: true, Segs: []MdSeg{{Text: firstLine}}})
	for _, cl := range strings.Split(code, "\n") {
		lines = append(lines, MdLine{Code: true, Segs: []MdSeg{{Text: cl}}})
	}
	lines = append(lines, MdLine{Code: true, Segs: []MdSeg{{Text: "```"}}})
	return lines
}

func processParagraph(n gast.Node, source []byte) []MdLine {
	segs := extractInline(n, source)
	if len(segs) == 0 {
		return nil
	}
	return []MdLine{{Segs: segs}}
}

func processHTMLBlock(n *gast.HTMLBlock, source []byte) []MdLine {
	var lines []MdLine
	htmlLines := n.Lines()
	for i := 0; i < htmlLines.Len(); i++ {
		seg := htmlLines.At(i)
		text := strings.TrimRight(string(seg.Value(source)), "\n")
		lines = append(lines, MdLine{Segs: []MdSeg{{Text: text}}})
	}
	return lines
}

func processTable(n *extast.Table, source []byte) []MdLine {
	var allRows [][]string

	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		switch row := c.(type) {
		case *extast.TableHeader:
			for tr := row.FirstChild(); tr != nil; tr = tr.NextSibling() {
				if tableRow, ok := tr.(*extast.TableRow); ok {
					var cells []string
					for cell := tableRow.FirstChild(); cell != nil; cell = cell.NextSibling() {
						if tc, ok := cell.(*extast.TableCell); ok {
							cells = append(cells, extractCellText(tc, source))
						}
					}
					allRows = append(allRows, cells)
				}
			}
		case *extast.TableRow:
			var cells []string
			for cell := row.FirstChild(); cell != nil; cell = cell.NextSibling() {
				if tc, ok := cell.(*extast.TableCell); ok {
					cells = append(cells, extractCellText(tc, source))
				}
			}
			allRows = append(allRows, cells)
		}
	}

	if len(allRows) == 0 {
		return nil
	}

	colCount := 0
	for _, row := range allRows {
		if len(row) > colCount {
			colCount = len(row)
		}
	}

	colWidths := make([]int, colCount)
	for _, row := range allRows {
		for i, cell := range row {
			if i < colCount && len(cell) > colWidths[i] {
				colWidths[i] = len(cell)
			}
		}
	}
	for i := range colWidths {
		if colWidths[i] < 3 {
			colWidths[i] = 3
		}
	}

	var lines []MdLine
	for rowIdx, row := range allRows {
		padded := make([]string, colCount)
		for i := 0; i < colCount; i++ {
			cell := ""
			if i < len(row) {
				cell = row[i]
			}
			padded[i] = padRight(cell, colWidths[i])
		}
		lines = append(lines, MdLine{
			Table:    true,
			TableRow: rowIdx == 0,
			Segs:     []MdSeg{{Text: "│ " + strings.Join(padded, " │ ") + " │"}},
		})
		if rowIdx == 0 {
			seps := make([]string, colCount)
			for i := range seps {
				seps[i] = strings.Repeat("─", colWidths[i])
			}
			lines = append(lines, MdLine{
				Table: true,
				Hr:    true,
				Segs:  []MdSeg{{Text: "│ " + strings.Join(seps, " │ ") + " │"}},
			})
		}
	}

	return lines
}

func padRight(s string, width int) string {
	if len(s) >= width {
		return s
	}
	return s + strings.Repeat(" ", width-len(s))
}

func extractCellText(n gast.Node, source []byte) string {
	var b strings.Builder
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		if t, ok := c.(*gast.Text); ok {
			b.Write(t.Value(source))
		} else {
			b.WriteString(nodePlainText(c, source))
		}
	}
	return strings.TrimSpace(b.String())
}

func extractListItemInline(li *gast.ListItem, source []byte) []MdSeg {
	var segs []MdSeg
	for c := li.FirstChild(); c != nil; c = c.NextSibling() {
		switch v := c.(type) {
		case *gast.TextBlock:
			segs = append(segs, extractInline(v, source)...)
		case *gast.Paragraph:
			segs = append(segs, extractInline(v, source)...)
		case *gast.List:
			childLines := processList(v, source)
			for _, l := range childLines {
				segs = append(segs, l.Segs...)
			}
		}
	}
	if len(segs) == 0 {
		segs = append(segs, MdSeg{Text: ""})
	}
	return segs
}

func extractInline(n gast.Node, source []byte) []MdSeg {
	var segs []MdSeg
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		segs = append(segs, inlineNodeToSegs(c, source)...)
	}
	if len(segs) == 0 {
		text := nodePlainText(n, source)
		if text != "" {
			segs = append(segs, MdSeg{Text: text})
		}
	}
	return segs
}

func inlineNodeToSegs(n gast.Node, source []byte) []MdSeg {
	switch v := n.(type) {
	case *gast.Text:
		return []MdSeg{{Text: string(v.Value(source))}}

	case *gast.String:
		return []MdSeg{{Text: string(v.Value)}}

	case *gast.CodeSpan:
		var segs []MdSeg
		for c := n.FirstChild(); c != nil; c = c.NextSibling() {
			if t, ok := c.(*gast.Text); ok {
				segs = append(segs, MdSeg{Text: string(t.Value(source)), Code: true})
			}
		}
		return segs

	case *gast.Emphasis:
		isBold := v.Level == 2
		var segs []MdSeg
		for c := n.FirstChild(); c != nil; c = c.NextSibling() {
			childSegs := inlineNodeToSegs(c, source)
			for i := range childSegs {
				if isBold {
					childSegs[i].Bold = true
				} else {
					childSegs[i].Italic = true
				}
			}
			segs = append(segs, childSegs...)
		}
		return segs

	case *extast.Strikethrough:
		var segs []MdSeg
		for c := n.FirstChild(); c != nil; c = c.NextSibling() {
			childSegs := inlineNodeToSegs(c, source)
			for i := range childSegs {
				childSegs[i].Strikethrough = true
			}
			segs = append(segs, childSegs...)
		}
		return segs

	case *gast.Link:
		var segs []MdSeg
		for c := n.FirstChild(); c != nil; c = c.NextSibling() {
			segs = append(segs, inlineNodeToSegs(c, source)...)
		}
		if len(segs) == 0 {
			segs = append(segs, MdSeg{Text: string(v.Destination)})
		}
		return segs

	case *gast.AutoLink:
		return []MdSeg{{Text: string(v.URL(source))}}

	case *gast.Image:
		return []MdSeg{{Text: "!" + string(v.Destination)}}

	case *gast.RawHTML:
		var segs []MdSeg
		for i := 0; i < v.Segments.Len(); i++ {
			seg := v.Segments.At(i)
			segs = append(segs, MdSeg{Text: string(seg.Value(source))})
		}
		return segs

	default:
		text := nodePlainText(n, source)
		if text != "" {
			return []MdSeg{{Text: text}}
		}
	}
	return nil
}

func nodePlainText(n gast.Node, source []byte) string {
	var b strings.Builder
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		if t, ok := c.(*gast.Text); ok {
			b.Write(t.Value(source))
		} else if s, ok := c.(*gast.String); ok {
			b.Write(s.Value)
		} else {
			b.WriteString(nodePlainText(c, source))
		}
	}
	return b.String()
}

func wrapSegs(segs []MdSeg, width int) [][]MdSeg {
	if width < 1 {
		width = 1
	}
	var out [][]MdSeg
	var cur []MdSeg
	curLen := 0
	flush := func() {
		if len(cur) > 0 {
			out = append(out, cur)
			cur = nil
			curLen = 0
		}
	}
	for _, seg := range segs {
		words := strings.Fields(seg.Text)
		if len(words) == 0 {
			continue
		}
		for _, w := range words {
			wl := DisplayWidth(w)
			if curLen > 0 && curLen+1+wl > width {
				flush()
			}
			if wl > width {
				for DisplayWidth(w) > width {
					cut := w
					cw := 0
					for i, r := range w {
						rw := RuneWidth(r)
						if cw+rw > width {
							cut = w[:i]
							break
						}
						cw += rw
					}
					cur = append(cur, MdSeg{Text: cut, Bold: seg.Bold, Italic: seg.Italic, Code: seg.Code, Strikethrough: seg.Strikethrough})
					curLen = width
					flush()
					w = w[len(cut):]
				}
				wl = DisplayWidth(w)
			}
			sp := ""
			if curLen > 0 {
				sp = " "
			}
			cur = append(cur, MdSeg{Text: sp + w, Bold: seg.Bold, Italic: seg.Italic, Code: seg.Code, Strikethrough: seg.Strikethrough})
			curLen += len(sp) + wl
		}
	}
	flush()
	if len(out) == 0 {
		out = append(out, []MdSeg{{Text: ""}})
	}
	return out
}

func (l MdLine) Text() string {
	var b strings.Builder
	for _, s := range l.Segs {
		b.WriteString(s.Text)
	}
	if len(l.Styled) > 0 && b.Len() == 0 {
		for _, s := range l.Styled {
			b.WriteString(s.Text)
		}
	}
	return b.String()
}

func MdToLines(src string, width int, th styles.Theme) []Line {
	md := parseMarkdownToMdLines(src)
	var out []Line

	blankStyle := th.Base().Background(th.Background)
	prevKind := ""
	addBlank := func() {
		out = append(out, LineFromSegments([]Segment{{Text: "", Style: blankStyle}}, width))
	}

	for _, ln := range md {
		kind := lineKind(ln)

		if kind == "heading" && prevKind != "" {
			addBlank()
		}
		if kind == "text" && prevKind == "text" {
			addBlank()
		}
		if kind == "code-open" || kind == "code-close" {
			if prevKind != "code-open" && prevKind != "code-body" && prevKind != "" {
				addBlank()
			}
		}
		if kind == "code-body" && prevKind == "" {
			addBlank()
		}
		if kind == "list" && prevKind != "" && prevKind != "list" {
			addBlank()
		}
		if kind == "table" && prevKind != "" && prevKind != "table" {
			addBlank()
		}

		switch {
		case ln.Code:
			out = append(out, renderCodeLine(ln, th, width)...)

		case ln.Hr:
			style := th.Base().Foreground(th.Border).Background(th.Background)
			text := ln.Text()
			if text == "" {
				text = strings.Repeat("─", width)
			}
			out = append(out, LineFromSegments([]Segment{{Text: text, Style: style}}, width))

		case ln.Table:
			out = append(out, renderTableLine(ln, th, width)...)

		default:
			out = append(out, renderProseLine(ln, th, width)...)
		}

		prevKind = kind
	}
	return trimLeadingBlankLines(out)
}

func lineKind(ln MdLine) string {
	if ln.Code {
		if ln.Styled != nil {
			return "code-body"
		}
		text := ln.Text()
		if text == "```" {
			return "code-close"
		}
		if strings.HasPrefix(text, "```") {
			return "code-open"
		}
		return "code-body"
	}
	if ln.Table {
		return "table"
	}
	if ln.Hr {
		return "hr"
	}
	if ln.Heading > 0 {
		return "heading"
	}
	if ln.Quote {
		return "quote"
	}
	if ln.Bullet {
		return "list"
	}
	return "text"
}

func renderCodeLine(ln MdLine, th styles.Theme, width int) []Line {
	var out []Line

	if ln.Styled != nil {
		styledSegs := applyTokenStyles(ln.Styled, ln.Lang, th)
		bgStyle := th.Base().Background(th.Background)
		for _, segs := range wrapStyledSegs(styledSegs, width) {
			padded := padCodeSegsToWidth(segs, width, bgStyle)
			out = append(out, LineFromSegments(padded, width))
		}
		return out
	}

	text := ln.Text()
	fence := strings.HasPrefix(text, "```")

	bgStyle := th.Base().Background(th.Background)
	codeStyle := bgStyle.Foreground(th.Hint)

	if fence {
		codeStyle = bgStyle.Foreground(th.Muted)
	}
	row := LineFromSegments(padToWidth([]Segment{{Text: text, Style: codeStyle}}, width, bgStyle), width)
	return append(out, row)
}

func renderTableLine(ln MdLine, th styles.Theme, width int) []Line {
	text := ln.Text()
	baseStyle := th.Base().Background(th.Background)
	borderStyle := baseStyle.Foreground(th.Border)

	if ln.Hr {
		return []Line{LineFromSegments(padToWidth([]Segment{{Text: text, Style: borderStyle}}, width, baseStyle), width)}
	}

	if ln.TableRow {
		segStyle := baseStyle.Foreground(th.Accent).Bold(true)
		seg := Segment{Text: text, Style: segStyle}
		return []Line{LineFromSegments(padToWidth([]Segment{seg}, width, baseStyle), width)}
	}

	segStyle := baseStyle.Foreground(th.Foreground)
	seg := Segment{Text: text, Style: segStyle}
	return []Line{LineFromSegments(padToWidth([]Segment{seg}, width, baseStyle), width)}
}

func renderProseLine(ln MdLine, th styles.Theme, width int) []Line {
	var out []Line
	blankStyle := th.Base().Background(th.Background)

	for _, segs := range wrapSegs(ln.Segs, width) {
		var segList []Segment
		baseStyle := th.Base().Background(th.Background)

		if ln.Heading > 0 {
			baseStyle = baseStyle.Foreground(th.Accent).Bold(true)
		} else if ln.Quote {
			baseStyle = baseStyle.Foreground(th.Hint)
		} else if ln.Bullet {
			baseStyle = baseStyle.Foreground(th.Foreground)
		}

		if ln.Quote {
			segList = append(segList, Segment{Text: "│ ", Style: baseStyle})
		} else if ln.Bullet {
			segList = append(segList, Segment{Text: "• ", Style: baseStyle.Foreground(th.Accent)})
		}

		if ln.Heading > 0 {
			segList = append(segList, Segment{Text: "▍ ", Style: th.Base().Foreground(th.AccentDim).Background(th.Background)})
		}

		for _, s := range segs {
			segStyle := baseStyle
			if s.Bold {
				segStyle = segStyle.Bold(true)
			}
			if s.Italic {
				segStyle = segStyle.Italic(true)
			}
			if s.Strikethrough {
				segStyle = segStyle.StrikeThrough(true)
			}
			if s.Code {
				segStyle = th.Base().Foreground(th.Accent).Background(th.CodeBg).Bold(true)
			}
			segList = append(segList, Segment{Text: s.Text, Style: segStyle})
		}
		out = append(out, LineFromSegments(segList, width))
	}

	if ln.Heading > 0 {
		out = append(out, LineFromSegments([]Segment{{Text: "", Style: blankStyle}}, width))
	}

	return out
}

func padToWidth(segs []Segment, width int, bgStyle tcell.Style) []Segment {
	w := 0
	for _, s := range segs {
		w += DisplayWidth(s.Text)
	}
	if w < width {
		segs = append(segs, Segment{Text: strings.Repeat(" ", width-w), Style: bgStyle})
	}
	return segs
}

func padCodeSegsToWidth(segs []Segment, width int, bgStyle tcell.Style) []Segment {
	w := 0
	for _, s := range segs {
		w += DisplayWidth(s.Text)
	}
	if w < width {
		segs = append(segs, Segment{Text: strings.Repeat(" ", width-w), Style: bgStyle})
	}
	return segs
}

func applyTokenStyles(segs []Segment, lang string, th styles.Theme) []Segment {
	l := getCachedLexer(lang)
	text := concatSegText(segs)
	iterator, err := l.Tokenise(nil, text)
	if err != nil {
		return segs
	}

	tokens := iterator.Tokens()
	codeStyle := th.Base().Background(th.Background)

	var result []Segment
	for _, tok := range tokens {
		colour := tokenTypeToThemeColor(tok.Type, th)
		result = append(result, Segment{
			Text:  tok.Value,
			Style: codeStyle.Foreground(colour),
		})
	}
	if len(result) == 0 {
		return []Segment{{Text: text, Style: codeStyle.Foreground(th.Foreground)}}
	}
	return result
}

func concatSegText(segs []Segment) string {
	var b strings.Builder
	for _, s := range segs {
		b.WriteString(s.Text)
	}
	return b.String()
}

func wrapStyledSegs(segs []Segment, width int) [][]Segment {
	if width < 1 {
		width = 1
	}
	var out [][]Segment
	var cur []Segment
	curLen := 0
	flush := func() {
		if len(cur) > 0 {
			out = append(out, cur)
			cur = nil
			curLen = 0
		}
	}
	for _, seg := range segs {
		lines := strings.Split(seg.Text, "\n")
		for i, line := range lines {
			if i > 0 {
				flush()
			}
			if line == "" {
				continue
			}
			wl := DisplayWidth(line)
			if curLen+wl > width && curLen > 0 {
				flush()
			}
			if wl > width {
				for len(line) > width {
					cut := truncateToWidth(line, width)
					cur = append(cur, Segment{Text: cut, Style: seg.Style})
					curLen = width
					flush()
					line = line[len(cut):]
				}
				if len(line) > 0 {
					cur = append(cur, Segment{Text: line, Style: seg.Style})
					curLen = DisplayWidth(line)
				}
			} else {
				cur = append(cur, Segment{Text: line, Style: seg.Style})
				curLen += wl
			}
		}
	}
	flush()
	if len(out) == 0 {
		out = append(out, []Segment{{Text: ""}})
	}
	return out
}

func truncateToWidth(s string, width int) string {
	w := 0
	for i := range s {
		rw := RuneWidth(rune(s[i]))
		if w+rw > width {
			return s[:i]
		}
		w += rw
	}
	return s
}

func trimLeadingBlankLines(lines []Line) []Line {
	i := 0
	for i < len(lines) && blankLine(lines[i]) {
		i++
	}
	return lines[i:]
}

func blankLine(l Line) bool {
	for _, c := range l {
		if c.R != ' ' && c.R != 0 {
			return false
		}
	}
	return true
}
