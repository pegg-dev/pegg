package components

import (
	"strings"
	"testing"
)

func TestVisibleRowsGrows(t *testing.T) {
	in := NewInput()
	if got := in.VisibleRows(); got != 3 {
		t.Fatalf("expected 3 rows on empty input, got %d", got)
	}
	for i := 0; i < 3; i++ {
		in.Newline()
	}
	if got := in.VisibleRows(); got != 4 {
		t.Fatalf("expected 4 rows after 3 newlines, got %d", got)
	}
	for i := 0; i < 5; i++ {
		in.Newline()
	}
	if got := in.VisibleRows(); got != 6 {
		t.Fatalf("expected rows capped at 6, got %d", got)
	}
}

func TestInsertRune(t *testing.T) {
	in := NewInput()
	for _, r := range "hello" {
		in.InsertRune(r)
	}
	if in.Value() != "hello" {
		t.Errorf("value = %q, want hello", in.Value())
	}
	if in.Col() != 5 {
		t.Errorf("col = %d, want 5", in.Col())
	}
}

func TestInsertMiddle(t *testing.T) {
	in := NewInput()
	for _, r := range "hllo" {
		in.InsertRune(r)
	}
	if in.Value() != "hllo" {
		t.Fatalf("setup value = %q, want hllo", in.Value())
	}
	in.Home()
	in.MoveRight()
	in.InsertRune('e')
	if in.Value() != "hello" {
		t.Errorf("value = %q, want hello", in.Value())
	}
}

func TestBackspace(t *testing.T) {
	in := NewInput()
	for _, r := range "hello" {
		in.InsertRune(r)
	}
	in.Backspace()
	if in.Value() != "hell" {
		t.Errorf("value = %q, want hell", in.Value())
	}
	if in.Col() != 4 {
		t.Errorf("col = %d, want 4", in.Col())
	}
}

func TestBackspaceJoinsLines(t *testing.T) {
	in := NewInput()
	for _, r := range "abc" {
		in.InsertRune(r)
	}
	in.Newline()
	for _, r := range "def" {
		in.InsertRune(r)
	}
	in.MoveLeft()
	in.MoveLeft()
	in.MoveLeft()
	in.Backspace()
	if in.Value() != "abcdef" {
		t.Errorf("value = %q, want abcdef", in.Value())
	}
}

func TestNewlineSplits(t *testing.T) {
	in := NewInput()
	for _, r := range "abcdef" {
		in.InsertRune(r)
	}
	in.Home()
	for i := 0; i < 3; i++ {
		in.MoveRight()
	}
	in.Newline()
	if in.Value() != "abc\ndef" {
		t.Errorf("value = %q, want abc\\ndef", in.Value())
	}
	if in.Row() != 1 || in.Col() != 0 {
		t.Errorf("cursor = (%d,%d), want (1,0)", in.Row(), in.Col())
	}
}

func TestDeleteForward(t *testing.T) {
	in := NewInput()
	for _, r := range "hello" {
		in.InsertRune(r)
	}
	in.Home()
	in.Delete()
	if in.Value() != "ello" {
		t.Errorf("value = %q, want ello", in.Value())
	}
}

func TestMovement(t *testing.T) {
	in := NewInput()
	for _, r := range "hello" {
		in.InsertRune(r)
	}
	in.Home()
	in.End()
	in.MoveLeft()
	in.MoveLeft()
	if in.Col() != 3 {
		t.Errorf("col = %d, want 3", in.Col())
	}
}

func TestMultiLineMovement(t *testing.T) {
	in := NewInput()
	for _, r := range "ab" {
		in.InsertRune(r)
	}
	in.Newline()
	for _, r := range "cd" {
		in.InsertRune(r)
	}
	in.MoveUp()
	if in.Row() != 0 {
		t.Errorf("row = %d, want 0", in.Row())
	}
	if in.Col() != 2 {
		t.Errorf("col = %d, want 2", in.Col())
	}
	in.MoveDown()
	if in.Row() != 1 || in.Col() != 2 {
		t.Errorf("cursor = (%d,%d), want (1,2)", in.Row(), in.Col())
	}
}

