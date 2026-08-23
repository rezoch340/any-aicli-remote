// Package atomicfile writes private state files with atomic replacement.
package atomicfile

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/google/renameio/v2"
)

// WritePrivate atomically replaces path with data. Parent directories and the
// resulting file are private to the current user.
func WritePrivate(path string, data []byte) error {
	directory := filepath.Dir(path)
	if operationError := os.MkdirAll(directory, 0o700); operationError != nil {
		return fmt.Errorf("create private directory: %w", operationError)
	}
	file, operationError := renameio.TempFile(directory, path)
	if operationError != nil {
		return fmt.Errorf("create replacement file: %w", operationError)
	}
	defer file.Cleanup()
	if operationError := file.Chmod(0o600); operationError != nil {
		return fmt.Errorf("protect replacement file: %w", operationError)
	}
	written, operationError := file.Write(data)
	if operationError != nil {
		return fmt.Errorf("write replacement file: %w", operationError)
	}
	if written != len(data) {
		return fmt.Errorf("write replacement file: wrote %d of %d bytes", written, len(data))
	}
	if operationError := file.CloseAtomicallyReplace(); operationError != nil {
		return fmt.Errorf("replace private file: %w", operationError)
	}
	return nil
}
