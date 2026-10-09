package components

import (
	"github.com/gdamore/tcell/v2"

	"github.com/peggco/pegg/internal/tui/layout"
	"github.com/peggco/pegg/internal/tui/styles"
)

var logoArt1 = []string{
	" _ __   ___  __ _  __ _ ",
	"| '_ \\ / _ \\/ _` |/ _` |",
	"| |_) |  __/ (_| | (_| |",
	"| .__/ \\___|\\__, |\\__, |",
	"| |          __/ | __/ |",
	"|_|         |___/ |___/ ",
}

var logoArt2 = []string{
	" _ __   ___  __ _  __ _ ",
	"| '_ \\ / _ \\/ _` |/ _` |",
	"| |_) |  __/ (_| | (_| |",
	"| .__/ \\___|\\__, |\\__, |",
	"| |          __/ X \\__ |",
	"|_|         |___/ \\___/ ",
}

type Logo struct {
	Width   int
	Height  int
	blinkOn bool
}

func NewLogo(w, h int) *Logo {
	return &Logo{Width: w, Height: h}
}

func DefaultLogo() *Logo {
	w := len(logoArt1[0])
	for _, l := range logoArt1 {
		if len(l) > w {
			w = len(l)
		}
	}
	for _, l := range logoArt2 {
		if len(l) > w {
			w = len(l)
		}
	}
	return &Logo{Width: w, Height: len(logoArt1)}
}

func (lg *Logo) SetBlink(on bool) { lg.blinkOn = on }

func (lg *Logo) OnTick(blinkOn bool) { lg.blinkOn = blinkOn }

func (lg *Logo) HandleKey(*tcell.EventKey) bool { return false }

func (lg *Logo) Draw(s tcell.Screen, bounds layout.Region, _ bool) {
	th := styles.Current()
	FillRegion(s, bounds, ' ', th.Base().Background(th.Background))
	art := logoArt1
	if !lg.blinkOn {
		art = logoArt2
	}
	style := th.Base().Foreground(th.Accent).Bold(true)
	for i, line := range art {
		if bounds.Top+i >= bounds.Bottom() {
			break
		}
		DrawText(s, bounds.Left, bounds.Top+i, line, style)
	}
}
