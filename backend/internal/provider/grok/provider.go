// Package grok implements Any AI CLI Remote's Grok provider adapter.
//
// This file is the adapter's composition root: identity, configuration, and
// construction. Session discovery and titles live in sessioncatalog.go,
// filesystem containment in sessionstore.go, history reads in
// sessionhistory.go, and wire handling in protocol.go and child_agents.go.
package grok

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	providerapi "github.com/rezoch340/any-aicli-remote/backend/internal/provider"
)

const ProviderID = "grok"

type GrokProvider struct {
	activeRoot     string
	archivedRoot   string
	executablePath string
	alwaysApprove  bool
	leader         bool
	historyPolicy  providerapi.HistoryPolicy
	renameMutex    sync.Mutex
}

type Config struct {
	SessionsDirectory string
	ExecutablePath    string
	AlwaysApprove     bool
	Leader            bool
	HistoryPolicy     providerapi.HistoryPolicy
}

func New(configuration Config) (*GrokProvider, error) {
	if operationError := configuration.HistoryPolicy.Validate(); operationError != nil {
		return nil, fmt.Errorf("validate history policy: %w", operationError)
	}
	activeRoot := strings.TrimSpace(configuration.SessionsDirectory)
	if activeRoot == "" {
		homeDirectory, operationError := os.UserHomeDir()
		if operationError != nil {
			return nil, fmt.Errorf("resolve home directory: %w", operationError)
		}
		activeRoot = filepath.Join(homeDirectory, ".grok", "sessions")
	}
	return &GrokProvider{activeRoot: activeRoot, archivedRoot: filepath.Join(filepath.Dir(activeRoot), "archived_sessions"), executablePath: strings.TrimSpace(configuration.ExecutablePath), alwaysApprove: configuration.AlwaysApprove, leader: configuration.Leader, historyPolicy: configuration.HistoryPolicy}, nil
}

func (providerInstance *GrokProvider) ID() string { return ProviderID }

func (providerInstance *GrokProvider) sessionRoots() []string {
	return []string{providerInstance.activeRoot, providerInstance.archivedRoot}
}

var _ providerapi.Provider = (*GrokProvider)(nil)
