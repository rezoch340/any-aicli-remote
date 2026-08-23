// Package sessionapi exposes provider-neutral session management operations.
//
// This file owns the service and provider lookup. Reads live in reads.go, the
// archived list in archive.go, rename delegation in rename.go, and payload
// coercion in values.go.
package sessionapi

import (
	"errors"
	"strings"
	"sync"

	providerapi "github.com/rezoch340/any-aicli-remote/backend/internal/provider"
)

var (
	SessionRequiredError = errors.New("sessionId required")
	NotFoundError        = errors.New("session not found")
	TitleRequiredError   = errors.New("title required")
	BadRequestError      = errors.New("bad request")
)

type Service struct {
	Providers         *providerapi.Registry
	DefaultProviderID string
	DataDirectory     string
	historyPolicy     providerapi.HistoryPolicy

	mutex sync.Mutex
}

func New(providers *providerapi.Registry, defaultProviderID, dataDirectory string, historyPolicy providerapi.HistoryPolicy) (*Service, error) {
	if providers == nil {
		providers = providerapi.NewRegistry()
	}
	if operationError := historyPolicy.Validate(); operationError != nil {
		return nil, operationError
	}
	return &Service{Providers: providers, DefaultProviderID: defaultProviderID, DataDirectory: dataDirectory, historyPolicy: historyPolicy}, nil
}

func (service *Service) HistoryPolicy() providerapi.HistoryPolicy { return service.historyPolicy }

func (service *Service) provider(providerID string) (providerapi.Provider, error) {
	providerID = strings.TrimSpace(providerID)
	if providerID == "" {
		providerID = service.DefaultProviderID
	}
	return service.Providers.Provider(providerID)
}