func TestWordNavigation(t *testing.T) {
	in := NewInput()
	for _, r := range "one two three" {
		in.InsertRune(r)
	}
	in.Home()
	in.WordRight()
	if in.Col() != 4 {
		t.Errorf("WordRight col = %d, want 4", in.Col())
	}
	in.WordRight()
	if in.Col() != 8 {
		t.Errorf("WordRight col = %d, want 8", in.Col())
	}
	in.End()
	in.WordLeft()
	if in.Col() != 8 {
		t.Errorf("WordLeft col = %d, want 8", in.Col())
	}
}

func TestKillToEndAndPaste(t *testing.T) {
	in := NewInput()
	for _, r := range "hello world" {
		in.InsertRune(r)
	}
	in.WordLeft()
	in.KillToEnd()
	if in.Value() != "hello " {
		t.Errorf("KillToEnd value = %q, want hello ", in.Value())
	}
	in.PasteKill()
	if in.Value() != "hello world" {
		t.Errorf("PasteKill value = %q, want hello world", in.Value())
	}
}

func TestKillToStart(t *testing.T) {
	in := NewInput()
	for _, r := range "hello world" {
		in.InsertRune(r)
	}
	in.WordLeft()
	in.KillToStart()
	if in.Value() != "world" {
		t.Errorf("KillToStart value = %q, want world", in.Value())
	}
	if in.Col() != 0 {
		t.Errorf("col = %d, want 0", in.Col())
	}
}

func TestDeleteWord(t *testing.T) {
	in := NewInput()
	for _, r := range "hello world" {
		in.InsertRune(r)
	}
	in.DeleteWord()
	if in.Value() != "hello " {
		t.Errorf("DeleteWord at EOL value = %q, want hello ", in.Value())
	}
	in.Backspace()
	if in.Value() != "hello" {
		t.Errorf("backspace value = %q, want hello", in.Value())
	}
}

func TestDeleteLine(t *testing.T) {
	in := NewInput()
	for _, r := range "aaa" {
		in.InsertRune(r)
	}
	in.Newline()
	for _, r := range "bbb" {
		in.InsertRune(r)
	}
	in.DeleteLine()
	if in.Value() != "aaa" {
		t.Errorf("DeleteLine value = %q, want aaa", in.Value())
	}
}

func TestDeleteLineOnlyLine(t *testing.T) {
	in := NewInput()
	for _, r := range "abc" {
		in.InsertRune(r)
	}
	in.DeleteLine()
	if in.Value() != "" || len(in.Lines()) != 1 {
		t.Errorf("DeleteLine on single line: value=%q lines=%d, want empty single line", in.Value(), len(in.Lines()))
	}
}

func TestMoveLineUpDown(t *testing.T) {
	in := NewInput()
	for _, r := range "aaa" {
		in.InsertRune(r)
	}
	in.Newline()
	for _, r := range "bbb" {
		in.InsertRune(r)
	}
	in.MoveLineUp()
	if in.Value() != "bbb\naaa" {
		t.Errorf("MoveLineUp value = %q, want bbb\\naaa", in.Value())
	}
	in.MoveLineDown()
	if in.Value() != "aaa\nbbb" {
		t.Errorf("MoveLineDown value = %q, want aaa\\nbbb", in.Value())
	}
}

