package components

import (
	"path/filepath"
	"strings"

	"github.com/gdamore/tcell/v2"

	"github.com/peggco/pegg/internal/llm"
	"github.com/peggco/pegg/internal/tui/layout"
	"github.com/peggco/pegg/internal/tui/styles"
)

const (
	cardWidth        = 24
	cardHeight       = 3
	paginationHeight = 1
	cardGap          = 1
)

type AttachmentBar struct {
	attachments []llm.Attachment
	focusIndex  int
	page        int
	focused     bool
	maxVisible  int
	OnChange    func()
}

func NewAttachmentBar() *AttachmentBar {
	return &AttachmentBar{maxVisible: 3}
}

func (ab *AttachmentBar) computeMaxVisible(width int) int {
	m := (width + cardGap) / (cardWidth + cardGap)
	if m < 1 {
		m = 1
	}
	return m
}

func (ab *AttachmentBar) Add(att llm.Attachment) {
	ab.attachments = append(ab.attachments, att)
	totalPages := ab.TotalPages()
	if ab.page >= totalPages && totalPages > 0 {
		ab.page = totalPages - 1
	}
}

func (ab *AttachmentBar) Remove(index int) {
	if index < 0 || index >= len(ab.attachments) {
		return
	}
	ab.attachments = append(ab.attachments[:index], ab.attachments[index+1:]...)
	if ab.focusIndex >= len(ab.attachments) {
		ab.focusIndex = len(ab.attachments) - 1
	}
	if ab.focusIndex < 0 {
		ab.focusIndex = 0
	}
	totalPages := ab.TotalPages()
	if ab.page >= totalPages && totalPages > 0 {
		ab.page = totalPages - 1
	}
	if ab.OnChange != nil {
		ab.OnChange()
	}
}

func (ab *AttachmentBar) Count() int { return len(ab.attachments) }

func (ab *AttachmentBar) Attachments() []llm.Attachment { return ab.attachments }

func (ab *AttachmentBar) Clear() {
	ab.attachments = nil
	ab.focusIndex = 0
	ab.page = 0
}

func (ab *AttachmentBar) FocusIndex() int { return ab.focusIndex }

func (ab *AttachmentBar) SetFocusIndex(i int) {
	if i < 0 {
		i = 0
	}
	if i >= len(ab.attachments) {
		i = len(ab.attachments) - 1
	}
	if i < 0 {
		i = 0
	}
	ab.focusIndex = i
	ab.syncPage()
}

func (ab *AttachmentBar) Focused() bool { return ab.focused }

func (ab *AttachmentBar) Focus() { ab.focused = true }

func (ab *AttachmentBar) Blur() { ab.focused = false }

func (ab *AttachmentBar) Page() int { return ab.page }

func (ab *AttachmentBar) SetPage(p int) {
	totalPages := ab.TotalPages()
	if totalPages == 0 {
		ab.page = 0
		return
	}
	if p < 0 {
		p = 0
	}
	if p >= totalPages {
		p = totalPages - 1
	}
	ab.page = p
}

func (ab *AttachmentBar) TotalPages() int {
	mv := ab.maxVisible
	if len(ab.attachments) <= mv {
		return 1
	}
	return (len(ab.attachments) + mv - 1) / mv
}

func (ab *AttachmentBar) syncPage() {
	mv := ab.maxVisible
	if len(ab.attachments) <= mv {
		ab.page = 0
		return
	}
	pageStart := ab.page * mv
	pageEnd := pageStart + mv
	if ab.focusIndex < pageStart {
		ab.page = ab.focusIndex / mv
	} else if ab.focusIndex >= pageEnd {
		ab.page = ab.focusIndex / mv
	}
}

func (ab *AttachmentBar) VisibleRange() (start, end int) {
	mv := ab.maxVisible
	start = ab.page * mv
	end = start + mv
	if end > len(ab.attachments) {
		end = len(ab.attachments)
	}
	return
}

