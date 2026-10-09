package vfs

import (
	"github.com/peggco/pegg/internal/core/hook"
)

type TransformContext struct {
	Path    string
	Content string
}

type FileDelete struct {
	Path string
}

type vfsHooks struct {
	beforeRead *hook.Hook[TransformContext]
	afterWrite *hook.Hook[TransformContext]
	onDelete   *hook.Hook[FileDelete]
	access     *hook.Hook[accessCheck]
}

func newHooks() vfsHooks {
	return vfsHooks{
		beforeRead: hook.NewHook[TransformContext](),
		afterWrite: hook.NewHook[TransformContext](),
		onDelete:   hook.NewHook[FileDelete](),
		access:     hook.NewHook[accessCheck](),
	}
}

func (v *VFS) OnBeforeRead(fn func(path, content string) string) {
	v.hooks.beforeRead.Add(func(tc TransformContext) TransformContext {
		tc.Content = fn(tc.Path, tc.Content)
		return tc
	})
}

func (v *VFS) OnAfterWrite(fn func(path, content string) string) {
	v.hooks.afterWrite.Add(func(tc TransformContext) TransformContext {
		tc.Content = fn(tc.Path, tc.Content)
		return tc
	})
}

func (v *VFS) OnFileDelete(fn func(FileDelete) FileDelete) {
	v.hooks.onDelete.Add(fn)
}
