// Package provider defines the provider-neutral session catalog and protocol
// boundaries used by the daemon core.
package provider

import (
	"context"
	"errors"
	"sort"
	"sync"
)

var (
	ProviderNotFoundError  = errors.New("provider not found")
	SessionNotFoundError   = errors.New("session not found")
	SessionRequiredError   = errors.New("sessionId required")
	WorkspaceRequiredError = errors.New("cwd required")
)

type SessionMetadata struct {
	ProviderID       string `json:"providerId"`
	SessionID        string `json:"sessionId"`
	Title            string `json:"title,omitempty"`
	Summary          string `json:"summary,omitempty"`
	ProjectDirectory string `json:"projectDir,omitempty"`
	CreatedAt        int64  `json:"createdAt,omitempty"`
	LastActiveAt     int64  `json:"lastActiveAt,omitempty"`
	SourcePath       string `json:"sourcePath,omitempty"`
	ResumeCommand    string `json:"resumeCommand,omitempty"`
}

type Message struct {
	Role      string `json:"role"`
	Content   string `json:"content"`
	Timestamp int64  `json:"ts,omitempty"`
}

type Event map[string]any
type HistoryMetadata map[string]any

type HistoryQuery struct {
	SessionID   string
	Live        bool
	Limit       int
	SinceBytes  int64
	BeforeBytes *int64
	MaxBytes    int64
	ChatOnly    bool
}

type HistoryPage struct {
	Session  SessionMetadata
	Events   []Event
	Metadata HistoryMetadata
}

type RenameResult struct {
	SessionID     string
	Title         string
	PreviousTitle string
	SourcePath    string
}

// SkillRootKind identifies the metadata convention used within a declared
// provider-owned discovery root.
type SkillRootKind string

const (
	SkillRootKindSkill   SkillRootKind = "skill"
	SkillRootKindCommand SkillRootKind = "command"
)

// SkillRootSource describes the origin shown to clients for entries discovered
// below a declared root. The scanner never infers an origin from path segments.
type SkillRootSource string

const (
	SkillRootSourceBundled     SkillRootSource = "bundled"
	SkillRootSourceUser        SkillRootSource = "user"
	SkillRootSourcePlugin      SkillRootSource = "plugin"
	SkillRootSourceCommand     SkillRootSource = "command"
	SkillRootSourceMarketplace SkillRootSource = "marketplace"
)

// SkillRoot is a complete provider declaration for one discovery root.
type SkillRoot struct {
	Kind   SkillRootKind
	Source SkillRootSource
	Path   string
}

// SkillRoots contains provider-owned discovery declarations. Kind is part of
// every entry, so there is no second grouping that can disagree with it.
type SkillRoots struct {
	Roots []SkillRoot
}

// SkillRootProvider is implemented by adapters that expose skills or slash
// commands stored in provider-specific locations.
type SkillRootProvider interface {
	SkillRoots(workingDirectory string) SkillRoots
}

// MergeSessionMetadata overlays authoritative active bindings onto richer
// catalog metadata and returns one deterministically ordered entry per
// provider/session pair.
func MergeSessionMetadata(catalogSessions, activeSessions []SessionMetadata) []SessionMetadata {
	merged := make(map[string]SessionMetadata, len(catalogSessions)+len(activeSessions))
	for _, metadata := range activeSessions {
		if metadata.ProviderID == "" || metadata.SessionID == "" {
			continue
		}
		merged[metadata.ProviderID+"\x00"+metadata.SessionID] = metadata
	}
	for _, metadata := range catalogSessions {
		if metadata.ProviderID == "" || metadata.SessionID == "" {
			continue
		}
		cacheKey := metadata.ProviderID + "\x00" + metadata.SessionID
		if active, present := merged[cacheKey]; present {
			metadata.ProjectDirectory = active.ProjectDirectory
			if active.LastActiveAt > metadata.LastActiveAt {
				metadata.LastActiveAt = active.LastActiveAt
			}
		}
		merged[cacheKey] = metadata
	}
	result := make([]SessionMetadata, 0, len(merged))
	for _, metadata := range merged {
		result = append(result, metadata)
	}
	SortSessions(result)
	return result
}

func SortSessions(sessions []SessionMetadata) {
	sort.SliceStable(sessions, func(firstIndex, secondIndex int) bool {
		firstTimestamp := sessions[firstIndex].LastActiveAt
		if firstTimestamp == 0 {
			firstTimestamp = sessions[firstIndex].CreatedAt
		}
		secondTimestamp := sessions[secondIndex].LastActiveAt
		if secondTimestamp == 0 {
			secondTimestamp = sessions[secondIndex].CreatedAt
		}
		if firstTimestamp == secondTimestamp {
			if sessions[firstIndex].ProviderID == sessions[secondIndex].ProviderID {
				return sessions[firstIndex].SessionID < sessions[secondIndex].SessionID
			}
			return sessions[firstIndex].ProviderID < sessions[secondIndex].ProviderID
		}
		return firstTimestamp > secondTimestamp
	})
}

type Provider interface {
	ID() string
	ScanSessions(context.Context) ([]SessionMetadata, error)
	ResolveSession(context.Context, string) (SessionMetadata, error)
	LoadMessages(context.Context, string) ([]Message, error)
	ReadHistory(context.Context, HistoryQuery) (HistoryPage, error)
	ReadSignals(context.Context, string) (map[string]any, error)
	RenameSession(context.Context, string, string) (RenameResult, error)
}

type Registry struct {
	accessMutex sync.RWMutex
	providers   map[string]Provider
}

func NewRegistry(providers ...Provider) *Registry {
	registry := &Registry{providers: make(map[string]Provider)}
	for _, providerInstance := range providers {
		registry.Register(providerInstance)
	}
	return registry
}

func (registry *Registry) Register(providerInstance Provider) {
	if registry == nil || providerInstance == nil || providerInstance.ID() == "" {
		return
	}
	registry.accessMutex.Lock()
	registry.providers[providerInstance.ID()] = providerInstance
	registry.accessMutex.Unlock()
}

func (registry *Registry) Provider(providerID string) (Provider, error) {
	if registry == nil {
		return nil, ProviderNotFoundError
	}
	registry.accessMutex.RLock()
	providerInstance := registry.providers[providerID]
	registry.accessMutex.RUnlock()
	if providerInstance == nil {
		return nil, ProviderNotFoundError
	}
	return providerInstance, nil
}

func (registry *Registry) ScanSessions(operationContext context.Context, providerID string) ([]SessionMetadata, error) {
	if providerID != "" {
		providerInstance, operationError := registry.Provider(providerID)
		if operationError != nil {
			return nil, operationError
		}
		return providerInstance.ScanSessions(operationContext)
	}
	registry.accessMutex.RLock()
	providerInstances := make([]Provider, 0, len(registry.providers))
	for _, providerInstance := range registry.providers {
		providerInstances = append(providerInstances, providerInstance)
	}
	registry.accessMutex.RUnlock()
	sessions := make([]SessionMetadata, 0)
	for _, providerInstance := range providerInstances {
		providerSessions, operationError := providerInstance.ScanSessions(operationContext)
		if operationError != nil {
			return nil, operationError
		}
		sessions = append(sessions, providerSessions...)
	}
	SortSessions(sessions)
	return sessions, nil
}
