package fsapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"unicode/utf8"
)

const (
	MaxRead  int64 = 2_000_000
	MaxWrite int   = 4_000_000
)

var (
	PathRequiredError     = errors.New("path required")
	OutsideWorkspaceError = errors.New("path outside workspace")
	NotDirectoryError     = errors.New("not a directory")
	NotFileError          = errors.New("not a file")
	FileTooLargeError     = errors.New("file too large")
	ContentRequiredError  = errors.New("content required")
	ContentTooLargeError  = errors.New("content too large")
)

var skippedDirectories = map[string]struct{}{
	".git": {}, "node_modules": {}, "__pycache__": {}, ".venv": {}, "venv": {},
	"dist": {}, "build": {}, ".next": {}, ".cache": {},
}

var textExtensions = map[string]struct{}{
	".py": {}, ".js": {}, ".ts": {}, ".tsx": {}, ".jsx": {}, ".json": {},
	".md": {}, ".txt": {}, ".css": {}, ".html": {}, ".htm": {}, ".xml": {},
	".yml": {}, ".yaml": {}, ".toml": {}, ".ini": {}, ".cfg": {}, ".env": {},
	".sh": {}, ".ps1": {}, ".bat": {}, ".cmd": {}, ".rs": {}, ".go": {},
	".java": {}, ".c": {}, ".h": {}, ".cpp": {}, ".hpp": {}, ".cs": {},
	".rb": {}, ".php": {}, ".sql": {}, ".r": {}, ".swift": {}, ".kt": {},
	".vue": {}, ".svelte": {}, ".scss": {}, ".less": {}, ".svg": {},
	".gitignore": {}, ".dockerfile": {}, ".cmake": {}, ".gradle": {}, ".log": {},
	".diff": {}, ".patch": {}, ".csv": {},
}

type Service struct {
	mutex          sync.RWMutex
	root           string
	rootFilesystem *os.Root
}

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

func New(root string) (*Service, error) {
	service := &Service{}
	if _, operationError := service.SetRoot(root); operationError != nil {
		return nil, operationError
	}
	return service, nil
}

func (service *Service) Close() error {
	service.mutex.Lock()
	defer service.mutex.Unlock()
	if service.rootFilesystem == nil {
		return nil
	}
	operationError := service.rootFilesystem.Close()
	service.rootFilesystem = nil
	return operationError
}

func (service *Service) Root() string {
	service.mutex.RLock()
	defer service.mutex.RUnlock()
	return service.root
}

func (service *Service) Info() RootInfo {
	service.mutex.RLock()
	defer service.mutex.RUnlock()
	exists := false
	if service.rootFilesystem != nil {
		if info, operationError := service.rootFilesystem.Stat("."); operationError == nil {
			exists = info.IsDir()
		}
	}
	return RootInfo{Root: service.root, Exists: exists}
}

func (service *Service) SetRoot(raw string) (RootInfo, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return RootInfo{}, PathRequiredError
	}
	absolutePath, operationError := filepath.Abs(filepath.Clean(raw))
	if operationError != nil {
		return RootInfo{}, fmt.Errorf("resolve workspace: %w", operationError)
	}
	info, operationError := os.Stat(absolutePath)
	if operationError != nil {
		return RootInfo{}, fmt.Errorf("open workspace: %w", operationError)
	}
	if !info.IsDir() {
		return RootInfo{}, NotDirectoryError
	}
	root, operationError := os.OpenRoot(absolutePath)
	if operationError != nil {
		return RootInfo{}, fmt.Errorf("open workspace: %w", operationError)
	}

	service.mutex.Lock()
	previous := service.rootFilesystem
	service.root = absolutePath
	service.rootFilesystem = root
	service.mutex.Unlock()
	if previous != nil {
		_ = previous.Close()
	}
	return RootInfo{Root: absolutePath, Exists: true}, nil
}

// Resolve converts a client path into an absolute workspace path. File operations
// still use os.Root, so a symlink cannot escape the workspace between validation
// and access.
func (service *Service) Resolve(raw string) (string, error) {
	service.mutex.RLock()
	defer service.mutex.RUnlock()
	relativePath, operationError := relativePath(service.root, raw)
	if operationError != nil {
		return "", operationError
	}
	if service.rootFilesystem == nil {
		return "", os.ErrClosed
	}
	if _, operationError := service.rootFilesystem.Stat(relativePath); operationError != nil && !errors.Is(operationError, os.ErrNotExist) {
		return "", normalizeRootError(operationError)
	}
	return absolutePath(service.root, relativePath), nil
}