func TestUndoRedo(t *testing.T) {
	in := NewInput()
	for _, r := range "abc" {
		in.InsertRune(r)
	}
	in.Undo()
	if in.Value() != "ab" {
		t.Errorf("after one undo value = %q, want ab", in.Value())
	}
	in.Undo()
	in.Undo()
	if in.Value() != "" {
		t.Errorf("after three undos value = %q, want empty", in.Value())
	}
	in.Redo()
	if in.Value() != "a" {
		t.Errorf("after one redo value = %q, want a", in.Value())
	}
	in.Redo()
	in.Redo()
	if in.Value() != "abc" {
		t.Errorf("after full redo value = %q, want abc", in.Value())
	}
}

func TestScrollKeepsCursorVisible(t *testing.T) {
	in := NewInput()
	in.SetRows(3, 6)
	for i := 0; i < 9; i++ {
		in.Newline()
	}
	if in.Scroll() != 4 {
		t.Errorf("scroll = %d, want 4", in.Scroll())
	}
	for i := 0; i < 5; i++ {
		in.MoveUp()
	}
	if in.Row() != 4 || in.Scroll() != 4 {
		t.Errorf("cursor=%d scroll=%d, want row 4 scroll 4", in.Row(), in.Scroll())
	}
	in.MoveUp()
	if in.Row() != 3 || in.Scroll() != 3 {
		t.Errorf("cursor=%d scroll=%d, want row 3 scroll 3", in.Row(), in.Scroll())
	}
}

func TestValueJoin(t *testing.T) {
	in := NewInput()
	in.lines = []string{"a", "b", "c"}
	if in.Value() != "a\nb\nc" {
		t.Errorf("value = %q, want a\\nb\\nc", in.Value())
	}
}

func TestClear(t *testing.T) {
	in := NewInput()
	for _, r := range "abc" {
		in.InsertRune(r)
	}
	in.Clear()
	if in.Value() != "" || len(in.Lines()) != 1 {
		t.Errorf("after Clear: value=%q lines=%d", in.Value(), len(in.Lines()))
	}
}

func TestSubmitTriggersCallback(t *testing.T) {
	in := NewInput()
	var got string
	in.OnSubmit = func(m string) { got = m }
	for _, r := range "fix it" {
		in.InsertRune(r)
	}
	in.Submit()
	if got != "fix it" {
		t.Errorf("onSubmit got %q, want fix it", got)
	}
	if in.Value() != "" {
		t.Errorf("input should clear after submit, value = %q", in.Value())
	}
}

func TestSubmitEmptyIgnored(t *testing.T) {
	in := NewInput()
	called := false
	in.OnSubmit = func(m string) { called = true }
	in.Submit()
	if called {
		t.Error("submit should not fire for empty input")
	}
}

func TestLineWidth(t *testing.T) {
	if lineWidth("héllo") != 5 {
		t.Errorf("lineWidth(héllo) = %d, want 5", lineWidth("héllo"))
	}
}

func TestCopyLines(t *testing.T) {
	src := []string{"a", "b"}
	dst := copyLines(src)
	dst[0] = "z"
	if src[0] != "a" {
		t.Error("copyLines must deep-copy the slice")
	}
}

func TestVisibleRowsManual(t *testing.T) {
	in := NewInput()
	in.lines = strings.Split("a\nb\nc\nd\ne\nf\ng", "\n")
	if got := in.VisibleRows(); got != 6 {
		t.Errorf("VisibleRows = %d, want 6 (capped)", got)
	}
}

func TestInsertChipSerializesToSlash(t *testing.T) {
	in := NewInput()
	in.InsertChip("go-development")
	if got := in.Value(); got != "/go-development" {
		t.Errorf("Value = %q, want /go-development", got)
	}
	if got := in.Col(); got != 1 {
		t.Errorf("col = %d, want 1 (chip is one unit)", got)
	}
}

func TestChipIsAtomicOnBackspace(t *testing.T) {
	in := NewInput()
	in.InsertChip("go-development")
	in.InsertRune('x')
	in.Backspace()
	if got := in.Value(); got != "/go-development" {
		t.Fatalf("value = %q, want /go-development", got)
	}
	in.Backspace()
	if got := in.Value(); got != "" {
		t.Errorf("value = %q, want empty (chip deleted atomically)", got)
	}
}

