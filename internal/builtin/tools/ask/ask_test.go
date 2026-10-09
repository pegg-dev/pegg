package ask

import (
	"strings"
	"testing"
)

func TestParseAskArgsDefaultsIDAndNormalizesType(t *testing.T) {
	qs, err := parseAskArgs(`{"questions":[{"question":"Pick one","type":"SELECT","options":["a","b"]},{"question":"Name?","type":"text"}]}`)
	if err != nil {
		t.Fatal(err)
	}
	if len(qs) != 2 {
		t.Fatalf("got %d questions, want 2", len(qs))
	}
	if qs[0].ID != "q1" || qs[0].Type != "select" {
		t.Errorf("q1 = %+v, want id=q1 type=select", qs[0])
	}
	if qs[1].ID != "q2" || qs[1].Type != "text" {
		t.Errorf("q2 = %+v, want id=q2 type=text", qs[1])
	}
}

func TestParseAskArgsErrorsIncludeExpectedJSON(t *testing.T) {
	tests := []struct {
		name string
		args string
		want string
	}{
		{"invalid json", `not json`, "invalid arguments"},
		{"empty", `{"questions":[]}`, "must not be empty"},
		{"missing question", `{"questions":[{"type":"text"}]}`, "question is required"},
		{"select without options", `{"questions":[{"question":"x","type":"select"}]}`, "no options"},
		{"missing type", `{"questions":[{"question":"x"}]}`, "type must be"},
		{"unknown type", `{"questions":[{"question":"x","type":"boolean"}]}`, "type must be"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseAskArgs(tt.args)
			if err == nil {
				t.Fatal("expected error")
			}
			msg := err.Error()
			if !strings.Contains(msg, tt.want) {
				t.Errorf("error %q does not contain %q", msg, tt.want)
			}
			if !strings.Contains(msg, "Expected JSON") {
				t.Errorf("error %q must include the expected JSON shape", msg)
			}
		})
	}
}
