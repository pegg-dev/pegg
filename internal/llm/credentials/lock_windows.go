//go:build windows

package credentials

import (
	"os"
	"time"
)

func lockFile(path string, fn func() error) error {
	deadline := time.Now().Add(5 * time.Second)
	for {
		f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0o600)
		if err == nil {
			f.Close()
			defer os.Remove(path)
			return fn()
		}
		if time.Now().After(deadline) {
			return err
		}
		time.Sleep(25 * time.Millisecond)
	}
}
