package vfs

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/peggco/pegg/internal/core/logger"
)

type Options struct {
	Log *logger.Logger
}

type VFS struct {
	root      string
	scope     string
	wscope    string
	writeOnly string
	mu        sync.RWMutex
	snapshots map[string]string
	ignorer   *Ignorer
	hooks     vfsHooks
	log       *logger.Logger
}

func New(root string, opts Options) (*VFS, error) {
	if root == "" {
		return nil, errors.New("vfs: root path is empty")
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("vfs: resolve root: %w", err)
	}
	fi, err := os.Stat(abs)
	if err != nil {
		return nil, fmt.Errorf("vfs: root %q: %w", abs, err)
	}
	if !fi.IsDir() {
		return nil, fmt.Errorf("vfs: root %q is not a directory", abs)
	}
	return &VFS{
		root:      filepath.Clean(abs),
		snapshots: make(map[string]string),
		ignorer:   newIgnorer(filepath.Clean(abs)),
		hooks:     newHooks(),
		log:       opts.Log,
	}, nil
}

func (v *VFS) Root() string {
	return v.root
}

func (v *VFS) Scoped(relScope string) (*VFS, error) {
	clean, err := prepareScope(relScope)
	if err != nil {
		return nil, err
	}
	if clean == "" {
		return v, nil
	}
	if err := v.ensureScopeDir(relScope, clean); err != nil {
		return nil, err
	}
	return &VFS{
		root:      v.root,
		scope:     clean,
		snapshots: v.snapshots,
		ignorer:   v.ignorer,
		hooks:     v.hooks,
		log:       v.log,
	}, nil
}

func (v *VFS) WriteScope(relScope string) (*VFS, error) {
	clean, err := prepareScope(relScope)
	if err != nil {
		return nil, err
	}
	if clean == "" {
		return v, nil
	}
	if err := v.ensureScopeDir(relScope, clean); err != nil {
		return nil, err
	}
	return &VFS{
		root:      v.root,
		wscope:    clean,
		snapshots: v.snapshots,
		ignorer:   v.ignorer,
		hooks:     v.hooks,
		log:       v.log,
	}, nil
}

func prepareScope(relScope string) (string, error) {
	clean := filepath.Clean(filepath.FromSlash(relScope))
	if clean == "." || clean == "" {
		return "", nil
	}
	if filepath.IsAbs(clean) {
		return "", errors.New("vfs: scope must be relative to the root")
	}
	if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", errors.New("vfs: scope must be inside the root")
	}
	return filepath.ToSlash(clean), nil
}

func (v *VFS) ensureScopeDir(relScope, clean string) error {
	abs := filepath.Join(v.root, filepath.FromSlash(clean))
	if err := os.MkdirAll(abs, 0o755); err != nil {
		return fmt.Errorf("vfs: create scope %q: %w", relScope, err)
	}
	return nil
}

func (v *VFS) Resolve(vpath string) (string, error) {
	return v.resolve(vpath, nil)
}

func (v *VFS) ResolveCtx(ctx context.Context, vpath string) (string, error) {
	return v.resolve(vpath, ctx)
}

func (v *VFS) resolve(vpath string, ctx context.Context) (string, error) {
	if vpath == "" || strings.ContainsRune(vpath, 0) {
		return "", v.outOfBounds("")
	}
	path := filepath.ToSlash(vpath)

	if path == "~" || strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", v.outOfBounds("")
		}
		if path == "~" {
			path = home
		} else {
			path = filepath.ToSlash(filepath.Join(home, path[2:]))
		}
	}
	if strings.HasPrefix(path, "/") {
		return v.evalWithinRootCtx(ctx, filepath.Clean(filepath.FromSlash(path)))
	}

	clean := filepath.Clean(filepath.FromSlash(path))
	if clean == "." || clean == "" {
		if v.scope != "" {
			return filepath.Join(v.root, filepath.FromSlash(v.scope)), nil
		}
		return v.root, nil
	}
	if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		abs := filepath.Clean(filepath.Join(v.root, filepath.FromSlash(clean)))
		req := AccessRequest{Op: OpResolve, Path: abs, Ctx: ctx}
		verdict := v.checkAccess(req)
		if verdict.Allow {
			if verdict.Path != "" {
				return verdict.Path, nil
			}
			return abs, nil
		}
		return "", v.denyAccess(req, verdict)
	}
	return v.evalWithinRootCtx(ctx, filepath.Join(v.root, clean))
}

