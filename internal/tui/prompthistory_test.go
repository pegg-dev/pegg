package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/vesvai/vesvai/internal/core/config"
)

func TestAppendPromptHistorySkipsBlanksAndRepeats(t *testing.T) {
	var got []string
	got = appendPromptHistory(got, "first")
	got = appendPromptHistory(got, "first")
	got = appendPromptHistory(got, "   ")
	got = appendPromptHistory(got, "second\n")
	want := []string{"first", "second"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("history = %v, want %v", got, want)
	}
}

func TestAppendPromptHistoryCaps(t *testing.T) {
	var got []string
	for i := range promptHistoryLimit + 10 {
		got = appendPromptHistory(got, fmt.Sprintf("prompt %d", i))
	}
	if len(got) != promptHistoryLimit {
		t.Errorf("len = %d, want %d", len(got), promptHistoryLimit)
	}
}

func TestMergePromptHistoryMovesSessionPromptsLast(t *testing.T) {
	stored := []string{"old", "shared", "newer"}
	recent := []string{"shared", "fresh"}
	got := mergePromptHistory(stored, recent)
	want := []string{"old", "newer", "shared", "fresh"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("merged = %v, want %v", got, want)
	}
}

func TestSaveAndLoadPromptHistory(t *testing.T) {
	t.Chdir(t.TempDir())

	if err := savePromptHistory([]string{"one", "two"}); err != nil {
		t.Fatalf("save: %v", err)
	}
	path, err := config.GetProjectConfigPath(promptHistoryFileName)
	if err != nil {
		t.Fatalf("path: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("history file missing at %s: %v", filepath.Base(path), err)
	}
	got := loadPromptHistory()
	want := []string{"one", "two"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("loaded = %v, want %v", got, want)
	}
}

func TestLoadPromptHistoryMissingFile(t *testing.T) {
	t.Chdir(t.TempDir())
	if got := loadPromptHistory(); got != nil {
		t.Errorf("loaded = %v, want nil", got)
	}
}

func TestRecordPromptPersistsAcrossSessions(t *testing.T) {
	t.Chdir(t.TempDir())

	a := &App{}
	a.recordPrompt("fix the build")
	a.recordPrompt("run the tests")

	if want := []string{"fix the build", "run the tests"}; !reflect.DeepEqual(a.promptHistory, want) {
		t.Fatalf("in-memory history = %v, want %v", a.promptHistory, want)
	}

	deadline := time.Now().Add(2 * time.Second)
	for {
		got := loadPromptHistory()
		if reflect.DeepEqual(got, []string{"fix the build", "run the tests"}) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("persisted history = %v, want both prompts", got)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
