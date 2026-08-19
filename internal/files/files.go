package files

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// AtomicWriteText writes a file by replacing it only after the complete
// contents have been written. State and metadata files are deliberately
// recoverable if a process is interrupted during an update.
func AtomicWriteText(path, text string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	if err := tmp.Chmod(0o644); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.WriteString(text); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

// AtomicWriteJSON writes indented JSON with a trailing newline, matching the
// existing metadata and state files.
func AtomicWriteJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal JSON: %w", err)
	}
	return AtomicWriteText(path, string(data)+"\n")
}

func ReadJSON(path string, value any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, value); err != nil {
		return fmt.Errorf("decode JSON %s: %w", path, err)
	}
	return nil
}
