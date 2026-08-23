// Ownership state. The state file is what proves a listener belongs to this
// daemon, so an unrelated process on the same port is never signalled.

package process

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"

	"github.com/rezoch340/any-aicli-remote/backend/internal/atomicfile"
)

func secretHash(secret string) string {
	if secret == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(sum[:])[:secretHashPrefixCharacters]
}

func (manager *Manager) LoadState() (*State, error) {
	configuration, errorValue := manager.configuration()
	if errorValue != nil {
		return nil, errorValue
	}
	rawData, errorValue := os.ReadFile(configuration.StatePath)
	if errors.Is(errorValue, os.ErrNotExist) {
		return nil, nil
	}
	if errorValue != nil {
		return nil, errorValue
	}
	var state State
	if errorValue := json.Unmarshal(rawData, &state); errorValue != nil {
		return nil, errorValue
	}
	return &state, nil
}

func (manager *Manager) saveState(state State) error {
	configuration, errorValue := manager.configuration()
	if errorValue != nil {
		return errorValue
	}
	rawData, errorValue := json.MarshalIndent(state, "", "  ")
	if errorValue != nil {
		return errorValue
	}
	return atomicfile.WritePrivate(configuration.StatePath, append(rawData, '\n'))
}

func (manager *Manager) removeState() {
	configuration, errorValue := manager.configuration()
	if errorValue == nil {
		_ = os.Remove(configuration.StatePath)
	}
}
