// Package factory is the application composition boundary for provider adapters.
// The HTTP server depends only on the generic components returned here.
package factory

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	providerapi "github.com/rezoch340/any-aicli-remote/backend/internal/provider"
	"github.com/rezoch340/any-aicli-remote/backend/internal/provider/grok"
	"github.com/rezoch340/any-aicli-remote/backend/internal/voice"
)

const (
	DefaultProviderID       = grok.ProviderID
	SessionsDirectoryOption = "sessions-directory"
	AlwaysApproveOption     = "always-approve"
	LeaderOption            = "leader"
)

type Configuration struct {
	ProviderID     string
	ExecutablePath string
	Options        map[string]string
}

type Components struct {
	Catalog    providerapi.Provider
	Protocol   providerapi.ProtocolAdapter
	SkillRoots providerapi.SkillRootProvider
	Voice      voice.Service
}

func New(configuration Configuration) (Components, error) {
	providerID := strings.TrimSpace(configuration.ProviderID)
	if providerID == "" {
		providerID = DefaultProviderID
	}
	switch providerID {
	case grok.ProviderID:
		components, operationError := newGrok(configuration)
		if operationError != nil {
			return Components{}, operationError
		}
		if operationError := validateComponents(components); operationError != nil {
			return Components{}, operationError
		}
		return components, nil
	default:
		return Components{}, fmt.Errorf("unsupported provider: %s", providerID)
	}
}

func newGrok(configuration Configuration) (Components, error) {
	alwaysApprove, operationError := booleanOption(configuration.Options, AlwaysApproveOption, false)
	if operationError != nil {
		return Components{}, operationError
	}
	leader, operationError := booleanOption(configuration.Options, LeaderOption, false)
	if operationError != nil {
		return Components{}, operationError
	}
	sessionsDirectory := strings.TrimSpace(configuration.Options[SessionsDirectoryOption])
	if sessionsDirectory == "" {
		homeDirectory, homeError := os.UserHomeDir()
		if homeError != nil {
			return Components{}, fmt.Errorf("resolve provider sessions directory: %w", homeError)
		}
		sessionsDirectory = filepath.Join(homeDirectory, ".grok", "sessions")
	}
	providerInstance := grok.New(grok.Config{
		SessionsDirectory: sessionsDirectory,
		ExecutablePath:    configuration.ExecutablePath,
		AlwaysApprove:     alwaysApprove,
		Leader:            leader,
	})
	return Components{
		Catalog:    providerInstance,
		Protocol:   providerInstance,
		SkillRoots: providerInstance,
		Voice:      grok.NewVoiceFromEnvironment(),
	}, nil
}

func booleanOption(options map[string]string, name string, fallback bool) (bool, error) {
	value := strings.TrimSpace(options[name])
	if value == "" {
		return fallback, nil
	}
	parsed, operationError := strconv.ParseBool(value)
	if operationError != nil {
		return false, fmt.Errorf("provider option %q must be boolean: %w", name, operationError)
	}
	return parsed, nil
}

func validateComponents(components Components) error {
	if components.Catalog == nil || components.Protocol == nil || components.SkillRoots == nil || components.Voice == nil {
		return errors.New("provider composition is incomplete")
	}
	if components.Catalog.ID() != components.Protocol.ID() {
		return errors.New("provider catalog and protocol identifiers differ")
	}
	return nil
}