func (service *Service) List(raw string) (Listing, error) {
	service.mutex.RLock()
	defer service.mutex.RUnlock()
	if service.rootFilesystem == nil {
		return Listing{}, os.ErrClosed
	}
	relativePath, operationError := relativePath(service.root, raw)
	if operationError != nil {
		return Listing{}, operationError
	}
	file, operationError := service.rootFilesystem.Open(relativePath)
	if operationError != nil {
		return Listing{}, normalizeRootError(operationError)
	}
	defer file.Close()
	info, operationError := file.Stat()
	if operationError != nil {
		return Listing{}, operationError
	}
	if !info.IsDir() {
		return Listing{}, NotDirectoryError
	}
	entries, operationError := file.ReadDir(-1)
	if operationError != nil {
		return Listing{}, operationError
	}
	sort.Slice(entries, func(leftIndex, rightIndex int) bool {
		leftItem, ierr := entries[leftIndex].Info()
		rightItem, jerr := entries[rightIndex].Info()
		identifier := ierr == nil && leftItem.IsDir()
		rightIsDirectory := jerr == nil && rightItem.IsDir()
		if identifier != rightIsDirectory {
			return identifier
		}
		return strings.ToLower(entries[leftIndex].Name()) < strings.ToLower(entries[rightIndex].Name())
	})

	result := Listing{
		Path:         absolutePath(service.root, relativePath),
		RelativePath: displayRelativePath(relativePath),
		Directories:  []Item{},
		Files:        []Item{},
		Root:         service.root,
	}
	if relativePath != "." {
		parent := filepath.Dir(relativePath)
		value := displayRelativePath(parent)
		result.Parent = &value
	}
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, ".") && name != ".env" && name != ".gitignore" {
			continue
		}
		childRelativePath := filepath.Join(relativePath, name)
		childInfo, statError := service.rootFilesystem.Stat(childRelativePath)
		if statError != nil { // Includes symlinks that leave the workspace.
			continue
		}
		if childInfo.IsDir() {
			if _, skip := skippedDirectories[name]; skip {
				continue
			}
		}
		item := Item{
			Name:             name,
			Path:             absolutePath(service.root, childRelativePath),
			RelativePath:     filepath.ToSlash(childRelativePath),
			ModificationTime: childInfo.ModTime().Unix(),
		}
		if childInfo.IsDir() {
			result.Directories = append(result.Directories, item)
		} else {
			item.Size = childInfo.Size()
			item.Text = IsTextPath(name)
			item.file = true
			result.Files = append(result.Files, item)
		}
	}
	return result, nil
}

func (service *Service) Read(raw string) (ReadResult, error) {
	if strings.TrimSpace(raw) == "" {
		return ReadResult{}, PathRequiredError
	}
	service.mutex.RLock()
	defer service.mutex.RUnlock()
	if service.rootFilesystem == nil {
		return ReadResult{}, os.ErrClosed
	}
	relativePath, operationError := relativePath(service.root, raw)
	if operationError != nil {
		return ReadResult{}, operationError
	}
	info, operationError := service.rootFilesystem.Stat(relativePath)
	if operationError != nil {
		if errors.Is(operationError, os.ErrNotExist) {
			return ReadResult{}, NotFileError
		}
		return ReadResult{}, normalizeRootError(operationError)
	}
	if !info.Mode().IsRegular() {
		return ReadResult{}, NotFileError
	}
	if info.Size() > MaxRead {
		return ReadResult{}, FileTooLargeError
	}
	result := ReadResult{
		Path:         absolutePath(service.root, relativePath),
		RelativePath: filepath.ToSlash(relativePath),
		Name:         filepath.Base(relativePath),
		Size:         info.Size(),
	}
	if !IsTextPath(filepath.Base(relativePath)) {
		result.Binary = true
		return result, nil
	}
	file, operationError := service.rootFilesystem.Open(relativePath)
	if operationError != nil {
		return ReadResult{}, normalizeRootError(operationError)
	}
	defer file.Close()
	data, operationError := io.ReadAll(io.LimitReader(file, MaxRead+1))
	if operationError != nil {
		return ReadResult{}, operationError
	}
	if int64(len(data)) > MaxRead {
		return ReadResult{}, FileTooLargeError
	}
	result.Text = true
	result.Content = decodeText(data)
	result.Size = int64(len(data))
	return result, nil
}

