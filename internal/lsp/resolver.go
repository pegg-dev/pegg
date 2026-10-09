package lsp

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/peggco/pegg/internal/core/config"
	"github.com/peggco/pegg/internal/utils/http"
	"github.com/peggco/pegg/internal/utils/tar"
)

const installTimeout = 5 * time.Minute

var defaultLookup = func(name string) (config.LanguageServerConfig, bool) {
	s, ok := Registered()[name]
	return s, ok
}

func isArchive(name string) bool {
	l := strings.ToLower(name)
	return strings.HasSuffix(l, ".zip") ||
		strings.HasSuffix(l, ".tar.gz") ||
		strings.HasSuffix(l, ".tgz") ||
		strings.HasSuffix(l, ".tar")
}

func extractArchive(archivePath, dir, command string) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}

	want := binaryFileName(command)

	writeEntry := func(name string, r io.Reader) (string, error) {
		tmp, err := os.CreateTemp(dir, ".lsp-extract-*")
		if err != nil {
			return "", err
		}
		if _, err := io.Copy(tmp, r); err != nil {
			_ = tmp.Close()
			_ = os.Remove(tmp.Name())
			return "", err
		}
		if err := tmp.Close(); err != nil {
			_ = os.Remove(tmp.Name())
			return "", err
		}
		if err := os.Chmod(tmp.Name(), 0o755); err != nil {
			_ = os.Remove(tmp.Name())
			return "", err
		}
		return tmp.Name(), nil
	}

	candidate := ""
	keep := func(tmp, base string) {
		if base == want {
			if candidate != "" && !strings.Contains(candidate, ".lsp-extract-") && candidate != filepath.Join(dir, want) {
				_ = os.Remove(candidate)
			}
			candidate = filepath.Join(dir, want)
			if err := os.Rename(tmp, candidate); err != nil {
				_ = os.Remove(tmp)
			}
			return
		}
		if candidate == "" {
			dst := filepath.Join(dir, base)
			if err := os.Rename(tmp, dst); err != nil {
				_ = os.Remove(tmp)
				return
			}
			candidate = dst
		}
	}

	switch {
	case strings.HasSuffix(strings.ToLower(archivePath), ".zip"):
		z, err := zip.OpenReader(archivePath)
		if err != nil {
			return "", err
		}
		defer z.Close()
		for _, f := range z.File {
			if f.FileInfo().IsDir() {
				continue
			}
			base := filepath.Base(f.Name)
			if base == "." || base == "/" || strings.HasPrefix(base, ".") {
				continue
			}
			rc, err := f.Open()
			if err != nil {
				continue
			}
			tmp, err := writeEntry(base, rc)
			_ = rc.Close()
			if err != nil {
				continue
			}
			keep(tmp, base)
			if candidate != "" && candidate == filepath.Join(dir, want) {
				break
			}
		}
	case strings.HasSuffix(strings.ToLower(archivePath), ".tar.gz"),
		strings.HasSuffix(strings.ToLower(archivePath), ".tgz"),
		strings.HasSuffix(strings.ToLower(archivePath), ".tar"):
		extracted, err := tar.ExtractGz(archivePath, dir)
		if err != nil {
			return "", err
		}
		return pickArchiveBinary(extracted, dir, want)
	}

	if candidate == "" {
		return "", fmt.Errorf("%w: no executable in %s", ErrNotFound, archivePath)
	}
	return candidate, nil
}

func ResolveBinary(name string, cfg config.LanguageServerConfig) (string, error) {
	command := strings.TrimSpace(cfg.Command)
	if command == "" {
		return "", ErrUnsupported
	}

	cacheDir, err := cachedBinaryDir()
	if err != nil {
		return "", err
	}
	cached := filepath.Join(cacheDir, binaryFileName(command))

	if isExecutableFile(cached) {
		return cached, nil
	}

	if found := extractedFromCache(cached, command); found != "" {
		return found, nil
	}

	if found := findInstalledBinary(command); found != "" {
		return found, nil
	}

	if cfg.Download == "" && cfg.Install == "" {
		if def, ok := defaultLookup(name); ok {
			cfg.Download = def.Download
			cfg.Install = def.Install
		}
	}

	if cfg.Download != "" {
		return downloadBinary(cfg.Download, cached, command)
	}

	if cfg.Install != "" {
		_ = os.Remove(cached + ".zip")
		_ = os.Remove(cached + ".tar.gz")
		_ = os.Remove(cached + ".tgz")
		_ = os.Remove(cached + ".tar")
		installed, err := installBinary(cfg.Install, command, cached)
		if err != nil {
			return "", err
		}
		return installed, nil
	}

	return "", fmt.Errorf("%w: %q not found on PATH and no download/install configured", ErrNotFound, command)
}

