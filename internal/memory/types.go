package memory

import (
	"time"

	"github.com/peggco/pegg/internal/core/config"
)

type MemoryConfig = config.MemoryConfig

const (
	dirName         = "memory"
	indexFile       = "index.md"
	activeCtxFile   = "active_context.md"
	decisionsFile   = "decisions.md"
	lessonsFile     = "lessons.md"
	notesFile       = "notes.md"
	sessionsDir     = "sessions"
	globalLessons   = "lessons.md"
	indexSeparator  = " | "
	indexFields     = 6
	maxPromptChars  = 2000
	maxToolInChars  = 2000
	maxToolOutChars = 4000
	maxCuratedChars = 1500
	maxGlobalLesson = 300
	queueSize       = 512
	obsPrefix       = "obs-"
	sumPrefix       = "sum-"
	globalMarker    = "[global]"
)

var observationTypes = []string{
	"bugfix", "feature", "refactor", "change", "discovery",
	"decision", "security_alert", "security_note", "sensitive",
}

func validObservationType(t string) bool {
	for _, v := range observationTypes {
		if t == v {
			return true
		}
	}
	return false
}

type ToolUse struct {
	Name    string    `json:"name"`
	Args    string    `json:"args,omitempty"`
	Output  string    `json:"output,omitempty"`
	Err     string    `json:"err,omitempty"`
	At      time.Time `json:"at"`
	Session string    `json:"session,omitempty"`
	Agent   string    `json:"agent,omitempty"`
}

type turn struct {
	agentID       string
	agentName     string
	displayName   string
	parentID      string
	sessionID     string
	prompt        string
	lastAssistant string
	toolUses      []ToolUse
	startedAt     time.Time
}

type Observation struct {
	ID        string    `json:"id"`
	Type      string    `json:"type"`
	Title     string    `json:"title"`
	Subtitle  string    `json:"subtitle,omitempty"`
	Facts     []string  `json:"facts,omitempty"`
	Concepts  []string  `json:"concepts,omitempty"`
	Narrative string    `json:"narrative,omitempty"`
	FilesRead []string  `json:"files_read,omitempty"`
	FilesMod  []string  `json:"files_modified,omitempty"`
	Hash      string    `json:"hash"`
	At        time.Time `json:"at"`
	Session   string    `json:"session,omitempty"`
	Agent     string    `json:"agent,omitempty"`
}

type Summary struct {
	ID           string    `json:"id"`
	Request      string    `json:"request,omitempty"`
	Investigated string    `json:"investigated,omitempty"`
	Learned      string    `json:"learned,omitempty"`
	Completed    string    `json:"completed,omitempty"`
	NextSteps    string    `json:"next_steps,omitempty"`
	Notes        string    `json:"notes,omitempty"`
	FilesEdited  []string  `json:"files_edited,omitempty"`
	Hash         string    `json:"hash"`
	At           time.Time `json:"at"`
	Session      string    `json:"session,omitempty"`
	Agent        string    `json:"agent,omitempty"`
}

type IndexEntry struct {
	ID       string
	Date     string
	Type     string
	Title    string
	Keywords string
	File     string
}

func (e IndexEntry) row() string {
	return e.ID + indexSeparator + e.Date + indexSeparator + e.Type +
		indexSeparator + sanitize(e.Title) + indexSeparator + sanitize(e.Keywords) +
		indexSeparator + e.File
}

func parseIndexRow(line string) (IndexEntry, bool) {
	parts := splitN(line, indexSeparator, indexFields)
	if len(parts) < indexFields {
		return IndexEntry{}, false
	}
	return IndexEntry{
		ID:       parts[0],
		Date:     parts[1],
		Type:     parts[2],
		Title:    parts[3],
		Keywords: parts[4],
		File:     parts[5],
	}, true
}

func cfgSkipsTool(c *MemoryConfig, name string) bool {
	if c == nil {
		return false
	}
	for _, t := range c.SkipTools {
		if t == name {
			return true
		}
	}
	return false
}
