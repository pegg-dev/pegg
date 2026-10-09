package components

import (
	"testing"

	"github.com/peggco/pegg/internal/llm"
)

func TestAttachmentBarAddAndCount(t *testing.T) {
	ab := NewAttachmentBar()
	if ab.Count() != 0 {
		t.Fatalf("expected 0, got %d", ab.Count())
	}
	ab.Add(llm.Attachment{Type: llm.AttachmentTypeImage, FileName: "test.png"})
	if ab.Count() != 1 {
		t.Fatalf("expected 1, got %d", ab.Count())
	}
}

func TestAttachmentBarRemove(t *testing.T) {
	ab := NewAttachmentBar()
	ab.Add(llm.Attachment{Type: llm.AttachmentTypeImage, FileName: "a.png"})
	ab.Add(llm.Attachment{Type: llm.AttachmentTypeFile, FileName: "b.txt"})
	ab.Remove(0)
	if ab.Count() != 1 {
		t.Fatalf("expected 1, got %d", ab.Count())
	}
	if ab.Attachments()[0].FileName != "b.txt" {
		t.Fatalf("expected b.txt, got %s", ab.Attachments()[0].FileName)
	}
}

func TestAttachmentBarPagination(t *testing.T) {
	ab := NewAttachmentBar()
	ab.maxVisible = 3
	for i := 0; i < 7; i++ {
		ab.Add(llm.Attachment{Type: llm.AttachmentTypeFile, FileName: "file"})
	}
	if ab.TotalPages() != 3 {
		t.Fatalf("expected 3 pages, got %d", ab.TotalPages())
	}
	s, e := ab.VisibleRange()
	if s != 0 || e != 3 {
		t.Fatalf("expected [0,3), got [%d,%d)", s, e)
	}
	ab.SetPage(1)
	s, e = ab.VisibleRange()
	if s != 3 || e != 6 {
		t.Fatalf("expected [3,6), got [%d,%d)", s, e)
	}
}

func TestAttachmentBarDynamicPagination(t *testing.T) {
	ab := NewAttachmentBar()
	for i := 0; i < 10; i++ {
		ab.Add(llm.Attachment{Type: llm.AttachmentTypeFile, FileName: "file"})
	}

	ab.maxVisible = 5
	if ab.TotalPages() != 2 {
		t.Fatalf("expected 2 pages with maxVisible=5, got %d", ab.TotalPages())
	}
	s, e := ab.VisibleRange()
	if s != 0 || e != 5 {
		t.Fatalf("expected [0,5), got [%d,%d)", s, e)
	}

	ab.maxVisible = 2
	if ab.TotalPages() != 5 {
		t.Fatalf("expected 5 pages with maxVisible=2, got %d", ab.TotalPages())
	}
	s, e = ab.VisibleRange()
	if s != 0 || e != 2 {
		t.Fatalf("expected [0,2), got [%d,%d)", s, e)
	}
}

func TestAttachmentBarClear(t *testing.T) {
	ab := NewAttachmentBar()
	ab.Add(llm.Attachment{Type: llm.AttachmentTypeImage, FileName: "a.png"})
	ab.Add(llm.Attachment{Type: llm.AttachmentTypeFile, FileName: "b.txt"})
	ab.Clear()
	if ab.Count() != 0 {
		t.Fatalf("expected 0 after clear, got %d", ab.Count())
	}
}

func TestAttachmentBarRequiredHeight(t *testing.T) {
	ab := NewAttachmentBar()
	if ab.RequiredHeight() != 0 {
		t.Fatalf("expected 0 for empty bar")
	}
	ab.Add(llm.Attachment{Type: llm.AttachmentTypeFile, FileName: "f.txt"})
	if ab.RequiredHeight() != cardHeight+paginationHeight {
		t.Fatalf("expected %d, got %d", cardHeight+paginationHeight, ab.RequiredHeight())
	}
}

func TestAttachmentBarFocusIndex(t *testing.T) {
	ab := NewAttachmentBar()
	ab.Add(llm.Attachment{Type: llm.AttachmentTypeFile, FileName: "a.txt"})
	ab.Add(llm.Attachment{Type: llm.AttachmentTypeFile, FileName: "b.txt"})
	ab.Focus()
	if !ab.Focused() {
		t.Fatal("expected focused")
	}
	ab.SetFocusIndex(1)
	if ab.FocusIndex() != 1 {
		t.Fatalf("expected focus index 1, got %d", ab.FocusIndex())
	}
	ab.Blur()
	if ab.Focused() {
		t.Fatal("expected not focused")
	}
}

func TestAttachmentBarComputeMaxVisible(t *testing.T) {
	ab := NewAttachmentBar()
	tests := []struct {
		width int
		want  int
	}{
		{80, 3},
		{120, 4},
		{48, 1},
		{25, 1},
		{100, 4},
	}
	for _, tc := range tests {
		got := ab.computeMaxVisible(tc.width)
		if got != tc.want {
			t.Errorf("width=%d: expected maxVisible=%d, got %d", tc.width, tc.want, got)
		}
	}
}

func TestAttachmentBarSyncPageOnRightArrow(t *testing.T) {
	ab := NewAttachmentBar()
	ab.maxVisible = 3
	for i := 0; i < 7; i++ {
		ab.Add(llm.Attachment{Type: llm.AttachmentTypeFile, FileName: "f"})
	}
	ab.Focus()

	ab.SetFocusIndex(2)
	if ab.Page() != 0 {
		t.Fatalf("page should be 0, got %d", ab.Page())
	}

	ab.SetFocusIndex(3)
	if ab.Page() != 1 {
		t.Fatalf("page should be 1 after moving to index 3, got %d", ab.Page())
	}

	ab.SetFocusIndex(6)
	if ab.Page() != 2 {
		t.Fatalf("page should be 2 for last item, got %d", ab.Page())
	}

	ab.SetFocusIndex(0)
	if ab.Page() != 0 {
		t.Fatalf("page should be 0 after moving back to start, got %d", ab.Page())
	}
}