func (v *VFS) Virtual(ppath string) string {
	clean := filepath.Clean(ppath)
	rel, err := filepath.Rel(v.root, clean)
	if err != nil {
		return filepath.ToSlash(clean)
	}
	if rel == "." {
		return ""
	}
	return filepath.ToSlash(rel)
}

func (v *VFS) evalWithinRoot(phys string) (string, error) {
	return v.evalWithinRootCtx(nil, phys)
}

func (v *VFS) evalWithinRootCtx(ctx context.Context, phys string) (string, error) {
	var rest []string
	probe := phys
	for {
		resolved, err := filepath.EvalSymlinks(probe)
		if err == nil {
			full := filepath.Clean(filepath.Join(resolved, filepath.Join(rest...)))
			if !v.within(full) {
				req := AccessRequest{Op: OpResolve, Path: full, Ctx: ctx}
				verdict := v.checkAccess(req)
				if verdict.Allow {
					if verdict.Path != "" {
						return verdict.Path, nil
					}
					return full, nil
				}
				return "", v.denyAccess(req, verdict)
			}
			return full, nil
		}
		parent := filepath.Dir(probe)
		if parent == probe {
			break
		}
		rest = append([]string{filepath.Base(probe)}, rest...)
		probe = parent
	}
	return "", fmt.Errorf("vfs: cannot resolve path %q", phys)
}

func (v *VFS) within(phys string) bool {
	base := v.root
	if v.scope != "" {
		base = filepath.Join(v.root, filepath.FromSlash(v.scope))
	}
	return phys == base || strings.HasPrefix(phys, base+string(filepath.Separator))
}

func (v *VFS) withinWriteScope(rel string) bool {
	if v.wscope == "" {
		return false
	}
	return rel == v.wscope || strings.HasPrefix(rel, v.wscope+"/")
}

func (v *VFS) writeAllowed(phys string) bool {
	if v.scope != "" {
		return v.within(phys)
	}
	if v.wscope != "" {
		base := filepath.Join(v.root, filepath.FromSlash(v.wscope))
		return phys == base || strings.HasPrefix(phys, base+string(filepath.Separator))
	}
	return true
}

func (v *VFS) ignored(rel string, isDir bool) bool {
	if v.scope != "" || v.withinWriteScope(rel) {
		return false
	}
	return v.ignorer.Ignored(rel, isDir)
}

func (v *VFS) writeIgnored(rel string, isDir bool) bool {
	if v.scope != "" || v.withinWriteScope(rel) {
		return false
	}
	if wo := v.writeOnlyRel(); wo != "" && rel != wo && !strings.HasPrefix(rel, wo+"/") {
		return true
	}
	if isPeggPath(rel) && !isPlansPath(rel) && !isRulesPath(rel) && !isSkillsPath(rel) {
		return true
	}
	return v.ignorer.Ignored(rel, isDir)
}

func (v *VFS) SetWriteOnly(rel string) error {
	clean, err := prepareScope(rel)
	if err != nil {
		return err
	}
	v.mu.Lock()
	v.writeOnly = clean
	v.mu.Unlock()
	return nil
}

func (v *VFS) ClearWriteOnly() {
	v.mu.Lock()
	v.writeOnly = ""
	v.mu.Unlock()
}

func (v *VFS) writeOnlyRel() string {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.writeOnly
}

func (v *VFS) resolveChecked(vpath string) (string, string, bool, error) {
	return v.resolveCheckedCtx(nil, vpath)
}

func (v *VFS) resolveCheckedCtx(ctx context.Context, vpath string) (string, string, bool, error) {
	phys, err := v.ResolveCtx(ctx, vpath)
	if err != nil {
		return "", "", false, err
	}
	rel := v.Virtual(phys)
	isDir := false
	if fi, err := os.Stat(phys); err == nil {
		isDir = fi.IsDir()
	}
	if !v.within(phys) {
		return phys, rel, isDir, nil
	}
	if v.ignored(rel, isDir) {
		return "", "", false, ErrIgnored
	}
	return phys, rel, isDir, nil
}

func (v *VFS) resolveWriteChecked(vpath string) (string, string, bool, error) {
	return v.resolveWriteCheckedCtx(nil, OpWrite, vpath)
}

