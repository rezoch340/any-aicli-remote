package fsapi

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// RootIdentity pins the filesystem identity of a canonical workspace path.
// Validate fails closed when that path is later replaced, including by a
// symlink to another directory.
type RootIdentity struct {
	canonicalPath string
	fileInfo      os.FileInfo
}

// PinRoot canonicalizes a workspace once and records its filesystem identity.
func PinRoot(raw string) (*RootIdentity, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, PathRequiredError
	}
	absolutePath, operationError := filepath.Abs(filepath.Clean(raw))
	if operationError != nil {
		return nil, fmt.Errorf("resolve workspace: %w", operationError)
	}
	pinnedPath := absolutePath
	linkInfo, operationError := os.Lstat(absolutePath)
	if operationError != nil {
		return nil, fmt.Errorf("open workspace: %w", operationError)
	}
	if linkInfo.Mode()&os.ModeSymlink != 0 {
		pinnedPath, operationError = filepath.EvalSymlinks(absolutePath)
		if operationError != nil {
			return nil, fmt.Errorf("open workspace: %w", operationError)
		}
	}
	fileInfo, operationError := os.Stat(pinnedPath)
	if operationError != nil {
		return nil, fmt.Errorf("open workspace: %w", operationError)
	}
	if !fileInfo.IsDir() {
		return nil, NotDirectoryError
	}
	return &RootIdentity{canonicalPath: pinnedPath, fileInfo: fileInfo}, nil
}

// Path returns the canonical path that was pinned.
func (identity *RootIdentity) Path() string {
	if identity == nil {
		return ""
	}
	return identity.canonicalPath
}

// Validate confirms that the pinned path still names the same directory and
// has not become a symbolic link.
func (identity *RootIdentity) Validate() error {
	if identity == nil || identity.canonicalPath == "" || identity.fileInfo == nil {
		return WorkspaceChangedError
	}
	linkInfo, operationError := os.Lstat(identity.canonicalPath)
	if operationError != nil || linkInfo.Mode()&os.ModeSymlink != 0 {
		return WorkspaceChangedError
	}
	currentInfo, operationError := os.Stat(identity.canonicalPath)
	if operationError != nil || !currentInfo.IsDir() || !os.SameFile(identity.fileInfo, currentInfo) {
		return WorkspaceChangedError
	}
	return nil
}

// OpenRoot opens the pinned directory and verifies the descriptor and path
// still identify the directory captured by PinRoot.
func (identity *RootIdentity) OpenRoot() (*os.Root, error) {
	if operationError := identity.Validate(); operationError != nil {
		return nil, operationError
	}
	rootFilesystem, operationError := os.OpenRoot(identity.Path())
	if operationError != nil {
		return nil, fmt.Errorf("open workspace: %w", operationError)
	}
	openedInfo, operationError := rootFilesystem.Stat(".")
	if operationError != nil || !os.SameFile(identity.fileInfo, openedInfo) {
		_ = rootFilesystem.Close()
		return nil, WorkspaceChangedError
	}
	if operationError := identity.Validate(); operationError != nil {
		_ = rootFilesystem.Close()
		return nil, operationError
	}
	return rootFilesystem, nil
}

func relativePath(root, raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "." || raw == string(filepath.Separator) {
		return ".", nil
	}
	var relativePath string
	if filepath.IsAbs(raw) {
		canonicalInput, operationError := canonicalizePathWithMissingTail(filepath.Clean(raw))
		if operationError != nil {
			return "", OutsideWorkspaceError
		}
		canonicalRoot, operationError := canonicalizePathWithMissingTail(filepath.Clean(root))
		if operationError != nil {
			return "", OutsideWorkspaceError
		}
		relativePath, operationError = filepath.Rel(canonicalRoot, canonicalInput)
		if operationError != nil {
			return "", OutsideWorkspaceError
		}
	} else {
		relativePath = filepath.Clean(raw)
	}
	if relativePath == "." {
		return relativePath, nil
	}
	if relativePath == ".." || strings.HasPrefix(relativePath, ".."+string(filepath.Separator)) || filepath.IsAbs(relativePath) {
		return "", OutsideWorkspaceError
	}
	return relativePath, nil
}

func canonicalizePathWithMissingTail(path string) (string, error) {
	existingPath := filepath.Clean(path)
	missingParts := make([]string, 0)
	for {
		_, operationError := os.Lstat(existingPath)
		if operationError == nil {
			break
		}
		if !errors.Is(operationError, os.ErrNotExist) {
			return "", operationError
		}
		parentPath := filepath.Dir(existingPath)
		if parentPath == existingPath {
			return "", operationError
		}
		missingParts = append(missingParts, filepath.Base(existingPath))
		existingPath = parentPath
	}
	canonicalPath, operationError := filepath.EvalSymlinks(existingPath)
	if operationError != nil {
		return "", operationError
	}
	for partIndex := len(missingParts) - 1; partIndex >= 0; partIndex-- {
		canonicalPath = filepath.Join(canonicalPath, missingParts[partIndex])
	}
	return canonicalPath, nil
}

func absolutePath(root, relativePath string) string {
	if relativePath == "." || relativePath == "" {
		return root
	}
	return filepath.Join(root, relativePath)
}

func normalizeRootError(operationError error) error {
	if operationError == nil {
		return nil
	}
	lower := strings.ToLower(operationError.Error())
	if strings.Contains(lower, "escapes") || strings.Contains(lower, "outside") {
		return fmt.Errorf("%w: %v", OutsideWorkspaceError, operationError)
	}
	return operationError
}
