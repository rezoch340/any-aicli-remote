// Workspace file operations. Every operation runs against the pinned os.Root
// under a validated workspace, so a symbolic link cannot escape between
// validation and access.

package fsapi

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

var skippedDirectories = map[string]struct{}{
	".git": {}, "node_modules": {}, "__pycache__": {}, ".venv": {}, "venv": {},
	"dist": {}, "build": {}, ".next": {}, ".cache": {},
}

func (service *Service) List(raw string) (Listing, error) {
	service.mutex.RLock()
	defer service.mutex.RUnlock()
	if operationError := service.validateRootLocked(); operationError != nil {
		return Listing{}, operationError
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
	entries, operationError := file.ReadDir(service.policy.MaxListItems + 1)
	if operationError != nil && !errors.Is(operationError, io.EOF) {
		return Listing{}, operationError
	}
	if len(entries) > service.policy.MaxListItems {
		return Listing{}, DirectoryListingTooLargeError
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
	if operationError := service.validateRootLocked(); operationError != nil {
		return ReadResult{}, operationError
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
	if info.Size() > service.policy.MaxReadBytes {
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
	data, operationError := io.ReadAll(io.LimitReader(file, service.policy.MaxReadBytes+1))
	if operationError != nil {
		return ReadResult{}, operationError
	}
	if int64(len(data)) > service.policy.MaxReadBytes {
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
	if int64(len(data)) > service.policy.MaxWriteBytes {
		return WriteResult{}, ContentTooLargeError
	}
	service.mutex.RLock()
	defer service.mutex.RUnlock()
	if operationError := service.validateRootLocked(); operationError != nil {
		return WriteResult{}, operationError
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
	if operationError := service.validateRootLocked(); operationError != nil {
		return "", operationError
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

func displayRelativePath(relativePath string) string {
	if relativePath == "" || relativePath == "." {
		return "."
	}
	return filepath.ToSlash(relativePath)
}