func (ab *AttachmentBar) HandleKey(ev *tcell.EventKey) bool {
	if !ab.focused || len(ab.attachments) == 0 {
		return false
	}
	switch ev.Key() {
	case tcell.KeyLeft:
		if ab.focusIndex > 0 {
			ab.focusIndex--
			ab.syncPage()
		}
		return true
	case tcell.KeyRight:
		if ab.focusIndex < len(ab.attachments)-1 {
			ab.focusIndex++
			ab.syncPage()
		}
		return true
	case tcell.KeyBackspace, tcell.KeyBackspace2, tcell.KeyDelete:
		ab.Remove(ab.focusIndex)
		return true
	}
	return false
}

func (ab *AttachmentBar) RequiredHeight() int {
	if len(ab.attachments) == 0 {
		return 0
	}
	return cardHeight + paginationHeight
}

func (ab *AttachmentBar) Draw(s tcell.Screen, bounds layout.Region, focused bool) {
	if len(ab.attachments) == 0 {
		return
	}

	ab.maxVisible = ab.computeMaxVisible(bounds.Width)

	start, end := ab.VisibleRange()
	visible := ab.attachments[start:end]

	totalCards := len(visible)
	for i, att := range visible {
		absIndex := start + i
		cardRegion := ab.cardBounds(bounds, i, totalCards)
		ab.drawCard(s, cardRegion, att, absIndex == ab.focusIndex && ab.focused)
	}

	if ab.TotalPages() > 1 {
		paginationY := bounds.Top + cardHeight
		ab.drawPagination(s, bounds, paginationY)
	}
}

func (ab *AttachmentBar) cardBounds(bounds layout.Region, index, total int) layout.Region {
	cardW := cardWidth
	gap := cardGap
	totalW := total*cardW + (total-1)*gap
	startX := bounds.Left + (bounds.Width-totalW)/2
	if startX < bounds.Left {
		startX = bounds.Left
	}
	return layout.Region{
		Left:   startX + index*(cardW+gap),
		Top:    bounds.Top,
		Width:  cardW,
		Height: cardHeight,
	}
}

func (ab *AttachmentBar) drawCard(s tcell.Screen, bounds layout.Region, att llm.Attachment, selected bool) {
	th := styles.Current()
	var style tcell.Style
	if selected {
		style = th.Base().Foreground(th.Accent).Background(th.InputBg)
	} else {
		style = th.Base().Foreground(th.Foreground).Background(th.InputBg)
	}

	FillRegion(s, bounds, ' ', style)
	DrawBox(s, bounds, style)

	icon := ab.fileIcon(att)
	name := ab.fileName(att)

	contentY := bounds.Top + 1
	iconX := bounds.Left + 2
	DrawText(s, iconX, contentY, icon, style)

	nameX := iconX + len(icon) + 1
	maxNameW := bounds.Width - 4 - len(icon) - 1
	if maxNameW < 1 {
		maxNameW = 1
	}
	truncated := TruncateTo(name, maxNameW)
	DrawText(s, nameX, contentY, truncated, style)
}

func (ab *AttachmentBar) fileIcon(att llm.Attachment) string {
	switch att.Type {
	case llm.AttachmentTypeImage:
		return "🖼"
	case llm.AttachmentTypeAudio:
		return "🎵"
	default:
		return "📄"
	}
}

func (ab *AttachmentBar) fileName(att llm.Attachment) string {
	if att.FileName != "" {
		return att.FileName
	}
	if att.URL != "" {
		return filepath.Base(att.URL)
	}
	return "attachment"
}

func (ab *AttachmentBar) drawPagination(s tcell.Screen, bounds layout.Region, y int) {
	th := styles.Current()
	style := th.Base().Foreground(th.Hint).Background(th.Background)

	totalPages := ab.TotalPages()
	var dots strings.Builder
	for i := 0; i < totalPages; i++ {
		if i == ab.page {
			dots.WriteString("●")
		} else {
			dots.WriteString("○")
		}
		if i < totalPages-1 {
			dots.WriteString(" ")
		}
	}

	dotsStr := dots.String()
	dotsW := len(dotsStr)
	dotsX := bounds.Left + (bounds.Width-dotsW)/2
	if dotsX < bounds.Left {
		dotsX = bounds.Left
	}
	if y < bounds.Bottom() {
		DrawText(s, dotsX, y, dotsStr, style)
	}
}