func TestChipAtomicMovement(t *testing.T) {
	in := NewInput()
	in.InsertChip("websearch")
	in.InsertRune('a')
	in.MoveLeft()
	if in.Col() != 1 {
		t.Errorf("col = %d, want 1 (cursor now after the chip)", in.Col())
	}
	in.MoveLeft()
	if in.Col() != 0 {
		t.Errorf("col = %d, want 0 (crossed the whole chip in one step)", in.Col())
	}
	in.MoveRight()
	if in.Col() != 1 {
		t.Errorf("col = %d, want 1 (re-entered after the chip)", in.Col())
	}
	in.MoveRight()
	if in.Col() != 2 {
		t.Errorf("col = %d, want 2", in.Col())
	}
}

func TestReplaceSkillReplacesSlashQuery(t *testing.T) {
	in := NewInput()
	for _, r := range []rune("/go") {
		in.InsertRune(r)
	}
	active, query := in.SlashQuery()
	if !active || query != "go" {
		t.Fatalf("SlashQuery = (%v, %q), want (true, go)", active, query)
	}
	in.ReplaceSkill("go-development")
	if got := in.Value(); got != "/go-development " {
		t.Errorf("Value = %q, want '/go-development '", got)
	}
	if in.Col() != 2 {
		t.Errorf("col = %d, want 2 (after chip + auto space)", in.Col())
	}
}

func TestSlashQueryInactiveWithoutSlash(t *testing.T) {
	in := NewInput()
	in.InsertRune('x')
	if active, _ := in.SlashQuery(); active {
		t.Error("SlashQuery should be inactive without a leading slash")
	}
}

func TestSlashQueryMidSentence(t *testing.T) {
	in := NewInput()
	for _, r := range []rune("do this /go") {
		in.InsertRune(r)
	}
	active, query := in.SlashQuery()
	if !active || query != "go" {
		t.Fatalf("SlashQuery = (%v,%q), want (true, go)", active, query)
	}
}

func TestSlashQueryIgnoresNonTokenSlashes(t *testing.T) {
	in := NewInput()
	for _, r := range []rune("a/b and https://x /go") {
		in.InsertRune(r)
	}
	active, query := in.SlashQuery()
	if !active || query != "go" {
		t.Fatalf("SlashQuery = (%v,%q), want (true, go)", active, query)
	}
}

func TestSlashQueryEndsAtSpace(t *testing.T) {
	in := NewInput()
	for _, r := range []rune("do /go and more") {
		in.InsertRune(r)
	}
	active, query := in.SlashQuery()
	if !active || query != "go" {
		t.Fatalf("SlashQuery = (%v,%q), want (true, go)", active, query)
	}
}

func TestReplaceSkillMidSentence(t *testing.T) {
	in := NewInput()
	for _, r := range []rune("do this /go") {
		in.InsertRune(r)
	}
	in.ReplaceSkill("go-development")
	if got := in.Value(); got != "do this /go-development " {
		t.Errorf("Value = %q, want 'do this /go-development '", got)
	}
	if in.Col() != len([]rune("do this "))+2 {
		t.Errorf("col = %d, want %d (after chip + auto space)", in.Col(), len([]rune("do this "))+2)
	}
}

func TestReplaceSkillMidSentenceDoesNotDoubleSpace(t *testing.T) {
	in := NewInput()
	for _, r := range []rune("do /go and more") {
		in.InsertRune(r)
	}
	in.ReplaceSkill("go-development")
	if got := in.Value(); got != "do /go-development and more" {
		t.Errorf("Value = %q, want 'do /go-development and more'", got)
	}
	if in.Col() != 4 {
		t.Errorf("col = %d, want 4 (after chip, before existing space)", in.Col())
	}
}

