package vfs

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func newTestFS(t *testing.T) *VFS {
	t.Helper()
	root := t.TempDir()
	fs, err := New(root, Options{})
	if err != nil {
		t.Fatalf("mount vfs: %v", err)
	}
	return fs
}

func TestOutOfBoundsError(t *testing.T) {
	fs := newTestFS(t)

	_, err := fs.Resolve("/etc/passwd")
	var oob *OutOfBoundsError
	if !errors.As(err, &oob) {
		t.Fatalf("expected *OutOfBoundsError, got %T: %v", err, err)
	}
	if oob.Path == "" {
		t.Fatal("expected the offending path in the error")
	}
	if !errors.Is(err, ErrOutOfBounds) {
		t.Fatalf("errors.Is(err, ErrOutOfBounds) = false")
	}
}

func TestAccessDefaultDeniesEscape(t *testing.T) {
	fs := newTestFS(t)

	if _, err := fs.ResolveCtx(context.Background(), "/etc/passwd"); !errors.Is(err, ErrOutOfBounds) {
		t.Fatalf("escape without hooks must be denied, got %v", err)
	}
}

func TestAccessFilterCanAllowEscape(t *testing.T) {
	fs := newTestFS(t)
	fs.OnAccessCheck(func(req AccessRequest, verdict AccessVerdict) AccessVerdict {
		if req.Op != OpResolve {
			t.Errorf("expected OpResolve, got %q", req.Op)
		}
		if req.Path != "/etc/passwd" {
			t.Errorf("expected path /etc/passwd, got %q", req.Path)
		}
		verdict.Allow = true
		verdict.Err = nil
		return verdict
	})

	path, err := fs.ResolveCtx(context.Background(), "/etc/passwd")
	if err != nil {
		t.Fatalf("ResolveCtx with allowing filter: %v", err)
	}
	if path != "/etc/passwd" {
		t.Fatalf("path = %q, want /etc/passwd", path)
	}
}

func TestAccessFilterCanDenyWithCustomError(t *testing.T) {
	fs := newTestFS(t)
	boom := errors.New("custom denial")
	fs.OnAccessCheck(func(req AccessRequest, verdict AccessVerdict) AccessVerdict {
		verdict.Err = boom
		return verdict
	})

	_, err := fs.ResolveCtx(context.Background(), "/etc/passwd")
	if !errors.Is(err, boom) {
		t.Fatalf("expected custom denial, got %v", err)
	}
}

