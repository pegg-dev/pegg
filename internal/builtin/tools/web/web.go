package web

import (
	"github.com/peggco/pegg/internal/agent/tools"
	"github.com/peggco/pegg/internal/vfs"
)

func WebTools(fs *vfs.VFS) {
	tools.Register(webfetchTool(fs))
	tools.Register(websearchTool(fs))
}
