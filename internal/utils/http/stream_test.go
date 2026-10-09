package http

import (
	"strings"
	"testing"
	"testing/iotest"
)

func TestNewLineDecoder(t *testing.T) {
	input := "hello\nworld\n"
	decoder := NewLineDecoder(strings.NewReader(input))

	line1, err := decoder.Decode()
	if err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	if string(line1) != "hello" {
		t.Errorf("Decode() = %q, want %q", string(line1), "hello")
	}

	line2, err := decoder.Decode()
	if err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	if string(line2) != "world" {
		t.Errorf("Decode() = %q, want %q", string(line2), "world")
	}
}

func TestNewLineDecoder_NoTrailingNewline(t *testing.T) {
	input := "hello\nworld"
	decoder := NewLineDecoder(strings.NewReader(input))

	line1, err := decoder.Decode()
	if err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	if string(line1) != "hello" {
		t.Errorf("Decode() = %q, want %q", string(line1), "hello")
	}

	line2, err := decoder.Decode()
	if err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	if string(line2) != "world" {
		t.Errorf("Decode() = %q, want %q", string(line2), "world")
	}
}

func TestNewLineDecoder_EmptyInput(t *testing.T) {
	decoder := NewLineDecoder(strings.NewReader(""))

	_, err := decoder.Decode()
	if err == nil {
		t.Error("Decode() should return error for empty input")
	}
}

func TestNewLineDecoder_SingleLine(t *testing.T) {
	input := "single line"
	decoder := NewLineDecoder(strings.NewReader(input))

	line, err := decoder.Decode()
	if err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	if string(line) != "single line" {
		t.Errorf("Decode() = %q, want %q", string(line), "single line")
	}
}

func TestNewLineDecoder_TrimsWhitespace(t *testing.T) {
	input := "  hello  \n  world  \n"
	decoder := NewLineDecoder(strings.NewReader(input))

	line1, err := decoder.Decode()
	if err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	if string(line1) != "hello" {
		t.Errorf("Decode() = %q, want %q", string(line1), "hello")
	}

	line2, err := decoder.Decode()
	if err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	if string(line2) != "world" {
		t.Errorf("Decode() = %q, want %q", string(line2), "world")
	}
}

func TestNewLineDecoder_MultipleReads(t *testing.T) {
	input := "line1\nline2\nline3"
	decoder := NewLineDecoder(strings.NewReader(input))

	var lines []string
	for {
		line, err := decoder.Decode()
		if err != nil {
			break
		}
		lines = append(lines, string(line))
	}

	if len(lines) < 1 {
		t.Fatalf("expected at least 1 line, got %d: %v", len(lines), lines)
	}
	if lines[0] != "line1" {
		t.Errorf("lines[0] = %q, want %q", lines[0], "line1")
	}
}

func TestNewLineDecoder_EOFWithDataMultipleLines(t *testing.T) {
	input := "data: {\"a\":1}\ndata: {\"b\":2}\n"
	decoder := NewLineDecoder(strings.NewReader(input))

	line1, err := decoder.Decode()
	if err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	if string(line1) != "data: {\"a\":1}" {
		t.Errorf("line1 = %q, want %q", string(line1), "data: {\"a\":1}")
	}

	line2, err := decoder.Decode()
	if err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	if string(line2) != "data: {\"b\":2}" {
		t.Errorf("line2 = %q, want %q", string(line2), "data: {\"b\":2}")
	}
}

func TestNewLineDecoder_EOFWithPartialLine(t *testing.T) {
	input := "data: {\"partial\":true}"
	decoder := NewLineDecoder(strings.NewReader(input))

	line, err := decoder.Decode()
	if err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	if string(line) != "data: {\"partial\":true}" {
		t.Errorf("line = %q, want %q", string(line), "data: {\"partial\":true}")
	}

	_, err = decoder.Decode()
	if err == nil {
		t.Error("Decode() should return error after EOF")
	}
}

func TestNewLineDecoder_EOFReturnedWithLastRead(t *testing.T) {
	input := "data: {\"a\":1}\n\ndata: [DONE]\n\n"
	decoder := NewLineDecoder(iotest.DataErrReader(strings.NewReader(input)))

	var lines []string
	for {
		line, err := decoder.Decode()
		if err != nil {
			break
		}
		if len(line) > 0 {
			lines = append(lines, string(line))
		}
	}

	want := []string{"data: {\"a\":1}", "data: [DONE]"}
	if len(lines) != len(want) {
		t.Fatalf("lines = %q, want %q", lines, want)
	}
	for i := range want {
		if lines[i] != want[i] {
			t.Errorf("lines[%d] = %q, want %q", i, lines[i], want[i])
		}
	}
}

func TestNewLineDecoder_LineTooLong(t *testing.T) {
	input := strings.Repeat("a", maxLineSize+1)
	decoder := NewLineDecoder(strings.NewReader(input))

	_, err := decoder.Decode()
	if err == nil {
		t.Error("Decode() should return error for oversized line")
	}
}

func TestParseSSEvent(t *testing.T) {
	tests := []struct {
		name     string
		line     []byte
		wantType string
		wantData string
	}{
		{
			name:     "data event",
			line:     []byte("data: {\"id\":\"chatcmpl-123\"}"),
			wantType: "data",
			wantData: `{"id":"chatcmpl-123"}`,
		},
		{
			name:     "done event",
			line:     []byte("data: [DONE]"),
			wantType: "done",
			wantData: "",
		},
		{
			name:     "simple text",
			line:     []byte("data: hello world"),
			wantType: "data",
			wantData: "hello world",
		},
		{
			name:     "empty data",
			line:     []byte("data: "),
			wantType: "data",
			wantData: "",
		},
		{
			name:     "with extra spaces",
			line:     []byte("data:   spaced  "),
			wantType: "data",
			wantData: "spaced",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotType, gotData := ParseSSEvent(tt.line)
			if gotType != tt.wantType {
				t.Errorf("type = %q, want %q", gotType, tt.wantType)
			}
			if gotData != tt.wantData {
				t.Errorf("data = %q, want %q", gotData, tt.wantData)
			}
		})
	}
}

func TestParseSSEvent_Invalid(t *testing.T) {
	tests := []struct {
		name string
		line []byte
	}{
		{
			name: "too short",
			line: []byte("dat"),
		},
		{
			name: "no data prefix",
			line: []byte("event: test"),
		},
		{
			name: "empty",
			line: []byte(""),
		},
		{
			name: "only colon",
			line: []byte("data:"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotType, gotData := ParseSSEvent(tt.line)
			if gotType != "" {
				t.Errorf("type = %q, want empty", gotType)
			}
			if gotData != "" {
				t.Errorf("data = %q, want empty", gotData)
			}
		})
	}
}

func TestParseSSEvent_JSON(t *testing.T) {
	jsonData := `{"id":"chatcmpl-123","choices":[{"delta":{"content":"Hello"},"index":0}]}`
	line := []byte("data: " + jsonData)

	gotType, gotData := ParseSSEvent(line)
	if gotType != "data" {
		t.Errorf("type = %q, want %q", gotType, "data")
	}
	if gotData != jsonData {
		t.Errorf("data = %q, want %q", gotData, jsonData)
	}
}