func (v *VFS) resolveWriteCheckedCtx(ctx context.Context, op AccessOp, vpath string) (string, string, bool, error) {
	phys, err := v.ResolveCtx(ctx, vpath)
	if err != nil {
		return "", "", false, err
	}
	if !v.writeAllowed(phys) {
		req := AccessRequest{Op: op, Path: phys, Ctx: ctx}
		verdict := v.checkAccess(req)
		if !verdict.Allow {
			return "", "", false, v.denyAccess(req, verdict)
		}
	}
	rel := v.Virtual(phys)
	isDir := false
	if fi, err := os.Stat(phys); err == nil {
		isDir = fi.IsDir()
	}
	if !v.within(phys) {
		return phys, rel, isDir, nil
	}
	if v.writeIgnored(rel, isDir) {
		return "", "", false, ErrIgnored
	}
	return phys, rel, isDir, nil
}

func (v *VFS) Read(vpath string) (string, error) {
	return v.ReadCtx(context.Background(), vpath)
}

func (v *VFS) ReadCtx(ctx context.Context, vpath string) (string, error) {
	return v.readRange(ctx, vpath, 0, 0)
}

func (v *VFS) ReadRange(vpath string, offset, limit int) (string, error) {
	return v.ReadRangeCtx(context.Background(), vpath, offset, limit)
}

func (v *VFS) ReadRangeCtx(ctx context.Context, vpath string, offset, limit int) (string, error) {
	return v.readRange(ctx, vpath, offset, limit)
}

func (v *VFS) readRange(ctx context.Context, vpath string, offset, limit int) (string, error) {
	phys, rel, _, err := v.resolveCheckedCtx(ctx, vpath)
	if err != nil {
		return "", err
	}
	v.debug("vfs: read %s", rel)

	data, err := os.ReadFile(phys)
	if err != nil {
		if os.IsNotExist(err) {
			return "", ErrNotFound
		}
		return "", err
	}

	hash := hashBytes(data)
	v.setSnapshot(rel, hash)

	lines := strings.Split(string(data), "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}

	start := offset - 1
	if start < 0 {
		start = 0
	}
	if start > len(lines) {
		start = len(lines)
	}
	end := len(lines)
	if limit > 0 && start+limit < end {
		end = start + limit
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Path: %s | Hash: %s | Size: %d bytes | Lines: %d\n", rel, hash, len(data), len(lines))
	resOffset := start + 1
	resLimit := end - start
	if (resOffset > 1 || limit > 0) && resLimit > 0 {
		fmt.Fprintf(&b, "Range: lines %d-%d\n", resOffset, resOffset+resLimit-1)
	}
	b.WriteString("---\n")
	for i := start; i < end; i++ {
		fmt.Fprintf(&b, "%6d: %s\n", i+1, lines[i])
	}

	out := b.String()
	tc := v.hooks.beforeRead.Apply(TransformContext{Path: rel, Content: out})
	return tc.Content, nil
}

func (v *VFS) Stat(vpath string) (FileInfo, error) {
	phys, rel, isDir, err := v.resolveChecked(vpath)
	if err != nil {
		return FileInfo{}, err
	}
	fi, err := os.Stat(phys)
	if err != nil {
		return FileInfo{}, ErrNotFound
	}
	return FileInfo{
		Path:    rel,
		Name:    fi.Name(),
		Size:    fi.Size(),
		IsDir:   isDir,
		ModTime: fi.ModTime(),
	}, nil
}

func (v *VFS) Exists(vpath string) bool {
	if _, _, _, err := v.resolveChecked(vpath); err != nil {
		return false
	}
	phys, err := v.Resolve(vpath)
	if err != nil {
		return false
	}
	_, err = os.Stat(phys)
	return err == nil
}

func (v *VFS) IsIgnored(vpath string) bool {
	phys, err := v.Resolve(vpath)
	if err != nil {
		return false
	}
	rel := v.Virtual(phys)
	fi, err := os.Stat(phys)
	isDir := err == nil && fi.IsDir()
	return v.ignorer.Ignored(rel, isDir)
}

func (v *VFS) Snapshot(vpath string) (string, bool) {
	phys, err := v.Resolve(vpath)
	if err != nil {
		return "", false
	}
	rel := v.Virtual(phys)
	v.mu.RLock()
	defer v.mu.RUnlock()
	hash, ok := v.snapshots[rel]
	return hash, ok
}

func (v *VFS) Forget(vpath string) {
	phys, err := v.Resolve(vpath)
	if err != nil {
		return
	}
	rel := v.Virtual(phys)
	v.mu.Lock()
	defer v.mu.Unlock()
	delete(v.snapshots, rel)
}

func (v *VFS) setSnapshot(rel, hash string) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.snapshots[rel] = hash
}

func hashBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func (v *VFS) debug(format string, args ...any) {
	if v.log != nil {
		v.log.Fdebug(format, args...)
	}
}
