package components

import (
	"github.com/gdamore/tcell/v2"

	"github.com/peggco/pegg/internal/tui/layout"
	"github.com/peggco/pegg/internal/tui/styles"
)

func DrawModalBackdrop(s tcell.Screen, bounds layout.Region) {
	th := styles.Current()
	FillRegion(s, bounds, ' ', th.Base().Background(th.Background))
}

func centeredBox(outer layout.Region, w, h int) (box, inner layout.Region) {
	w = min(w, outer.Width)
	h = min(h, outer.Height)
	box = layout.CenterIn(outer, w, h)
	if box.Width < 2 || box.Height < 2 {
		return box, box
	}
	return box, layout.Pad(box, 1, 1)
}

func CenteredBoxRegion(outer layout.Region, w, h int) layout.Region {
	_, inner := centeredBox(outer, w, h)
	return inner
}

func DrawCenteredBox(s tcell.Screen, outer layout.Region, w, h int, title string) layout.Region {
	th := styles.Current()
	box, inner := centeredBox(outer, w, h)
	if box.Width < 2 || box.Height < 2 {
		return box
	}
	bg := th.Base().Background(th.InputBg)
	FillRegion(s, box, ' ', bg)
	DrawBox(s, box, th.Base().Foreground(th.Border))
	if title != "" {
		t := " " + TruncateTo(title, box.Width-6) + " "
		DrawText(s, box.Left+2, box.Top, t, th.Base().Foreground(th.Accent).Background(th.InputBg))
	}
	return inner
}

func DrawFooter(s tcell.Screen, bounds layout.Region, text string) {
	th := styles.Current()
	DrawText(s, bounds.Left, bounds.Bottom()-1, TruncateTo(text, bounds.Width),
		th.Base().Foreground(th.Hint).Background(th.InputBg))
}
