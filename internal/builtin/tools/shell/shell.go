package shell

import (
	"github.com/peggco/pegg/internal/agent/tools"
	"github.com/peggco/pegg/internal/vfs"
)

func ShellTools(fs *vfs.VFS) {
	tools.Register(bashTool(fs))
}
