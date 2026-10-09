package components

import (
	"fmt"

	"github.com/gdamore/tcell/v2"

	"github.com/peggco/pegg/internal/llm"
	"github.com/peggco/pegg/internal/tui/layout"
	"github.com/peggco/pegg/internal/tui/styles"
)

type StatusBar struct {
	running         bool
	modelName       string
	provider        string
	reasoningEffort string
	usage           llm.Usage
	maxInputTokens  int
}

func NewStatusBar() *StatusBar { return &StatusBar{} }

func (sb *StatusBar) SetRunning(r bool)           { sb.running = r }
func (sb *StatusBar) SetModelName(n string)       { sb.modelName = n }
func (sb *StatusBar) SetProvider(p string)        { sb.provider = p }
func (sb *StatusBar) SetReasoningEffort(e string) { sb.reasoningEffort = e }
func (sb *StatusBar) SetUsage(u llm.Usage)        { sb.usage = u }
func (sb *StatusBar) SetMaxInputTokens(n int)     { sb.maxInputTokens = n }

func (sb *StatusBar) HandleKey(*tcell.EventKey) bool { return false }

func (sb *StatusBar) Draw(s tcell.Screen, bounds layout.Region, _ bool) {
	th := styles.Current()
	base := th.Base().Background(th.StatusBar)
	FillRegion(s, bounds, ' ', base)

	y := bounds.Top
	innerW := bounds.Width - 2
	if innerW < 1 {
		return
	}

	leftX := bounds.Left + 1

	if sb.running {
		DrawText(s, leftX, y, "●", base.Foreground(th.Accent).Bold(true))
	} else {
		DrawText(s, leftX, y, "●", base.Foreground(th.Hint))
	}
	leftX += 2

	if sb.modelName != "" {
		DrawText(s, leftX, y, sb.modelName, base.Foreground(th.Foreground))
		leftX += len(sb.modelName)
	}
	if sb.provider != "" {
		DrawText(s, leftX, y, "/"+sb.provider, base.Foreground(th.Hint))
		leftX += len(sb.provider) + 1
	}
	if sb.reasoningEffort != "" {
		DrawText(s, leftX, y, " ["+sb.reasoningEffort+"]", base.Foreground(th.Accent))
		leftX += len(sb.reasoningEffort) + 3
	}

	rightX := bounds.Right() - 1

	type segment struct {
		text  string
		style tcell.Style
	}
	var segs []segment

	segs = append(segs, segment{"Ctrl+P", base.Foreground(th.Foreground)})

	if sb.usage.Cost > 0 {
		segs = append(segs, segment{fmt.Sprintf("$%.4f", sb.usage.Cost), base.Foreground(th.Foreground)})
	}

	if sb.usage.TotalTokens > 0 {
		segs = append(segs, segment{llm.FormatContextUsage(sb.usage, sb.maxInputTokens), base.Foreground(th.Foreground)})
	}

	for i, seg := range segs {
		x := rightX - len(seg.text)
		if x >= bounds.Left {
			DrawText(s, x, y, seg.text, seg.style)
		}
		rightX -= len(seg.text)
		if i < len(segs)-1 {
			rightX -= 1
			if rightX >= bounds.Left {
				s.SetContent(rightX, y, '·', nil, base.Foreground(th.Hint))
			}
			rightX -= 1
		}
	}
}
