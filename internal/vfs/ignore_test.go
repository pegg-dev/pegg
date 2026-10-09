package vfs

import (
	"os"
	"path/filepath"
	"testing"
)

func writeIgnoreFiles(t *testing.T, root, gitignore, peggignore string) {
	t.Helper()
	if gitignore != "" {
		if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte(gitignore), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if peggignore != "" {
		if err := os.WriteFile(filepath.Join(root, ".peggignore"), []byte(peggignore), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func newTestIgnorer(t *testing.T, gitignore, peggignore string) *Ignorer {
	t.Helper()
	root := t.TempDir()
	writeIgnoreFiles(t, root, gitignore, peggignore)
	return newIgnorer(root)
}

func TestIgnoreBasic(t *testing.T) {
	ig := newTestIgnorer(t, `
*.log
/build/
secret.txt
`, "")

	cases := []struct {
		rel   string
		isDir bool
		want  bool
	}{
		{"app.log", false, true},
		{"a/b/deep.log", false, true},
		{"app.txt", false, false},
		{"build", true, true},
		{"build/main.go", false, true},
		{"a/build", true, false},
		{"a/build/main.go", false, false},
		{"buildx", false, false},
		{"secret.txt", false, true},
		{"sub/secret.txt", false, true},
		{"sub/other.txt", false, false},
	}
	for _, c := range cases {
		if got := ig.Ignored(c.rel, c.isDir); got != c.want {
			t.Errorf("Ignored(%q, dir=%v) = %v, want %v", c.rel, c.isDir, got, c.want)
		}
	}
}

func TestIgnoreAnchored(t *testing.T) {
	ig := newTestIgnorer(t, "/build/\nfoo/bar\n", "")
	if !ig.Ignored("build", true) {
		t.Error("anchored /build/ should match root build")
	}
	if ig.Ignored("a/build", true) {
		t.Error("anchored /build/ should not match nested build")
	}
	if !ig.Ignored("foo/bar", false) {
		t.Error("foo/bar (middle slash) should be anchored and match")
	}
	if ig.Ignored("x/foo/bar", false) {
		t.Error("foo/bar should not match at deeper levels")
	}
}

func TestIgnoreWildcards(t *testing.T) {
	ig := newTestIgnorer(t, "*.min.js\nimg/logo@?x.png\n", "")
	if !ig.Ignored("dist/app.min.js", false) {
		t.Error("*.min.js should match anywhere")
	}
	if !ig.Ignored("img/logo@2x.png", false) {
		t.Error("? should match a single character")
	}
	if ig.Ignored("img/logo@22x.png", false) {
		t.Error("? must not match two characters")
	}
}

func TestIgnoreDoubleStar(t *testing.T) {
	ig := newTestIgnorer(t, "**/node_modules/\n", "")
	if !ig.Ignored("node_modules", true) {
		t.Error("**/node_modules/ should match root node_modules")
	}
	if !ig.Ignored("a/b/node_modules", true) {
		t.Error("**/node_modules/ should match nested node_modules")
	}
	if !ig.Ignored("a/b/node_modules/pkg/index.js", false) {
		t.Error("**/node_modules/ should ignore descendants")
	}
}

func TestIgnoreNegation(t *testing.T) {
	ig := newTestIgnorer(t, "*.log\n!important.log\n", "")
	if !ig.Ignored("app.log", false) {
		t.Error("*.log should ignore app.log")
	}
	if ig.Ignored("important.log", false) {
		t.Error("!important.log should re-include")
	}
}

func TestIgnoreDirOnlyDoesNotMatchFile(t *testing.T) {
	ig := newTestIgnorer(t, "vendor/\n", "")
	if !ig.Ignored("vendor", true) {
		t.Error("vendor/ should ignore the vendor directory")
	}
	if !ig.Ignored("a/vendor/pkg.go", false) {
		t.Error("vendor/ should ignore files under vendor")
	}
	if ig.Ignored("vendor", false) {
		t.Error("vendor/ should not ignore a file named vendor")
	}
}

func TestIgnoreGitDirAlwaysIgnored(t *testing.T) {
	ig := newTestIgnorer(t, "", "")
	if !ig.Ignored(".git", true) {
		t.Error(".git should always be ignored")
	}
	if !ig.Ignored(".git/config", false) {
		t.Error(".git contents should always be ignored")
	}
}

func TestIgnorePeggUnion(t *testing.T) {
	ig := newTestIgnorer(t, "*.log", "config.template.json\n")
	if !ig.Ignored("config.template.json", false) {
		t.Error(".peggignore should hide files")
	}
	if !ig.Ignored("app.log", false) {
		t.Error(".gitignore should still apply")
	}
}

func TestIgnoreNestedPrecedence(t *testing.T) {
	root := t.TempDir()
	writeIgnoreFiles(t, root, "*.txt\n", "")
	if err := os.MkdirAll(filepath.Join(root, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "sub", ".gitignore"), []byte("!keep.txt\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	ig := newIgnorer(root)
	if !ig.Ignored("sub/other.txt", false) {
		t.Error("root *.txt should ignore sub/other.txt")
	}
	if ig.Ignored("sub/keep.txt", false) {
		t.Error("nested negation should win over root pattern")
	}
}

func TestIgnoreEmptyAndComments(t *testing.T) {
	ig := newTestIgnorer(t, "\n# comment\n\n", "")
	if ig.Ignored("anything.txt", false) {
		t.Error("empty and comment lines must not produce rules")
	}
}

func TestParsePatternEdgeCases(t *testing.T) {
	if _, ok := parsePattern(""); ok {
		t.Error("empty line must be skipped")
	}
	if _, ok := parsePattern("#comment"); ok {
		t.Error("comment must be skipped")
	}
	if _, ok := parsePattern("/"); ok {
		t.Error("bare slash must be skipped")
	}
	p, ok := parsePattern("!keep.txt")
	if !ok || !p.negated {
		t.Error("negation flag expected")
	}
	p, ok = parsePattern("dist/")
	if !ok || !p.dirOnly {
		t.Error("dirOnly flag expected")
	}
	p, ok = parsePattern("/anchored")
	if !ok || !p.anchored {
		t.Error("anchored flag expected")
	}
	p, ok = parsePattern("mid/slash")
	if !ok || !p.anchored {
		t.Error("middle slash should anchor the pattern")
	}
}
