package vfs

import (
	"fmt"
	"os"

	"github.com/peggco/pegg/internal/core/logger"
)

func VFSModule(log *logger.Logger) (*VFS, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("bootstrap: get working directory: %w", err)
	}
	return New(cwd, Options{Log: log})
}
