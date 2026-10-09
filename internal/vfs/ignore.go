package vfs

import (
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/peggco/pegg/internal/core/config"
)

const ignoreFileGit = ".gitignore"

const ignoreFilePegg = ".peggignore"

const (
	peggDir   = ".pegg"
	PlansDir  = peggDir + "/plans"
	RulesDir  = peggDir + "/rules"
	SkillsDir = peggDir + "/skills"
)

func isGlobalWritable(abs string) bool {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return false
	}
	if resolved, err := filepath.EvalSymlinks(home); err == nil {
		home = resolved
	}
	base := filepath.Join(home, config.GlobalConfigDirName)
	clean := filepath.Clean(abs)
	if clean == filepath.Join(base, config.GlobalConfigFileName) {
		return true
	}
	skills := filepath.Join(base, "skills")
	return clean == skills || strings.HasPrefix(clean, skills+string(filepath.Separator))
}

func isPeggPath(rel string) bool {
	return rel == peggDir || strings.HasPrefix(rel, peggDir+"/")
}

func isPlansPath(rel string) bool {
	return rel == PlansDir || strings.HasPrefix(rel, PlansDir+"/")
}

func isRulesPath(rel string) bool {
	return rel == RulesDir || strings.HasPrefix(rel, RulesDir+"/")
}

func isSkillsPath(rel string) bool {
	return rel == SkillsDir || strings.HasPrefix(rel, SkillsDir+"/")
}

type pattern struct {
	negated  bool
	dirOnly  bool
	anchored bool
	segs     []string
}

type Ignorer struct {
	root  string
	mu    sync.RWMutex
	cache map[string][]pattern
}

func newIgnorer(root string) *Ignorer {
	return &Ignorer{
		root:  root,
		cache: make(map[string][]pattern),
	}
}

func (ig *Ignorer) Ignored(rel string, isDir bool) bool {
	rel = strings.Trim(rel, "/")
	if rel == "" {
		return false
	}
	if rel == ".git" || strings.HasPrefix(rel, ".git/") {
		return true
	}
	if isPeggPath(rel) {
		return false
	}

	matched := false
	ignored := false
	for _, dir := range ancestorDirs(rel, isDir) {
		for _, p := range ig.patternsFor(dir) {
			if !matches(p, strings.TrimPrefix(rel, dirPrefix(dir)), isDir) {
				continue
			}
			matched = true
			ignored = !p.negated
		}
	}
	return matched && ignored
}

func dirPrefix(dir string) string {
	if dir == "" {
		return ""
	}
	return dir + "/"
}

func ancestorDirs(rel string, isDir bool) []string {
	segs := strings.Split(rel, "/")
	limit := len(segs)
	if isDir {
		limit--
	}
	if limit < 0 {
		limit = 0
	}
	dirs := make([]string, 0, limit+1)
	dirs = append(dirs, "")
	for i := 0; i < limit; i++ {
		dirs = append(dirs, strings.Join(segs[:i+1], "/"))
	}
	return dirs
}

func (ig *Ignorer) patternsFor(dir string) []pattern {
	ig.mu.RLock()
	pats, ok := ig.cache[dir]
	ig.mu.RUnlock()
	if ok {
		return pats
	}

	pats = ig.load(dir)

	ig.mu.Lock()
	ig.cache[dir] = pats
	ig.mu.Unlock()
	return pats
}

func (ig *Ignorer) load(dir string) []pattern {
	phys := ig.root
	if dir != "" {
		phys = filepath.Join(phys, filepath.FromSlash(dir))
	}
	var pats []pattern
	for _, name := range []string{ignoreFileGit, ignoreFilePegg} {
		data, err := os.ReadFile(filepath.Join(phys, name))
		if err != nil {
			continue
		}
		pats = append(pats, parseIgnore(data)...)
	}
	return pats
}

func parseIgnore(data []byte) []pattern {
	var pats []pattern
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimRight(line, " \t\r")
		if p, ok := parsePattern(line); ok {
			pats = append(pats, p)
		}
	}
	return pats
}

func parsePattern(line string) (pattern, bool) {
	if line == "" || strings.HasPrefix(line, "#") {
		return pattern{}, false
	}
	negated := false
	if strings.HasPrefix(line, "!") {
		negated = true
		line = line[1:]
	}
	if strings.HasPrefix(line, "\\#") || strings.HasPrefix(line, "\\!") {
		line = line[1:]
	}
	dirOnly := strings.HasSuffix(line, "/")
	line = strings.TrimSuffix(line, "/")
	anchored := strings.HasPrefix(line, "/")
	line = strings.TrimPrefix(line, "/")
	if line == "" {
		return pattern{}, false
	}
	if !anchored && strings.Contains(line, "/") {
		anchored = true
	}
	return pattern{
		negated:  negated,
		dirOnly:  dirOnly,
		anchored: anchored,
		segs:     strings.Split(line, "/"),
	}, true
}

func matches(p pattern, rel string, isDir bool) bool {
	segs := strings.Split(rel, "/")
	if !p.dirOnly {
		return matchPath(p, segs)
	}
	limit := len(segs)
	if !isDir {
		limit--
	}
	for i := 1; i <= limit; i++ {
		if matchPath(p, segs[:i]) {
			return true
		}
	}
	return false
}

func matchPath(p pattern, segs []string) bool {
	if p.anchored {
		return matchSegs(p.segs, segs, 0, 0)
	}
	for start := 0; start <= len(segs); start++ {
		if matchSegs(p.segs, segs, 0, start) {
			return true
		}
	}
	return false
}

func matchSegs(pat, segs []string, pi, si int) bool {
	if pi == len(pat) {
		return si == len(segs)
	}
	if pat[pi] == "**" {
		for i := si; i <= len(segs); i++ {
			if matchSegs(pat, segs, pi+1, i) {
				return true
			}
		}
		return false
	}
	if si >= len(segs) || !globMatch(pat[pi], segs[si]) {
		return false
	}
	return matchSegs(pat, segs, pi+1, si+1)
}

func globMatch(pat, s string) bool {
	pi, si, star, mark := 0, 0, -1, 0
	for si < len(s) {
		if pi < len(pat) && (pat[pi] == '?' || pat[pi] == s[si]) {
			pi++
			si++
			continue
		}
		if pi < len(pat) && pat[pi] == '*' {
			star = pi
			mark = si
			pi++
			continue
		}
		if star >= 0 {
			pi = star + 1
			mark++
			si = mark
			continue
		}
		return false
	}
	for pi < len(pat) && pat[pi] == '*' {
		pi++
	}
	return pi == len(pat)
}
