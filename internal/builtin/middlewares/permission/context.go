package permission

import (
	"context"
	"path/filepath"

	"github.com/peggco/pegg/internal/vfs"
)

type permitCtxKey struct{}

type permitState struct {
	unrestricted bool
	paths        map[string]struct{}
}

func WithUnrestricted(ctx context.Context) context.Context {
	return withPermit(ctx, func(st *permitState) { st.unrestricted = true })
}

func WithPermittedPath(ctx context.Context, absPath string) context.Context {
	if absPath == "" {
		return ctx
	}
	clean := filepath.Clean(absPath)
	return withPermit(ctx, func(st *permitState) {
		if st.paths == nil {
			st.paths = make(map[string]struct{})
		}
		st.paths[clean] = struct{}{}
	})
}

func withPermit(ctx context.Context, mutate func(*permitState)) context.Context {
	if ctx == nil {
		return ctx
	}
	st, ok := ctx.Value(permitCtxKey{}).(*permitState)
	if !ok {
		st = &permitState{}
	}
	copy := &permitState{unrestricted: st.unrestricted}
	if len(st.paths) > 0 {
		copy.paths = make(map[string]struct{}, len(st.paths))
		for p := range st.paths {
			copy.paths[p] = struct{}{}
		}
	}
	mutate(copy)
	return context.WithValue(ctx, permitCtxKey{}, copy)
}

func (m *Middleware) AccessChecker(req vfs.AccessRequest, verdict vfs.AccessVerdict) vfs.AccessVerdict {
	if req.Ctx == nil {
		return verdict
	}
	st, ok := req.Ctx.Value(permitCtxKey{}).(*permitState)
	if !ok || st == nil {
		return verdict
	}
	if st.unrestricted {
		verdict.Allow = true
		verdict.Err = nil
		return verdict
	}
	if _, ok := st.paths[filepath.Clean(req.Path)]; ok {
		verdict.Allow = true
		verdict.Err = nil
		return verdict
	}
	return verdict
}
