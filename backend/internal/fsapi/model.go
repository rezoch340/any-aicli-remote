// Workspace filesystem model: limits, sentinel errors, and the JSON shapes
// returned to clients.

package fsapi

import (
	"encoding/json"
	"errors"
	"math"
)

type Policy struct {
	MaxReadBytes  int64
	MaxWriteBytes int64
	MaxListItems  int
}

func (policy Policy) Validate() error {
	if policy.MaxReadBytes <= 0 {
		return errors.New("filesystem maximum read bytes must be positive")
	}
	if policy.MaxReadBytes >= math.MaxInt64 {
		return errors.New("filesystem maximum read bytes must leave room for sentinel")
	}
	if policy.MaxListItems <= 0 {
		return errors.New("filesystem maximum list items must be positive")
	}
	if policy.MaxListItems >= int(^uint(0)>>1) {
		return errors.New("filesystem maximum list items must leave room for sentinel")
	}
	if policy.MaxWriteBytes <= 0 {
		return errors.New("filesystem maximum write bytes must be positive")
	}
	if policy.MaxWriteBytes >= math.MaxInt64 {
		return errors.New("filesystem maximum write bytes must leave room for sentinel")
	}
	return nil
}

var (
	PathRequiredError             = errors.New("path required")
	OutsideWorkspaceError         = errors.New("path outside workspace")
	WorkspaceChangedError         = errors.New("workspace root changed")
	NotDirectoryError             = errors.New("not a directory")
	NotFileError                  = errors.New("not a file")
	FileTooLargeError             = errors.New("file too large")
	ContentRequiredError          = errors.New("content required")
	ContentTooLargeError          = errors.New("content too large")
	DirectoryListingTooLargeError = errors.New("directory listing too large")
)

type RootInfo struct {
	Root   string `json:"root"`
	Exists bool   `json:"exists"`
}

type Item struct {
	Name             string `json:"name"`
	Path             string `json:"path"`
	RelativePath     string `json:"rel"`
	ModificationTime int64  `json:"mtime"`
	Size             int64  `json:"size,omitempty"`
	Text             bool   `json:"text,omitempty"`
	file             bool
}

func (service Item) MarshalJSON() ([]byte, error) {
	base := map[string]any{"name": service.Name, "path": service.Path, "rel": service.RelativePath, "mtime": service.ModificationTime}
	if service.file {
		base["size"] = service.Size
		base["text"] = service.Text
	}
	return json.Marshal(base)
}

type Listing struct {
	Path         string  `json:"path"`
	RelativePath string  `json:"rel"`
	Parent       *string `json:"parent"`
	Directories  []Item  `json:"dirs"`
	Files        []Item  `json:"files"`
	Root         string  `json:"root"`
}

type ReadResult struct {
	Path         string `json:"path"`
	RelativePath string `json:"rel,omitempty"`
	Name         string `json:"name,omitempty"`
	Text         bool   `json:"text"`
	Binary       bool   `json:"binary,omitempty"`
	Content      string `json:"content,omitempty"`
	Size         int64  `json:"size"`
}

type WriteResult struct {
	OK           bool   `json:"ok"`
	Path         string `json:"path"`
	RelativePath string `json:"rel"`
	Size         int64  `json:"size"`
}