func extractedFromCache(cached, command string) string {
	for _, ext := range []string{".zip", ".tar.gz", ".tgz", ".tar"} {
		staged := cached + ext
		if _, err := os.Stat(staged); err != nil {
			continue
		}
		dir := filepath.Dir(cached)
		if out, err := extractArchive(staged, dir, command); err == nil {
			if isExecutableFile(out) {
				_ = os.Remove(staged)
				return out
			}
		}
	}
	return ""
}

func pickArchiveBinary(extracted []string, dir, want string) (string, error) {
	var fallback string
	for _, p := range extracted {
		base := filepath.Base(p)
		if base == "." || base == "/" || strings.HasPrefix(base, ".") {
			continue
		}
		if base == want {
			dst := filepath.Join(dir, want)
			if filepath.Clean(p) != filepath.Clean(dst) {
				if err := os.Rename(p, dst); err != nil {
					return "", err
				}
			}
			if err := os.Chmod(dst, 0o755); err != nil {
				return "", err
			}
			return dst, nil
		}
		if fallback == "" {
			fallback = p
		}
	}
	if fallback != "" {
		if err := os.Chmod(fallback, 0o755); err != nil {
			return "", err
		}
		return fallback, nil
	}
	return "", fmt.Errorf("%w: no executable in %s", ErrNotFound, dir)
}

func installBinary(installCommand, command, cached string) (string, error) {
	if isShellSnippet(installCommand) {
		cmd := exec.Command("/bin/sh", "-c", installCommand)
		if err := cmd.Start(); err != nil {
			return "", fmt.Errorf("lsp: start installer: %w", err)
		}
		if err := cmd.Wait(); err != nil {
			return "", fmt.Errorf("lsp: run installer %q: %w", installCommand, err)
		}
	} else {
		args, err := splitCommand(installCommand)
		if err != nil {
			return "", err
		}
		if len(args) == 0 {
			return "", fmt.Errorf("lsp: empty install command")
		}
		cmd := exec.Command(args[0], args[1:]...)
		if err := cmd.Start(); err != nil {
			return "", fmt.Errorf("lsp: start installer: %w", err)
		}
		if err := cmd.Wait(); err != nil {
			return "", fmt.Errorf("lsp: run installer %q: %w", installCommand, err)
		}
	}

	if staged := extractedFromCache(cached, command); staged != "" {
		return staged, nil
	}
	if isExecutableFile(cached) {
		return cached, nil
	}
	installed := findInstalledBinary(command)
	if installed == "" {
		return "", fmt.Errorf("%w: %q not produced by install command", ErrNotFound, command)
	}

	if err := cacheBinary(installed, cached); err == nil && isExecutableFile(cached) {
		return cached, nil
	}
	return installed, nil
}

func isShellSnippet(s string) bool {
	t := strings.TrimSpace(s)
	return strings.ContainsAny(t, ";&|$`><")
}

func findInstalledBinary(command string) string {
	if found := findExecutable(command); found != "" {
		return found
	}

	patterns := map[string]bool{}
	add := func(d string) {
		if d != "" {
			patterns[filepath.Clean(d)] = true
		}
	}
	home, _ := os.UserHomeDir()
	env := os.Environ()
	for _, kv := range env {
		key, value, ok := strings.Cut(kv, "=")
		if !ok {
			continue
		}
		switch key {
		case "GOBIN":
			add(value)
		case "GOPATH":
			for _, g := range filepath.SplitList(value) {
				add(filepath.Join(g, "bin"))
			}
		}
	}
	for _, d := range []string{
		filepath.Join(home, "go", "bin"),
		filepath.Join(home, ".go", "bin"),
		filepath.Join(home, ".local", "bin"),
		filepath.Join(home, ".cargo", "bin"),
		filepath.Join(home, ".deno", "bin"),
		filepath.Join(home, ".juliaup", "bin"),
		filepath.Join(home, ".ghcup", "bin"),
		filepath.Join(home, ".nix-profile", "bin"),
		filepath.Join(home, ".opam"),
	} {
		add(d)
	}
	if home != "" && home != string(filepath.Separator) {
		add(home)
	}
	patterns[filepath.Join(home, ".opam", "*", "bin")] = true
	patterns[filepath.Join(home, ".rustup", "toolchains", "*", "bin")] = true

	checked := map[string]bool{}
	for p := range patterns {
		matches, err := filepath.Glob(p)
		if err != nil {
			continue
		}
		for _, dir := range matches {
			candidate := filepath.Join(dir, command)
			if checked[candidate] {
				continue
			}
			checked[candidate] = true
			if isExecutableFile(candidate) {
				return candidate
			}
		}
	}
	return ""
}