func (service *Service) Write(raw, content string) (WriteResult, error) {
	if strings.TrimSpace(raw) == "" {
		return WriteResult{}, PathRequiredError
	}
	data := []byte(content)
	if len(data) > MaxWrite {
		return WriteResult{}, ContentTooLargeError
	}
	service.mutex.RLock()
	defer service.mutex.RUnlock()
	if service.rootFilesystem == nil {
		return WriteResult{}, os.ErrClosed
	}
	relativePath, operationError := relativePath(service.root, raw)
	if operationError != nil {
		return WriteResult{}, operationError
	}
	if relativePath == "." {
		return WriteResult{}, NotFileError
	}
	if info, statError := service.rootFilesystem.Stat(relativePath); statError == nil && info.IsDir() {
		return WriteResult{}, NotFileError
	} else if statError != nil && !errors.Is(statError, os.ErrNotExist) {
		return WriteResult{}, normalizeRootError(statError)
	}
	if parent := filepath.Dir(relativePath); parent != "." {
		if operationError := service.rootFilesystem.MkdirAll(parent, 0o755); operationError != nil {
			return WriteResult{}, normalizeRootError(operationError)
		}
	}
	if operationError := service.rootFilesystem.WriteFile(relativePath, data, 0o644); operationError != nil {
		return WriteResult{}, normalizeRootError(operationError)
	}
	info, operationError := service.rootFilesystem.Stat(relativePath)
	if operationError != nil {
		return WriteResult{}, operationError
	}
	return WriteResult{
		OK:           true,
		Path:         absolutePath(service.root, relativePath),
		RelativePath: filepath.ToSlash(relativePath),
		Size:         info.Size(),
	}, nil
}

func (service *Service) Mkdir(raw string) (string, error) {
	if strings.TrimSpace(raw) == "" {
		return "", PathRequiredError
	}
	service.mutex.RLock()
	defer service.mutex.RUnlock()
	if service.rootFilesystem == nil {
		return "", os.ErrClosed
	}
	relativePath, operationError := relativePath(service.root, raw)
	if operationError != nil {
		return "", operationError
	}
	if operationError := service.rootFilesystem.MkdirAll(relativePath, 0o755); operationError != nil {
		return "", normalizeRootError(operationError)
	}
	return absolutePath(service.root, relativePath), nil
}

func IsTextPath(name string) bool {
	lower := strings.ToLower(filepath.Base(name))
	if _, valid := textExtensions[strings.ToLower(filepath.Ext(lower))]; valid {
		return true
	}
	switch lower {
	case "dockerfile", "makefile", "license", "readme":
		return true
	}
	return filepath.Ext(lower) == ""
}

func relativePath(root, raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "." || raw == string(filepath.Separator) {
		return ".", nil
	}
	var relativePath string
	if filepath.IsAbs(raw) {
		var operationError error
		relativePath, operationError = filepath.Rel(root, filepath.Clean(raw))
		if operationError != nil {
			return "", OutsideWorkspaceError
		}
	} else {
		relativePath = filepath.Clean(raw)
	}
	if relativePath == "." {
		return relativePath, nil
	}
	if relativePath == ".." || strings.HasPrefix(relativePath, ".."+string(filepath.Separator)) || filepath.IsAbs(relativePath) {
		return "", OutsideWorkspaceError
	}
	return relativePath, nil
}

func displayRelativePath(relativePath string) string {
	if relativePath == "" || relativePath == "." {
		return "."
	}
	return filepath.ToSlash(relativePath)
}

func absolutePath(root, relativePath string) string {
	if relativePath == "." || relativePath == "" {
		return root
	}
	return filepath.Join(root, relativePath)
}

func normalizeRootError(operationError error) error {
	if operationError == nil {
		return nil
	}
	lower := strings.ToLower(operationError.Error())
	if strings.Contains(lower, "escapes") || strings.Contains(lower, "outside") {
		return fmt.Errorf("%w: %v", OutsideWorkspaceError, operationError)
	}
	return operationError
}

func decodeText(data []byte) string {
	if len(data) >= 3 && data[0] == 0xef && data[1] == 0xbb && data[2] == 0xbf {
		data = data[3:]
	}
	if utf8.Valid(data) {
		return string(data)
	}
	runes := make([]rune, len(data))
	for byteIndex, byteValue := range data {
		runes[byteIndex] = rune(byteValue)
	}
	return string(runes)
}
