// Package gitapi exposes read-only Git operations for the session workspace.
//
// This file owns the service and workspace root resolution. Result types live
// in model.go, read operations in operations.go, command execution in
// command.go, and output parsing in text.go.
package gitapi

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/rezoch340/any-aicli-remote/backend/internal/fsapi"
)

type Service struct {
	workspace    func() string
	rootIdentity *fsapi.RootIdentity
	gitPath      string
	policy       Policy
}

func New(workspace func() string, policy Policy) (*Service, error) {
	if validationError := policy.Validate(); validationError != nil {
		return nil, validationError
	}
	gitPath, _ := exec.LookPath("git")
	return &Service{workspace: workspace, gitPath: gitPath, policy: policy}, nil
}

func NewWithGit(workspace func() string, gitPath string, policy Policy) (*Service, error) {
	if validationError := policy.Validate(); validationError != nil {
		return nil, validationError
	}
	return &Service{workspace: workspace, gitPath: gitPath, policy: policy}, nil
}

func NewPinned(rootIdentity *fsapi.RootIdentity, policy Policy) (*Service, error) {
	if validationError := policy.Validate(); validationError != nil {
		return nil, validationError
	}
	gitPath, _ := exec.LookPath("git")
	return &Service{rootIdentity: rootIdentity, gitPath: gitPath, policy: policy}, nil
}

func NewWithPinnedGit(rootIdentity *fsapi.RootIdentity, gitPath string, policy Policy) (*Service, error) {
	if validationError := policy.Validate(); validationError != nil {
		return nil, validationError
	}
	return &Service{rootIdentity: rootIdentity, gitPath: gitPath, policy: policy}, nil
}

func (service *Service) root() (string, error) {
	if service.rootIdentity != nil {
		if operationError := service.rootIdentity.Validate(); operationError != nil {
			return "", WorkspaceUnavailableError
		}
		return service.rootIdentity.Path(), nil
	}
	if service.workspace == nil {
		return "", WorkspaceUnavailableError
	}
	raw := strings.TrimSpace(service.workspace())
	if raw == "" {
		return "", WorkspaceUnavailableError
	}
	root, operationError := filepath.Abs(filepath.Clean(raw))
	if operationError != nil {
		return "", fmt.Errorf("resolve workspace: %w", operationError)
	}
	info, operationError := os.Stat(root)
	if operationError != nil || !info.IsDir() {
		return "", WorkspaceUnavailableError
	}
	return root, nil
}
