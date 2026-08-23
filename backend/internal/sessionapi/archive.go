// Archived session list. The archive flag is daemon-owned state persisted next
// to the other runtime data, not provider state.

package sessionapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/rezoch340/any-aicli-remote/backend/internal/atomicfile"
	"github.com/rezoch340/any-aicli-remote/backend/internal/compat"
)

type ArchivedResult struct {
	OK    bool     `json:"ok"`
	IDs   []string `json:"ids"`
	Count int      `json:"count"`
	Path  string   `json:"path,omitempty"`
}

type SetArchivedRequest struct {
	IDs       []string `json:"ids"`
	ID        string   `json:"id"`
	SessionID string   `json:"sessionId"`
	Archived  *bool    `json:"archived"`
}

func (service *Service) Archived() (ArchivedResult, error) {
	service.mutex.Lock()
	defer service.mutex.Unlock()
	sessionIDs, path, operationError := service.loadArchivedLocked()
	if operationError != nil {
		return ArchivedResult{}, operationError
	}
	return ArchivedResult{OK: true, IDs: sessionIDs, Count: len(sessionIDs), Path: path}, nil
}

func (service *Service) SetArchived(request SetArchivedRequest) (ArchivedResult, error) {
	service.mutex.Lock()
	defer service.mutex.Unlock()
	sessionIDs, _, operationError := service.loadArchivedLocked()
	if operationError != nil {
		return ArchivedResult{}, operationError
	}
	if request.IDs != nil {
		sessionIDs = cleanIDs(request.IDs)
	} else {
		sessionID := strings.TrimSpace(firstNonEmpty(request.ID, request.SessionID))
		if sessionID == "" {
			return ArchivedResult{}, fmt.Errorf("%w: ids[] or id required", BadRequestError)
		}
		archivedSet := make(map[string]struct{}, len(sessionIDs)+1)
		for _, identifier := range sessionIDs {
			archivedSet[identifier] = struct{}{}
		}
		wantArchived := request.Archived == nil
		if request.Archived == nil {
			_, alreadyArchived := archivedSet[sessionID]
			wantArchived = !alreadyArchived
		} else {
			wantArchived = *request.Archived
		}
		if wantArchived {
			archivedSet[sessionID] = struct{}{}
		} else {
			delete(archivedSet, sessionID)
		}
		sessionIDs = sessionIDs[:0]
		for identifier := range archivedSet {
			sessionIDs = append(sessionIDs, identifier)
		}
		sort.Strings(sessionIDs)
	}
	sessionIDs, operationError = service.saveArchivedLocked(sessionIDs)
	if operationError != nil {
		return ArchivedResult{}, operationError
	}
	return ArchivedResult{OK: true, IDs: sessionIDs, Count: len(sessionIDs)}, nil
}

func (service *Service) archivedPath() string {
	return filepath.Join(service.DataDirectory, "archived_sessions.json")
}

func (service *Service) loadArchivedLocked() ([]string, string, error) {
	path := service.archivedPath()
	data, operationError := os.ReadFile(path)
	if errors.Is(operationError, os.ErrNotExist) {
		return []string{}, path, nil
	}
	if operationError != nil {
		return nil, path, operationError
	}
	sessionIDs, _, operationError := compat.ParseArchivedSessionIDs(data)
	if operationError != nil {
		return nil, path, operationError
	}
	return cleanIDs(sessionIDs), path, nil
}

func (service *Service) saveArchivedLocked(sessionIDs []string) ([]string, error) {
	sessionIDs = cleanIDs(sessionIDs)
	data, operationError := json.MarshalIndent(sessionIDs, "", "  ")
	if operationError != nil {
		return nil, operationError
	}
	if operationError := atomicfile.WritePrivate(service.archivedPath(), append(data, '\n')); operationError != nil {
		return nil, operationError
	}
	return sessionIDs, nil
}

func cleanIDs(sessionIDs []string) []string {
	seen := make(map[string]struct{}, len(sessionIDs))
	cleaned := make([]string, 0, len(sessionIDs))
	for _, rawSessionID := range sessionIDs {
		sessionID := strings.TrimSpace(rawSessionID)
		if sessionID == "" {
			continue
		}
		if _, present := seen[sessionID]; present {
			continue
		}
		seen[sessionID] = struct{}{}
		cleaned = append(cleaned, sessionID)
	}
	sort.Strings(cleaned)
	return cleaned
}
