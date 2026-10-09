package lsp

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"

	"github.com/peggco/pegg/internal/core/config"
	utiltar "github.com/peggco/pegg/internal/utils/tar"
)

func makeZip(t *testing.T, name string, body []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create(name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write(body); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestExtractZipAndTarGz(t *testing.T) {
	body := []byte("#!/bin/sh\necho fake\n")
	zipBytes := makeZip(t, "terraform-ls", body)
	dir := t.TempDir()
	zpath := filepath.Join(dir, "terraform-ls.zip")
	if err := os.WriteFile(zpath, zipBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := extractArchive(zpath, dir, "terraform-ls")
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(dir, "terraform-ls")
	if out != want {
		t.Fatalf("zip out = %q, want %q", out, want)
	}
	data, err := os.ReadFile(want)
	if err != nil || !bytes.Equal(data, body) {
		t.Fatalf("zip content mismatch: %v %q", err, data)
	}
	if !isExecutableFile(want) {
		t.Fatal("extracted zip binary must be executable")
	}

	tarpath := filepath.Join(dir, "terraform-ls.tar.gz")
	if err := utiltar.WriteGz(tarpath, []utiltar.Entry{
		{Name: "terraform-ls", Mode: 0o755, Body: body},
	}); err != nil {
		t.Fatal(err)
	}
	out, err = extractArchive(tarpath, dir, "terraform-ls")
	if err != nil {
		t.Fatal(err)
	}
	if out != want {
		t.Fatalf("tar out = %q, want %q", out, want)
	}
}

func TestResolveBinaryDownloadZipToCache(t *testing.T) {
	home := t.TempDir()
	empty := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("PATH", empty)

	body := []byte("#!/bin/sh\nfake terraform-ls\n")
	srv, err := newBlobServer(makeZip(t, "terraform-ls", body))
	if err != nil {
		t.Fatal(err)
	}
	defer srv.close()

	cfg := config.LanguageServerConfig{
		Command:  "terraform-ls",
		Download: srv.addr() + "/terraform-ls_1.0.0_linux_amd64.zip",
	}
	bin, err := ResolveBinary("terraform-ls", cfg)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(home, ".pegg", "lsps", "terraform-ls")
	if bin != want {
		t.Fatalf("bin = %q, want %q", bin, want)
	}
	data, _ := os.ReadFile(want)
	if !bytes.Equal(data, body) {
		t.Fatalf("cached = %q", data)
	}
	if _, err := os.Stat(want + ".zip"); err == nil {
		t.Fatal("staged zip should be removed after extraction")
	}
	if srv.requestCount() != 1 {
		t.Fatalf("hits = %d, want 1", srv.requestCount())
	}
	if _, err := ResolveBinary("terraform-ls", cfg); err != nil {
		t.Fatal(err)
	}
	if srv.requestCount() != 1 {
		t.Fatalf("hits = %d after cache, want 1", srv.requestCount())
	}
}

func TestInstallCommandStagesArchive(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("PATH", "/usr/bin:/bin")

	cacheDir := filepath.Join(home, ".pegg", "lsps")
	staged := filepath.Join(cacheDir, "terraform-ls.zip")
	cached := filepath.Join(cacheDir, "terraform-ls")

	body := []byte("#!/bin/sh\nfake terraform-ls\n")
	zipBytes := makeZip(t, "terraform-ls", body)

	snippet := `mkdir -p "` + cacheDir + `" && ` +
		`echo "` + hostSafeBase64(zipBytes) + `" | base64 -d > "` + staged + `"`

	cfg := config.LanguageServerConfig{
		Command: "terraform-ls",
		Install: snippet,
	}
	bin, err := ResolveBinary("terraform-ls", cfg)
	if err != nil {
		t.Fatal(err)
	}
	if bin != cached {
		t.Fatalf("bin = %q, want %q", bin, cached)
	}
	data, _ := os.ReadFile(cached)
	if !bytes.Equal(data, body) {
		t.Fatalf("cached content mismatch: %q", data)
	}
	if !isExecutableFile(cached) {
		t.Fatal("cached binary not executable")
	}
	if _, err := os.Stat(staged); err == nil {
		t.Fatal("staged zip should be removed after extraction")
	}
}

func hostSafeBase64(b []byte) string {
	return base64.StdEncoding.EncodeToString(b)
}

func TestResolveBinaryDownloadTarGzToCache(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("PATH", filepath.Join(t.TempDir(), "empty"))

	body := []byte("#!/bin/sh\nfake tinymist\n")
	tarGz := tarBytes(t, map[string][]byte{"bin/tinymist": body})
	srv, err := newBlobServer(tarGz)
	if err != nil {
		t.Fatal(err)
	}
	defer srv.close()

	cfg := config.LanguageServerConfig{
		Command:  "tinymist",
		Download: srv.addr() + "/tinymist-x86_64-unknown-linux-gnu.tar.gz",
	}
	bin, err := ResolveBinary("tinymist", cfg)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(home, ".pegg", "lsps", "tinymist")
	if bin != want {
		t.Fatalf("bin = %q, want %q", bin, want)
	}
	data, _ := os.ReadFile(want)
	if !bytes.Equal(data, body) {
		t.Fatalf("cached = %q", data)
	}
	if !isExecutableFile(want) {
		t.Fatal("cached binary must be executable")
	}
	if _, err := os.Stat(want + ".tar.gz"); err == nil {
		t.Fatal("staged tar.gz should be removed")
	}
}

func tarBytes(t *testing.T, files map[string][]byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, body := range files {
		if err := tw.WriteHeader(&tar.Header{
			Name: name, Mode: 0o755, Size: int64(len(body)),
		}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write(body); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}
