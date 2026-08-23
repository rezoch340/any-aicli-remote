// Git result model: sentinel errors, tunable limits, and the JSON shapes
// returned to clients.

package gitapi

import (
	"errors"
	"math"
	"time"
)

var (
	GitNotFoundError          = errors.New("git not found")
	WorkspaceUnavailableError = errors.New("workspace unavailable")
	PathOutsideWorkspaceError = errors.New("path outside workspace")
)

type Policy struct {
	CommandTimeout        time.Duration
	DiffTimeout           time.Duration
	DirtyFileLimit        int
	DiffRuneLimit         int
	LogDefaultLimit       int
	LogMaxLimit           int
	ContextFileReadBytes  int64
	ContextPreviewRunes   int
	CommandOutputMaxBytes int64
}

func (policy Policy) Validate() error {
	switch {
	case policy.CommandTimeout <= 0:
		return errors.New("git command timeout must be positive")
	case policy.DiffTimeout <= 0:
		return errors.New("git diff timeout must be positive")
	case policy.DirtyFileLimit <= 0:
		return errors.New("git dirty file limit must be positive")
	case policy.DiffRuneLimit <= 0:
		return errors.New("git diff rune limit must be positive")
	case policy.LogDefaultLimit <= 0:
		return errors.New("git log default limit must be positive")
	case policy.LogMaxLimit < policy.LogDefaultLimit:
		return errors.New("git log maximum limit must not be below default")
	case policy.ContextFileReadBytes <= 0:
		return errors.New("git context file read bytes must be positive")
	case policy.ContextFileReadBytes >= math.MaxInt64:
		return errors.New("git context file read bytes must leave room for sentinel")
	case policy.ContextPreviewRunes <= 0:
		return errors.New("git context preview runes must be positive")
	case policy.CommandOutputMaxBytes <= 0:
		return errors.New("git command output max bytes must be positive")
	case policy.CommandOutputMaxBytes >= math.MaxInt64:
		return errors.New("git command output max bytes must leave room for sentinel")
	}
	return nil
}

type DirtyFile struct {
	Code string `json:"code"`
	Path string `json:"path"`
}

type StatusResult struct {
	OK         bool        `json:"ok"`
	Error      string      `json:"error,omitempty"`
	Git        bool        `json:"git"`
	Root       string      `json:"root,omitempty"`
	Branch     string      `json:"branch,omitempty"`
	CommitHash string      `json:"sha,omitempty"`
	Ahead      int         `json:"ahead"`
	Behind     int         `json:"behind"`
	Dirty      int         `json:"dirty"`
	Files      []DirtyFile `json:"files"`
	Head       string      `json:"head,omitempty"`
}

type DiffResult struct {
	OK     bool   `json:"ok"`
	Path   string `json:"path"`
	Staged bool   `json:"staged"`
	Diff   string `json:"diff"`
	Code   int    `json:"code"`
}

type Commit struct {
	Hash    string `json:"hash"`
	Date    string `json:"date"`
	Subject string `json:"subject"`
}

type LogResult struct {
	OK      bool     `json:"ok"`
	Commits []Commit `json:"commits"`
}

type ContextFile struct {
	Name         string `json:"name"`
	RelativePath string `json:"rel"`
	Size         int64  `json:"size"`
	Preview      string `json:"preview"`
}

type ProjectContext struct {
	OK     bool          `json:"ok"`
	Root   string        `json:"root"`
	Branch *string       `json:"branch"`
	Files  []ContextFile `json:"files"`
}
