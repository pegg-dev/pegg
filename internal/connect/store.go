package connect

import (
	"os"
	"path/filepath"
	"time"

	json "github.com/goccy/go-json"

	"github.com/peggco/pegg/internal/core/config"
)

const (
	connectDirName = "connect"
	deviceFileName = "device.json"
	auditFileName  = "audit.log"
)

func Dir() (string, error) {
	base, err := config.GetConfigDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(base, connectDirName)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	return dir, nil
}

func devicePath() (string, error) {
	d, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, deviceFileName), nil
}

func auditPath() (string, error) {
	d, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, auditFileName), nil
}

func Audit(event string, fields map[string]any) {
	path, err := auditPath()
	if err != nil {
		return
	}
	rec := map[string]any{
		"ts":    time.Now().UTC().Format(time.RFC3339Nano),
		"event": event,
	}
	for k, v := range fields {
		rec[k] = v
	}
	line, err := json.Marshal(rec)
	if err != nil {
		return
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.Write(append(line, '\n'))
}