func TestReplaceMentionMidSentenceDoesNotDoubleSpace(t *testing.T) {
	in := NewInput()
	for _, r := range []rune("ask @dev to help") {
		in.InsertRune(r)
	}
	in.ReplaceMention("developer")
	if got := in.Value(); got != "ask @developer to help" {
		t.Errorf("Value = %q, want 'ask @developer to help'", got)
	}
	if in.Col() != 5 {
		t.Errorf("col = %d, want 5 (after chip, before existing space)", in.Col())
	}
}

func TestSkillChipVisualWidthMatchesDraw(t *testing.T) {
	in := NewInput()
	in.InsertChip("go-development")
	got := in.visualLineWidth("/go-development")
	want := len("/go-development")
	if got != want {
		t.Errorf("visualLineWidth = %d, want %d (drawn width)", got, want)
	}
}

func TestMentionChipVisualWidthMatchesDraw(t *testing.T) {
	in := NewInput()
	in.InsertMention("developer")
	got := in.visualLineWidth("@developer")
	want := len("@developer")
	if got != want {
		t.Errorf("visualLineWidth = %d, want %d (drawn width)", got, want)
	}
}

func TestChipWithMixedText(t *testing.T) {
	in := NewInput()
	in.InsertRune('a')
	in.InsertChip("plan")
	in.InsertRune('b')
	if got := in.Value(); got != "a/planb" {
		t.Errorf("Value = %q, want a/planb", got)
	}
}

func TestInsertMentionSerializesToAt(t *testing.T) {
	in := NewInput()
	in.InsertMention("developer")
	if got := in.Value(); got != "@developer" {
		t.Errorf("Value = %q, want @developer", got)
	}
}

func TestMentionIsAtomicOnBackspace(t *testing.T) {
	in := NewInput()
	in.InsertMention("developer")
	in.InsertRune('x')
	in.Backspace()
	if got := in.Value(); got != "@developer" {
		t.Errorf("Value = %q, want @developer after backspace", got)
	}
	in.Backspace()
	if got := in.Value(); got != "" {
		t.Errorf("Value = %q, want empty after deleting mention chip", got)
	}
}

func TestReplaceMentionReplacesAtQuery(t *testing.T) {
	in := NewInput()
	for _, r := range []rune("@dev") {
		in.InsertRune(r)
	}
	if v := in.Value(); v != "@dev" {
		t.Fatalf("setup: Value = %q, want @dev", v)
	}
	in.ReplaceMention("developer")
	if got := in.Value(); got != "@developer " {
		t.Errorf("Value = %q, want '@developer '", got)
	}
	if in.Col() != 2 {
		t.Errorf("col = %d, want 2 (after chip + auto space)", in.Col())
	}
}

func TestReplaceMentionMidSentence(t *testing.T) {
	in := NewInput()
	for _, r := range []rune("ask @dev to help") {
		in.InsertRune(r)
	}
	if v := in.Value(); v != "ask @dev to help" {
		t.Fatalf("setup: Value = %q, want 'ask @dev to help'", v)
	}
	in.ReplaceMention("developer")
	if got := in.Value(); got != "ask @developer to help" {
		t.Errorf("Value = %q, want 'ask @developer to help'", got)
	}
}

func TestMentionWithMixedText(t *testing.T) {
	in := NewInput()
	in.InsertRune('a')
	in.InsertMention("dev")
	in.InsertRune('b')
	if got := in.Value(); got != "a@devb" {
		t.Errorf("Value = %q, want a@devb", got)
	}
}

func TestInsertChipAndMention(t *testing.T) {
	in := NewInput()
	in.InsertChip("plan")
	in.InsertMention("explorer")
	if got := in.Value(); got != "/plan@explorer" {
		t.Errorf("Value = %q, want /plan@explorer", got)
	}
}

