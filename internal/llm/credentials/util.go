package credentials

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

func decodeObject(data []byte) (map[string]any, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var obj map[string]any
	if err := dec.Decode(&obj); err != nil {
		return nil, err
	}
	return obj, nil
}

func objectField(obj map[string]any, key string) map[string]any {
	if v, ok := obj[key].(map[string]any); ok {
		return v
	}
	return nil
}

func stringField(obj map[string]any, key string) string {
	if v, ok := obj[key].(string); ok {
		return v
	}
	return ""
}

func intField(obj map[string]any, key string) (int64, bool) {
	switch v := obj[key].(type) {
	case json.Number:
		if n, err := v.Int64(); err == nil {
			return n, true
		}
		if f, err := v.Float64(); err == nil {
			return int64(f), true
		}
	}
	return 0, false
}

func writeJSONAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("credentials: create dir: %w", err)
	}
	tmp, err := os.CreateTemp(dir, ".creds-*.tmp")
	if err != nil {
		return fmt.Errorf("credentials: create temp file: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}
