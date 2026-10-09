package memory

import (
	"regexp"
	"strconv"
	"strings"
)

type SearchHit struct {
	ID    string
	Type  string
	Date  string
	File  string
	Title string
	Text  string
	Score int
}

func sanitize(s string) string {
	s = strings.ReplaceAll(s, "|", "-")
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", " ")
	return strings.TrimSpace(s)
}

func splitN(s, sep string, n int) []string {
	parts := strings.SplitN(s, sep, n)
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	return parts
}

func splitBlocks(content string) []string {
	var blocks []string
	var cur strings.Builder
	started := false
	for _, line := range strings.Split(content, "\n") {
		if strings.HasPrefix(line, "## ") {
			if started && cur.Len() > 0 {
				blocks = append(blocks, strings.TrimSpace(cur.String()))
			}
			cur.Reset()
			cur.WriteString(line)
			started = true
			continue
		}
		if started {
			cur.WriteString("\n")
			cur.WriteString(line)
		}
	}
	if started && cur.Len() > 0 {
		blocks = append(blocks, strings.TrimSpace(cur.String()))
	}
	return blocks
}

func extractBlock(content, heading string) (string, bool) {
	blocks := splitBlocks(content)
	for _, b := range blocks {
		if strings.HasPrefix(b, heading) {
			return b, true
		}
	}
	return "", false
}

var idRe = regexp.MustCompile(`^## (obs-|sum-)[0-9a-f]{6,}`)

func extractID(block string) string {
	if m := idRe.FindString(block); m != "" {
		return strings.TrimPrefix(strings.TrimPrefix(m, "## "), " ")
	}
	return ""
}

func parseBlockHeader(block string) (id, typ, title, date string) {
	header := firstHeading(block)
	if header == "" {
		return extractID(block), "", "", ""
	}
	id = extractID(block)
	rest := strings.TrimPrefix(header, "## ")
	if i := strings.Index(rest, "["); i >= 0 {
		if j := strings.Index(rest[i:], "]"); j > 1 {
			typ = strings.TrimSpace(rest[i+1 : i+j])
			rest = strings.TrimSpace(rest[i+j+1:])
		}
	}
	if i := strings.Index(rest, "—"); i >= 0 {
		title = strings.TrimSpace(rest[:i])
		date = strings.TrimSpace(rest[i+len("—"):])
	} else {
		title = rest
	}
	return id, typ, title, date
}

func xmlEscape(s string) string {
	r := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		`"`, "&quot;",
		"'", "&apos;",
	)
	return r.Replace(s)
}

func parseGateIndices(out string) []int {
	out = stripFences(strings.TrimSpace(out))
	if out == "" || strings.Contains(strings.ToLower(out), "none") {
		return nil
	}
	var idx []int
	for _, part := range strings.FieldsFunc(out, func(r rune) bool {
		return r == ',' || r == ' ' || r == '\t' || r == '\n'
	}) {
		part = strings.TrimSpace(strings.TrimRight(part, ".,;:"))
		n, err := strconv.Atoi(part)
		if err != nil {
			continue
		}
		idx = append(idx, n-1)
	}
	return idx
}

func firstHeading(block string) string {
	for _, line := range strings.Split(block, "\n") {
		if strings.HasPrefix(line, "## ") {
			return strings.TrimPrefix(line, "## ")
		}
	}
	return ""
}

func scoreBlock(lowerQuery string, terms []string, title, body string) int {
	if lowerQuery == "" {
		return 0
	}
	titleLower := strings.ToLower(title)
	bodyLower := strings.ToLower(body)
	if strings.Contains(titleLower, lowerQuery) {
		return 100 + len(terms)
	}
	score := 0
	for _, t := range terms {
		if t == "" {
			continue
		}
		if strings.Contains(titleLower, t) {
			score += 4
		}
		if strings.Contains(bodyLower, t) {
			score += 1
		}
	}
	return score
}

func clampTo(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}
