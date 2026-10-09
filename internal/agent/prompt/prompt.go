package prompt

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	json "github.com/goccy/go-json"
	"github.com/peggco/pegg/internal/core/config"
)

var ErrMissingVar = errors.New("prompt: variable not found")

type Format int

const (
	FormatMarkdown Format = iota
	FormatXML
	FormatJSON
)

func (f Format) String() string {
	switch f {
	case FormatMarkdown:
		return "markdown"
	case FormatXML:
		return "xml"
	case FormatJSON:
		return "json"
	}
	return fmt.Sprintf("format(%d)", int(f))
}

func (f Format) valid() bool {
	return f >= FormatMarkdown && f <= FormatJSON
}

type Vars map[string]any

func (v Vars) Get(path string) (any, bool) {
	if v == nil {
		return nil, false
	}
	var cur any = map[string]any(v)
	for _, key := range strings.Split(path, ".") {
		m, ok := cur.(map[string]any)
		if !ok {
			return nil, false
		}
		cur, ok = m[key]
		if !ok {
			return nil, false
		}
	}
	return cur, true
}

func (v Vars) Has(path string) bool {
	_, ok := v.Get(path)
	return ok
}

func (v Vars) Bool(path string) bool {
	val, ok := v.Get(path)
	if !ok {
		return false
	}
	b, ok := val.(bool)
	return ok && b
}

type Part interface {
	id() string
	renderMarkdown(rc *renderCtx) (string, error)
	renderXML(rc *renderCtx) (string, error)
	renderJSON(rc *renderCtx) (any, error)
}

type Prompt struct {
	parts []Part
	vars  Vars
}

func New() *Prompt {
	return &Prompt{vars: Vars{}}
}

func (p *Prompt) Set(key string, value any) *Prompt {
	p.vars[key] = value
	return p
}

func (p *Prompt) SetVars(v Vars) *Prompt {
	p.vars = make(Vars, len(v))
	for k, val := range v {
		p.vars[k] = val
	}
	return p
}

func (p *Prompt) Vars() Vars {
	v := make(Vars, len(p.vars))
	for k, val := range p.vars {
		v[k] = val
	}
	return v
}

func (p *Prompt) Add(parts ...Part) *Prompt {
	p.parts = append(p.parts, parts...)
	return p
}

func (p *Prompt) Clone() *Prompt {
	q := New()
	q.SetVars(p.vars)
	q.parts = append(q.parts, p.parts...)
	return q
}

func (p *Prompt) IDs() []string {
	var ids []string
	for _, part := range p.parts {
		if id := part.id(); id != "" {
			ids = append(ids, id)
		}
	}
	return ids
}

func (p *Prompt) Select(ids ...string) *Prompt {
	want := make(map[string]bool, len(ids))
	for _, id := range ids {
		want[id] = true
	}
	q := New()
	q.SetVars(p.vars)
	for _, part := range p.parts {
		if want[part.id()] {
			q.parts = append(q.parts, part)
		}
	}
	return q
}

func (p *Prompt) Build(format Format) (string, error) {
	if !format.valid() {
		return "", fmt.Errorf("prompt: unsupported format %v", format)
	}
	rc := &renderCtx{format: format, vars: p.vars}
	if format == FormatJSON {
		nodes, err := jsonNodes(rc, p.parts)
		if err != nil {
			return "", err
		}
		var buf bytes.Buffer
		enc := json.NewEncoder(&buf)
		enc.SetEscapeHTML(false)
		enc.SetIndent("", "  ")
		if err := enc.Encode(nodes); err != nil {
			return "", fmt.Errorf("prompt: encode json: %w", err)
		}
		return strings.TrimRight(buf.String(), "\n"), nil
	}
	sep := "\n\n"
	if format == FormatXML {
		sep = "\n"
	}
	return joinText(rc, p.parts, sep)
}

func (p *Prompt) MustBuild(format Format) string {
	out, err := p.Build(format)
	if err != nil {
		panic(err)
	}
	return out
}

func (p *Prompt) AgentsMd() *Prompt {
	content, err := os.ReadFile("AGENTS.md")
	if err != nil || len(content) == 0 {
		return p
	}
	return p.Add(Heading(1, "Project Instructions")).
		Add(Raw(string(content)))
}

func (p *Prompt) Rules() *Prompt {
	globalDir, _ := config.GetConfigPath("rules")
	projectDir, _ := config.GetProjectConfigPath("rules")

	var allRules []string

	if rules := readRuleDir(globalDir); len(rules) > 0 {
		allRules = append(allRules, rules...)
	}

	if rules := readRuleDir(projectDir); len(rules) > 0 {
		allRules = append(allRules, rules...)
	}

	if len(allRules) == 0 {
		return p
	}

	p = p.Add(Heading(1, "Rules")).
		Add(Paragraph("WARNING: You MUST strictly follow these rules. Violation of these rules is unacceptable."))

	for _, content := range allRules {
		p = p.Add(Raw(content))
	}

	return p
}

func readRuleDir(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}

	var names []string
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		names = append(names, entry.Name())
	}

	sort.Strings(names)

	var rules []string
	for _, name := range names {
		content, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil || len(content) == 0 {
			continue
		}
		rules = append(rules, string(content))
	}

	return rules
}

func Render(text string, vars Vars) (string, error) {
	return interpolate(&renderCtx{vars: vars}, text)
}
