package vfs

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type FileInfo struct {
	Path    string
	Name    string
	Size    int64
	IsDir   bool
	ModTime time.Time
}

func (v *VFS) Write(vpath string, data []byte) (string, error) {
	return v.WriteCtx(context.Background(), vpath, data)
}

func (v *VFS) WriteCtx(ctx context.Context, vpath string, data []byte) (string, error) {
	phys, rel, _, err := v.resolveWriteCheckedCtx(ctx, OpWrite, vpath)
	if err != nil {
		return "", err
	}
	if err := v.atomicWrite(phys, data); err != nil {
		return "", err
	}
	hash := hashBytes(data)
	v.setSnapshot(rel, hash)
	v.debug("vfs: wrote %s (%d bytes)", rel, len(data))

	out := ""
	tc := v.hooks.afterWrite.Apply(TransformContext{Path: rel, Content: out})
	return tc.Content, nil
}

func (v *VFS) Edit(vpath, oldString, newString string, replaceAll bool) (string, error) {
	return v.EditCtx(context.Background(), vpath, oldString, newString, replaceAll)
}

func (v *VFS) EditCtx(ctx context.Context, vpath, oldString, newString string, replaceAll bool) (string, error) {
	phys, rel, _, err := v.resolveWriteCheckedCtx(ctx, OpEdit, vpath)
	if err != nil {
		return "", err
	}

	v.mu.RLock()
	snapshot, ok := v.snapshots[rel]
	v.mu.RUnlock()
	if !ok {
		return "", ErrReadRequired
	}

	data, err := os.ReadFile(phys)
	if err != nil {
		return "", ErrNotFound
	}
	if hashBytes(data) != snapshot {
		return "", ErrFileChanged
	}

	strData := string(data)
	matchCount := strings.Count(strData, oldString)

	if matchCount == 0 {
		return "", ErrNoMatch
	}

	if !replaceAll && matchCount > 1 {
		return "", ErrMultipleMatch
	}

	occurrences := 1
	if replaceAll {
		occurrences = -1
	}
	content, changed := replaceOccurrences(string(data), oldString, newString, occurrences)
	if !changed {
		return "", ErrNoMatch
	}
	if err := v.atomicWrite(phys, []byte(content)); err != nil {
		return "", err
	}

	hash := hashBytes([]byte(content))
	v.setSnapshot(rel, hash)
	v.debug("vfs: edited %s", rel)

	out := ""
	tc := v.hooks.afterWrite.Apply(TransformContext{Path: rel, Content: out})
	return tc.Content, nil
}

func (v *VFS) Delete(vpath string) error {
	return v.DeleteCtx(context.Background(), vpath)
}

func (v *VFS) DeleteCtx(ctx context.Context, vpath string) error {
	phys, rel, _, err := v.resolveWriteCheckedCtx(ctx, OpDelete, vpath)
	if err != nil {
		return err
	}

	ev := v.hooks.onDelete.Apply(FileDelete{Path: rel})
	if ev.Path != rel {
		phys, rel, _, err = v.resolveWriteCheckedCtx(ctx, OpDelete, ev.Path)
		if err != nil {
			return err
		}
	}

	if err := os.Remove(phys); err != nil {
		return err
	}
	v.mu.Lock()
	delete(v.snapshots, rel)
	v.mu.Unlock()
	v.debug("vfs: deleted %s", rel)
	return nil
}

func (v *VFS) atomicWrite(phys string, data []byte) error {
	dir := filepath.Dir(phys)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".pegg-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() {
		if tmpName != "" {
			_ = os.Remove(tmpName)
		}
	}()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}

	mode := os.FileMode(0o644)
	if fi, err := os.Stat(phys); err == nil {
		mode = fi.Mode().Perm()
	}
	if err := os.Chmod(tmpName, mode); err != nil {
		return err
	}
	if err := os.Rename(tmpName, phys); err != nil {
		return err
	}
	tmpName = ""
	return nil
}

func replaceOccurrences(s, old, new string, n int) (string, bool) {
	if old == "" {
		return s, false
	}
	count := strings.Count(s, old)
	if count == 0 {
		return s, false
	}
	limit := count
	if n >= 0 && n < limit {
		limit = n
	}
	if limit == 0 {
		return s, false
	}
	return strings.Replace(s, old, new, limit), true
}