func TestInsertLongText(t *testing.T) {
	in := NewInput()
	long := strings.Repeat("a", 150)
	in.InsertLongText(long)
	got := in.Value()
	if got != long {
		t.Errorf("Value should expand to full text, got len=%d, want len=%d", len(got), len(long))
	}
}

func TestInsertLongTextShort(t *testing.T) {
	in := NewInput()
	short := "hello world"
	in.InsertLongText(short)
	got := in.Value()
	if got != short {
		t.Errorf("Value = %q, want %q", got, short)
	}
}

func TestSerializeChipsExpandsLongTextAfterSkillChip(t *testing.T) {
	in := NewInput()
	in.InsertChip("plan")
	long := strings.Repeat("b", 150)
	in.InsertLongText(long)
	got := in.Value()
	expected := "/plan" + long
	if got != expected {
		t.Errorf("Value len=%d, want len=%d", len(got), len(expected))
	}
}

func inputWRows(in *Input, width int) []string {
	var rows []string
	for _, s := range in.wrapLines(width) {
		rs := []rune(in.lines[s.lineIdx])
		rows = append(rows, string(rs[s.start:s.end]))
	}
	return rows
}

func TestInputWrapBreaksAtSpaces(t *testing.T) {
	in := NewInput()
	in.lines = []string{"hello tasarım"}
	got := inputWRows(in, 10)
	want := []string{"hello ", "tasarım"}
	if len(got) != len(want) {
		t.Fatalf("rows = %q, want %q", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("row %d = %q, want %q (all %q)", i, got[i], want[i], got)
		}
	}
}

func TestInputWrapHardBreaksOverlongWord(t *testing.T) {
	in := NewInput()
	in.lines = []string{"abcdefghijkl"}
	got := inputWRows(in, 5)
	if strings.Join(got, "") != "abcdefghijkl" {
		t.Fatalf("hard wrap lost characters: %q", got)
	}
	for _, r := range got {
		if in.visualLineWidth(r) > 5 {
			t.Fatalf("row %q exceeds width 5", r)
		}
	}
}

func TestInputVisualRowOfFollowsWordWrap(t *testing.T) {
	in := NewInput()
	in.SetInnerWidth(10)
	in.lines = []string{"hello tasarım world"}
	cases := []struct{ col, want int }{
		{0, 0}, {5, 0}, {6, 1}, {7, 1}, {14, 2}, {19, 2},
	}
	for _, c := range cases {
		if got := in.visualRowOf(0, c.col); got != c.want {
			t.Errorf("visualRowOf(0,%d) = %d, want %d", c.col, got, c.want)
		}
	}
}

func TestInputSetValue(t *testing.T) {
	in := NewInput()
	in.InsertRune('x')
	in.SetValue("line1\nline2")
	if in.Value() != "line1\nline2" {
		t.Fatalf("Value = %q, want multiline", in.Value())
	}
	if in.Row() != 1 || in.Col() != 5 {
		t.Fatalf("cursor = %d,%d, want 1,5", in.Row(), in.Col())
	}
	in.SetValue("")
	if in.Value() != "" || in.Row() != 0 || in.Col() != 0 {
		t.Fatalf("after empty SetValue: value=%q row=%d col=%d", in.Value(), in.Row(), in.Col())
	}
}

func TestInputMoveUpDownFollowsWordWrap(t *testing.T) {
	in := NewInput()
	in.SetInnerWidth(10)
	for _, r := range "hello tasarım world" {
		in.InsertRune(r)
	}
	if in.Col() != 19 {
		t.Fatalf("setup col = %d, want 19", in.Col())
	}
	in.MoveUp()
	if in.Row() != 0 || in.Col() != 11 {
		t.Fatalf("after MoveUp row,col = %d,%d, want 0,11", in.Row(), in.Col())
	}
	in.MoveDown()
	if in.Row() != 0 || in.Col() != 19 {
		t.Fatalf("after MoveDown row,col = %d,%d, want 0,19", in.Row(), in.Col())
	}
}
