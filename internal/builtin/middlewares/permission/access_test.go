package permission

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/peggco/pegg/internal/vfs"
)

func TestAccessCheckerUnrestrictedGrant(t *testing.T) {
	fs := newTestPermissionFS(t)
	mw := New(Deps{})
	fs.OnAccessCheck(mw.AccessChecker)

	ctx := WithUnrestricted(context.Background())
	path, err := fs.ResolveCtx(ctx, "/etc/passwd")
	if err != nil {
		t.Fatalf("ResolveCtx with unrestricted ctx: %v", err)
	}
	if path != "/etc/passwd" {
		t.Fatalf("path = %q, want /etc/passwd", path)
	}
}

func TestAccessCheckerPermittedPathGrant(t *testing.T) {
	fs := newTestPermissionFS(t)
	mw := New(Deps{})
	fs.OnAccessCheck(mw.AccessChecker)

	outside := filepath.Join(filepath.Dir(fs.Root()), "outside.txt")
	if err := os.WriteFile(outside, []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := fs.ResolveCtx(context.Background(), outside)
	var oob *vfs.OutOfBoundsError
	if !errors.As(err, &oob) {
		t.Fatalf("expected out of bounds, got %v", err)
	}

	ctx := WithPermittedPath(context.Background(), oob.Path)
	path, err := fs.ResolveCtx(ctx, outside)
	if err != nil {
		t.Fatalf("ResolveCtx with permitted path: %v", err)
	}
	if path != filepath.Clean(outside) {
		t.Fatalf("path = %q, want %q", path, outside)
	}
}

func TestAccessCheckerNoGrantDenies(t *testing.T) {
	fs := newTestPermissionFS(t)
	mw := New(Deps{})
	fs.OnAccessCheck(mw.AccessChecker)

	if _, err := fs.ResolveCtx(context.Background(), "/etc/passwd"); !errors.Is(err, vfs.ErrOutOfBounds) {
		t.Fatalf("escape without grant must be denied, got %v", err)
	}
}

func TestAccessCheckerReadAndWriteWithGrant(t *testing.T) {
	fs := newTestPermissionFS(t)
	mw := New(Deps{})
	fs.OnAccessCheck(mw.AccessChecker)

	outside := filepath.Join(filepath.Dir(fs.Root()), "secret.txt")
	if err := os.WriteFile(outside, []byte("top secret\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := fs.Read(outside); !errors.Is(err, vfs.ErrOutOfBounds) {
		t.Fatalf("strict read should fail, got %v", err)
	}

	ctx := WithUnrestricted(context.Background())
	content, err := fs.ReadCtx(ctx, outside)
	if err != nil {
		t.Fatalf("ReadCtx with unrestricted ctx: %v", err)
	}
	if content == "" {
		t.Fatal("expected file content")
	}

	outDir := filepath.Join(filepath.Dir(fs.Root()), "writedir")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := fs.WriteCtx(ctx, filepath.Join(outDir, "new.txt"), []byte("data")); err != nil {
		t.Fatalf("WriteCtx with unrestricted ctx: %v", err)
	}
}

func newTestPermissionFS(t *testing.T) *vfs.VFS {
	t.Helper()
	root := t.TempDir()
	fs, err := vfs.New(root, vfs.Options{})
	if err != nil {
		t.Fatalf("mount vfs: %v", err)
	}
	return fs
}