func TestAccessFilterCanRedirect(t *testing.T) {
	fs := newTestFS(t)
	target := filepath.Join(filepath.Dir(fs.Root()), "target.txt")
	if err := os.WriteFile(target, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	fs.OnAccessCheck(func(req AccessRequest, verdict AccessVerdict) AccessVerdict {
		if req.Op == OpResolve {
			verdict.Allow = true
			verdict.Path = target
			verdict.Err = nil
		}
		return verdict
	})

	path, err := fs.ResolveCtx(context.Background(), "/etc/passwd")
	if err != nil {
		t.Fatalf("ResolveCtx with redirecting filter: %v", err)
	}
	if path != filepath.Clean(target) {
		t.Fatalf("path = %q, want %q", path, target)
	}
}

func TestAccessAppliesToReadsAndWrites(t *testing.T) {
	fs := newTestFS(t)
	outside := filepath.Join(filepath.Dir(fs.Root()), "secret.txt")
	if err := os.WriteFile(outside, []byte("top secret\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := fs.Read(outside); !errors.Is(err, ErrOutOfBounds) {
		t.Fatalf("strict read should fail, got %v", err)
	}

	fs.OnAccessCheck(func(req AccessRequest, verdict AccessVerdict) AccessVerdict {
		verdict.Allow = true
		verdict.Err = nil
		return verdict
	})

	content, err := fs.ReadCtx(context.Background(), outside)
	if err != nil {
		t.Fatalf("ReadCtx with allowing filter: %v", err)
	}
	if content == "" {
		t.Fatal("expected file content")
	}

	if _, err := fs.WriteCtx(context.Background(), filepath.Join(filepath.Dir(fs.Root()), "new.txt"), []byte("x")); err != nil {
		t.Fatalf("WriteCtx with allowing filter: %v", err)
	}
}

func TestGlobalPeggPathsAreWritable(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	fs := newTestFS(t)

	cfg := filepath.Join(home, ".pegg", "pegg.json")
	if _, err := fs.Write(cfg, []byte(`{"theme":"dark"}`)); err != nil {
		t.Fatalf("write global config: %v", err)
	}
	if data, err := os.ReadFile(cfg); err != nil || string(data) != `{"theme":"dark"}` {
		t.Fatalf("global config = %q, err = %v", data, err)
	}

	skill := filepath.Join(home, ".pegg", "skills", "demo", "SKILL.md")
	if _, err := fs.Write(skill, []byte("# demo\n")); err != nil {
		t.Fatalf("write global skill: %v", err)
	}
	if _, err := os.Stat(skill); err != nil {
		t.Fatalf("global skill not written: %v", err)
	}
}

func TestGlobalPeggOtherPathsStayDenied(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	fs := newTestFS(t)

	for _, p := range []string{
		filepath.Join(home, ".pegg", "secrets.env"),
		filepath.Join(home, ".pegg", "skills-other", "x"),
		filepath.Join(home, "elsewhere", "file"),
	} {
		if _, err := fs.Write(p, []byte("x")); !errors.Is(err, ErrOutOfBounds) {
			t.Fatalf("write %q: want ErrOutOfBounds, got %v", p, err)
		}
	}
}

func TestAccessWriteOpIsPassed(t *testing.T) {
	fs := newTestFS(t)
	scoped, err := fs.WriteScope("sub")
	if err != nil {
		t.Fatal(err)
	}
	var writeOps []AccessOp
	scoped.OnAccessCheck(func(req AccessRequest, verdict AccessVerdict) AccessVerdict {
		if req.Op == OpWrite {
			writeOps = append(writeOps, req.Op)
		}
		verdict.Allow = true
		verdict.Err = nil
		return verdict
	})

	if _, err := scoped.WriteCtx(context.Background(), "../op.txt", []byte("x")); err != nil {
		t.Fatalf("WriteCtx: %v", err)
	}
	if len(writeOps) == 0 {
		t.Fatal("expected an OpWrite access check for the scoped write")
	}
}

func TestIgnoreStillEnforcedWithEscapeAllowed(t *testing.T) {
	fs := newTestFS(t)
	if err := os.WriteFile(filepath.Join(fs.Root(), ".gitignore"), []byte("ignored.txt\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(fs.Root(), "ignored.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	fs.OnAccessCheck(func(req AccessRequest, verdict AccessVerdict) AccessVerdict {
		verdict.Allow = true
		verdict.Err = nil
		return verdict
	})

	if _, err := fs.ReadCtx(context.Background(), "ignored.txt"); !errors.Is(err, ErrIgnored) {
		t.Fatalf("ignore protection must stay enforced, got %v", err)
	}
}

func TestIgnoreDoesNotApplyOutsideWorkspace(t *testing.T) {
	fs := newTestFS(t)
	parent := filepath.Dir(fs.Root())

	if err := os.WriteFile(filepath.Join(fs.Root(), ".gitignore"), []byte("*.secret\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(parent, ".gitignore"), []byte("*.secret\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(parent, "notes.secret")
	if err := os.WriteFile(outside, []byte("data\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	fs.OnAccessCheck(func(req AccessRequest, verdict AccessVerdict) AccessVerdict {
		verdict.Allow = true
		verdict.Err = nil
		return verdict
	})

	content, err := fs.ReadCtx(context.Background(), outside)
	if err != nil {
		t.Fatalf("allowed escape must bypass ignore rules outside the workspace, got %v", err)
	}
	if content == "" {
		t.Fatal("expected file content")
	}

	if _, err := fs.WriteCtx(context.Background(), filepath.Join(parent, "out.secret"), []byte("x")); err != nil {
		t.Fatalf("allowed write outside workspace must bypass ignore rules, got %v", err)
	}
}