func cacheBinary(src, dest string) error {
	dir := filepath.Dir(dest)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	tmp := dest + ".tmp"
	if err := copyFile(src, tmp); err != nil {
		return err
	}
	if err := os.Chmod(tmp, os.FileMode(0o755)); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, dest); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

func copyFile(src, dest string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}

func splitCommand(s string) ([]string, error) {
	var args []string
	var cur strings.Builder
	inQuote := rune(0)
	started := false
	for _, r := range s {
		switch {
		case inQuote != 0:
			if r == inQuote {
				inQuote = 0
			} else {
				cur.WriteRune(r)
			}
		case r == '\'' || r == '"':
			inQuote = r
			started = true
		case r == ' ' || r == '\t':
			if started {
				args = append(args, cur.String())
				cur.Reset()
				started = false
			}
		default:
			cur.WriteRune(r)
			started = true
		}
	}
	if inQuote != 0 {
		return nil, fmt.Errorf("lsp: unbalanced quote in install command")
	}
	if started {
		args = append(args, cur.String())
	}
	if len(args) == 0 {
		return nil, fmt.Errorf("lsp: empty install command")
	}
	return args, nil
}

func binaryFileName(command string) string {
	name := filepath.Base(filepath.FromSlash(command))
	if name == "" || name == "." {
		return "lsp-binary"
	}
	return name
}

func findExecutable(command string) string {
	if isAbsolute(command) {
		if isExecutableFile(command) {
			return command
		}
		return ""
	}
	env := os.Environ()
	var path string
	for _, kv := range env {
		if i := strings.IndexByte(kv, '='); i > 0 && kv[:i] == "PATH" {
			path = kv[i+1:]
			break
		}
	}
	if path == "" {
		return ""
	}
	sep := ":"
	if strings.ContainsRune(path, ';') {
		sep = ";"
	}
	for _, dir := range strings.Split(path, sep) {
		if strings.TrimSpace(dir) == "" {
			continue
		}
		candidate := filepath.Join(dir, command)
		if isExecutableFile(candidate) {
			return candidate
		}
	}
	return ""
}

func isExecutableFile(path string) bool {
	fi, err := os.Stat(path)
	if err != nil || !fi.Mode().IsRegular() {
		return false
	}
	return (fi.Mode().Perm() & os.FileMode(0o111)) != os.FileMode(0)
}

func isAbsolute(path string) bool {
	return strings.HasPrefix(path, "/") || strings.HasPrefix(path, "\\")
}

func cachedBinaryDir() (string, error) {
	configDir, err := config.GetConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(configDir, "lsps"), nil
}

func downloadBinary(url, dest, command string) (string, error) {
	dir := filepath.Dir(dest)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("lsp: create cache dir %s: %w", dir, err)
	}

	body, err := http.Fetch(context.Background(), url, http.FetchOptions{
		MaxBytes: 256 << 20,
	})
	if err != nil {
		return "", fmt.Errorf("lsp: download %s: %w", url, err)
	}

	if isArchiveName(url) {
		archivePath := dest + archiveExt(url)
		if err := os.WriteFile(archivePath, body, 0o644); err != nil {
			return "", fmt.Errorf("lsp: write %s: %w", archivePath, err)
		}
		out, err := extractArchive(archivePath, dir, command)
		if err != nil {
			return "", err
		}
		_ = os.Remove(archivePath)
		return out, nil
	}

	tmp := dest + ".tmp"
	if err := os.WriteFile(tmp, body, 0o755); err != nil {
		return "", fmt.Errorf("lsp: write %s: %w", tmp, err)
	}
	if err := os.Chmod(tmp, os.FileMode(0o755)); err != nil {
		_ = os.Remove(tmp)
		return "", fmt.Errorf("lsp: chmod %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, dest); err != nil {
		_ = os.Remove(tmp)
		return "", fmt.Errorf("lsp: rename %s: %w", tmp, err)
	}
	return dest, nil
}

func archiveExt(name string) string {
	lower := strings.ToLower(name)
	switch {
	case strings.HasSuffix(lower, ".zip"):
		return ".zip"
	case strings.HasSuffix(lower, ".tar.gz"), strings.HasSuffix(lower, ".tgz"):
		return ".tar.gz"
	case strings.HasSuffix(lower, ".tar"):
		return ".tar"
	}
	return ""
}

func isArchiveName(name string) bool {
	return archiveExt(name) != ""
}
