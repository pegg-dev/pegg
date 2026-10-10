package schedule

import (
	"testing"
	"time"
)

func TestValidateCron(t *testing.T) {
	valid := []string{
		"*/5 * * * *",
		"0 9 * * MON-FRI",
		"0 0 1 * *",
		"@daily",
		"0 */6 * * *",
	}
	for _, spec := range valid {
		if err := Validate(spec); err != nil {
			t.Errorf("Validate(%q) unexpected error: %v", spec, err)
		}
	}

	invalid := []string{"", "not a cron", "* * *", "60 * * * *"}
	for _, spec := range invalid {
		if err := Validate(spec); err == nil {
			t.Errorf("Validate(%q) expected error", spec)
		}
	}
}

func TestNextRun(t *testing.T) {
	from := time.Now()
	next, err := NextRun("0 9 * * *", from)
	if err != nil {
		t.Fatalf("NextRun: %v", err)
	}
	if next == nil {
		t.Fatal("expected a next run")
	}
	if !next.After(from) {
		t.Fatalf("NextRun = %v, want after %v", next, from)
	}
	if next.Sub(from) > 25*time.Hour {
		t.Fatalf("NextRun = %v is too far from %v", next, from)
	}
}
