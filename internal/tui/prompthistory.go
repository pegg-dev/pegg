package tui

import (
	"encoding/json"
	"os"
	"strings"
	"sync"

	"github.com/vesvai/vesvai/internal/core/config"
)

const (
	promptHistoryFileName = "prompt-history.json"
	promptHistoryLimit    = 200
)

var promptHistoryMu sync.Mutex

func promptHistoryPath() (string, error) {
	return config.GetProjectConfigPath(promptHistoryFileName)
}

func loadPromptHistory() []string {
	path, err := promptHistoryPath()
	if err != nil {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var entries []string
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil
	}
	return capPromptHistory(entries)
}

func savePromptHistory(entries []string) error {
	promptHistoryMu.Lock()
	defer promptHistoryMu.Unlock()

	if err := config.EnsureProjectConfigDir(); err != nil {
		return err
	}
	path, err := promptHistoryPath()
	if err != nil {
		return err
	}
	data, err := json.Marshal(capPromptHistory(entries))
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func appendPromptHistory(entries []string, entry string) []string {
	entry = strings.TrimRight(entry, "\n")
	if strings.TrimSpace(entry) == "" {
		return entries
	}
	if n := len(entries); n > 0 && entries[n-1] == entry {
		return entries
	}
	return capPromptHistory(append(entries, entry))
}

func mergePromptHistory(stored, recent []string) []string {
	inRecent := make(map[string]bool, len(recent))
	for _, e := range recent {
		inRecent[e] = true
	}
	out := make([]string, 0, len(stored)+len(recent))
	for _, e := range stored {
		if !inRecent[e] {
			out = append(out, e)
		}
	}
	return capPromptHistory(append(out, recent...))
}

func (a *App) recordPrompt(input string) {
	a.chatMu.Lock()
	a.promptHistory = appendPromptHistory(a.promptHistory, input)
	entries := make([]string, len(a.promptHistory))
	copy(entries, a.promptHistory)
	a.chatMu.Unlock()

	go func() { _ = savePromptHistory(entries) }()
}

func capPromptHistory(entries []string) []string {
	if len(entries) > promptHistoryLimit {
		return entries[len(entries)-promptHistoryLimit:]
	}
	return entries
}
