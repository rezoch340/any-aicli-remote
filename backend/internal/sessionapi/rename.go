// Session rename delegated to the owning provider adapter.

package sessionapi

import (
	"context"
	"errors"
	"path/filepath"
	"strings"

	providerapi "github.com/rezoch340/any-aicli-remote/backend/internal/provider"
)

type RenameRequest struct {
	ProviderID       string `json:"providerId"`
	SessionID        string `json:"sessionId"`
	ID               string `json:"id"`
	Title            string `json:"title"`
	Name             string `json:"name"`
	WorkingDirectory string `json:"cwd,omitempty"`
}

type RenameResult struct {
	OK         bool   `json:"ok"`
	Error      string `json:"error,omitempty"`
	ProviderID string `json:"providerId"`
	SessionID  string `json:"sessionId"`
	Title      string `json:"title,omitempty"`
	Previous   string `json:"previous"`
	Directory  string `json:"dir,omitempty"`
}

func (service *Service) Rename(operationContext context.Context, request RenameRequest) (RenameResult, error) {
	sessionID := strings.TrimSpace(firstNonEmpty(request.SessionID, request.ID))
	if sessionID == "" {
		return RenameResult{}, SessionRequiredError
	}
	title := strings.TrimSpace(firstNonEmpty(request.Title, request.Name))
	if title == "" {
		return RenameResult{}, TitleRequiredError
	}
	providerInstance, operationError := service.provider(request.ProviderID)
	if operationError != nil {
		return RenameResult{}, operationError
	}
	providerResult, operationError := providerInstance.RenameSession(operationContext, sessionID, truncateRunes(title, service.historyPolicy.RenameTitleMaxRunes))
	if operationError != nil {
		result := RenameResult{OK: false, Error: NotFoundError.Error(), ProviderID: providerInstance.ID(), SessionID: sessionID}
		if errors.Is(operationError, providerapi.SessionNotFoundError) {
			return result, NotFoundError
		}
		return result, operationError
	}
	directory := ""
	if providerResult.SourcePath != "" {
		directory = filepath.Dir(providerResult.SourcePath)
	}
	return RenameResult{
		OK: true, ProviderID: providerInstance.ID(), SessionID: providerResult.SessionID,
		Title: providerResult.Title, Previous: providerResult.PreviousTitle, Directory: directory,
	}, nil
}
