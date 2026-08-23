// Package fsapi exposes the session workspace filesystem.
//
// This file owns workspace binding and containment-checked handles. The client
// JSON model lives in model.go, file operations in operations.go, text
// classification in text.go, and root pinning in root.go.
package fsapi

import (
	"errors"
	"os"
	"strings"
	"sync"
)

type Service struct {
	mutex          sync.RWMutex
	root           string
	rootFilesystem *os.Root
	rootIdentity   *RootIdentity
	policy         Policy
}

func New(root string, policy Policy) (*Service, error) {
	if validationError := policy.Validate(); validationError != nil {
		return nil, validationError
	}
	service := &Service{policy: policy}
	if _, operationError := service.SetRoot(root); operationError != nil {
		return nil, operationError
	}
	return service, nil
}

// NewPinned opens a filesystem service for an already-bound workspace. The
// opened os.Root and the path are checked against the original identity.
func NewPinned(identity *RootIdentity, policy Policy) (*Service, error) {
	if validationError := policy.Validate(); validationError != nil {
		return nil, validationError
	}
	rootFilesystem, operationError := identity.OpenRoot()
	if operationError != nil {
		return nil, operationError
	}
	return &Service{root: identity.Path(), rootFilesystem: rootFilesystem, rootIdentity: identity, policy: policy}, nil
}

func (service *Service) Close() error {
	service.mutex.Lock()
	defer service.mutex.Unlock()
	if service.rootFilesystem == nil {
		return nil
	}
	operationError := service.rootFilesystem.Close()
	service.rootFilesystem = nil
	return operationError
}

func (service *Service) Root() string {
	service.mutex.RLock()
	defer service.mutex.RUnlock()
	return service.root
}

func (service *Service) Info() RootInfo {
	service.mutex.RLock()
	defer service.mutex.RUnlock()
	exists := false
	if service.rootFilesystem != nil && service.validateRootLocked() == nil {
		if info, operationError := service.rootFilesystem.Stat("."); operationError == nil {
			exists = info.IsDir()
		}
	}
	return RootInfo{Root: service.root, Exists: exists}
}

func (service *Service) SetRoot(raw string) (RootInfo, error) {
	identity, operationError := PinRoot(raw)
	if operationError != nil {
		return RootInfo{}, operationError
	}
	root, operationError := identity.OpenRoot()
	if operationError != nil {
		return RootInfo{}, operationError
	}

	service.mutex.Lock()
	previous := service.rootFilesystem
	service.root = identity.Path()
	service.rootFilesystem = root
	service.rootIdentity = identity
	service.mutex.Unlock()
	if previous != nil {
		_ = previous.Close()
	}
	return RootInfo{Root: identity.Path(), Exists: true}, nil
}

func (service *Service) validateRootLocked() error {
	if service.rootFilesystem == nil {
		return os.ErrClosed
	}
	if operationError := service.rootIdentity.Validate(); operationError != nil {
		return operationError
	}
	openedInfo, operationError := service.rootFilesystem.Stat(".")
	if operationError != nil || !os.SameFile(service.rootIdentity.fileInfo, openedInfo) {
		return WorkspaceChangedError
	}
	return nil
}

// Resolve converts a client path into an absolute workspace path. File operations
// still use os.Root, so a symlink cannot escape the workspace between validation
// and access.
func (service *Service) Resolve(raw string) (string, error) {
	service.mutex.RLock()
	defer service.mutex.RUnlock()
	relativePath, operationError := relativePath(service.root, raw)
	if operationError != nil {
		return "", operationError
	}
	if operationError := service.validateRootLocked(); operationError != nil {
		return "", operationError
	}
	if _, operationError := service.rootFilesystem.Stat(relativePath); operationError != nil && !errors.Is(operationError, os.ErrNotExist) {
		return "", normalizeRootError(operationError)
	}
	return absolutePath(service.root, relativePath), nil
}

// OpenRead opens a workspace-relative regular file through os.Root so symlink
// traversal cannot escape the workspace between validation and access.
func (service *Service) OpenRead(raw string) (*os.File, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, PathRequiredError
	}
	service.mutex.RLock()
	defer service.mutex.RUnlock()
	if operationError := service.validateRootLocked(); operationError != nil {
		return nil, operationError
	}
	relativePath, operationError := relativePath(service.root, raw)
	if operationError != nil {
		return nil, operationError
	}
	file, operationError := service.rootFilesystem.Open(relativePath)
	if operationError != nil {
		return nil, normalizeRootError(operationError)
	}
	fileInfo, operationError := file.Stat()
	if operationError != nil {
		_ = file.Close()
		return nil, operationError
	}
	if !fileInfo.Mode().IsRegular() {
		_ = file.Close()
		return nil, NotFileError
	}
	return file, nil
}

// OpenDirectory opens a workspace-relative directory, pinning its descriptor.
func (service *Service) OpenDirectory(raw string) (*os.File, error) {
	service.mutex.RLock()
	defer service.mutex.RUnlock()
	if operationError := service.validateRootLocked(); operationError != nil {
		return nil, operationError
	}
	relativePath, operationError := relativePath(service.root, raw)
	if operationError != nil {
		return nil, operationError
	}
	file, operationError := service.rootFilesystem.Open(relativePath)
	if operationError != nil {
		return nil, normalizeRootError(operationError)
	}
	fileInfo, operationError := file.Stat()
	if operationError != nil {
		_ = file.Close()
		return nil, operationError
	}
	if !fileInfo.IsDir() {
		_ = file.Close()
		return nil, NotDirectoryError
	}
	return file, nil
}
