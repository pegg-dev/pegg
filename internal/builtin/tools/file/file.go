package file

import (
	"errors"

	"github.com/peggco/pegg/internal/agent/tool"
	"github.com/peggco/pegg/internal/agent/tools"
	"github.com/peggco/pegg/internal/vfs"
)

func isScopeError(err error) bool {
	return errors.Is(err, vfs.ErrOutOfBounds)
}

func FileTools(fs *vfs.VFS) {
	for _, t := range Tools(fs) {
		tools.Register(t)
	}
}

func Tools(fs *vfs.VFS) []tool.Tool {
	return []tool.Tool{
		readTool(fs),
		editTool(fs),
		writeTool(fs),
		listTool(fs),
		globTool(fs),
		grepTool(fs),
	}
}
