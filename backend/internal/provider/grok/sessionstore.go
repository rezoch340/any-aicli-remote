// Session storage access for the Grok adapter. Every read and write of a
// session directory is routed through here so root containment, symbolic-link
// rejection, and metadata size limits have one canonical implementation.

package grok

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"

	"github.com/rezoch340/any-aicli-remote/backend/internal/fsapi"
)

const (
	temporaryFileCreationAttempts = 16
	randomSuffixBytes             = 16
)

var MetadataFileTooLargeError = errors.New("metadata file too large")

type sessionAccess struct {
	root        *os.Root
	sourcePath  string
	summaryData map[string]any
}

func (access *sessionAccess) Close() error {
	if access == nil || access.root == nil {
		return nil
	}
	return access.root.Close()
}

func (providerInstance *GrokProvider) validateSource(sessionID, sourcePath string) (string, error) {
	access, operationError := providerInstance.openSessionSource(sessionID, sourcePath)
	if operationError != nil {
		return "", operationError
	}
	defer access.Close()
	return access.sourcePath, nil
}

func (providerInstance *GrokProvider) openSessionSource(expectedSessionID, sourcePath string) (*sessionAccess, error) {
	sourceAbsolute, operationError := filepath.Abs(filepath.Clean(sourcePath))
	if operationError != nil {
		return nil, operationError
	}
	selectedRoot := ""
	selectedMatch := ""
	selectedRelative := ""
	for _, configuredRoot := range providerInstance.sessionRoots() {
		rootAbsolute, absoluteError := filepath.Abs(filepath.Clean(configuredRoot))
		if absoluteError != nil {
			continue
		}
		canonicalRoot, canonicalError := filepath.EvalSymlinks(rootAbsolute)
		if canonicalError != nil {
			continue
		}
		for _, matchRoot := range []string{rootAbsolute, canonicalRoot} {
			relativeSource, relativeError := filepath.Rel(matchRoot, sourceAbsolute)
			if relativeError != nil || relativeSource == ".." || strings.HasPrefix(relativeSource, ".."+string(filepath.Separator)) || filepath.IsAbs(relativeSource) {
				continue
			}
			if selectedRoot == "" || len(matchRoot) > len(selectedMatch) {
				selectedRoot = canonicalRoot
				selectedMatch = matchRoot
				selectedRelative = relativeSource
			}
		}
	}
	if selectedRoot == "" {
		return nil, errors.New("path is outside provider roots")
	}
	if filepath.Base(selectedRelative) != "summary.json" {
		return nil, errors.New("unexpected Grok session source")
	}
	sessionRelative := filepath.Dir(selectedRelative)
	if sessionRelative == "." {
		return nil, errors.New("unexpected Grok session directory")
	}

	providerRootIdentity, operationError := fsapi.PinRoot(selectedRoot)
	if operationError != nil {
		return nil, operationError
	}
	providerRoot, operationError := providerRootIdentity.OpenRoot()
	if operationError != nil {
		return nil, operationError
	}
	defer providerRoot.Close()
	if operationError := validateDirectoryComponents(providerRoot, sessionRelative); operationError != nil {
		return nil, operationError
	}
	sessionRoot, operationError := openStableSubroot(providerRoot, sessionRelative)
	if operationError != nil {
		return nil, operationError
	}
	if operationError := validateDirectoryComponents(providerRoot, sessionRelative); operationError != nil {
		_ = sessionRoot.Close()
		return nil, operationError
	}
	summaryData, operationError := providerInstance.readMetadataFile(sessionRoot, "summary.json")
	if operationError != nil {
		_ = sessionRoot.Close()
		return nil, operationError
	}
	summary := map[string]any{}
	decoder := json.NewDecoder(strings.NewReader(string(summaryData)))
	decoder.UseNumber()
	operationError = decoder.Decode(&summary)
	if operationError != nil {
		_ = sessionRoot.Close()
		return nil, operationError
	}
	if summary == nil {
		_ = sessionRoot.Close()
		return nil, errors.New("JSON object required")
	}
	info, _ := summary["info"].(map[string]any)
	summarySessionID := strings.TrimSpace(stringValue(info["id"]))
	if summarySessionID == "" {
		_ = sessionRoot.Close()
		return nil, errors.New("summary info.id required")
	}
	if filepath.Base(sessionRelative) != summarySessionID {
		_ = sessionRoot.Close()
		return nil, errors.New("session directory does not match summary info.id")
	}
	if expectedSessionID != "" && summarySessionID != expectedSessionID {
		_ = sessionRoot.Close()
		return nil, errors.New("summary sessionId mismatch")
	}
	return &sessionAccess{
		root: sessionRoot, sourcePath: filepath.Join(selectedRoot, selectedRelative), summaryData: summary,
	}, nil
}

func openRegularFile(root *os.Root, name string) (*os.File, error) {
	fileInfo, operationError := root.Lstat(name)
	if operationError != nil {
		return nil, operationError
	}
	if fileInfo.Mode()&os.ModeSymlink != 0 || !fileInfo.Mode().IsRegular() {
		return nil, errors.New("regular file required")
	}
	file, operationError := root.Open(name)
	if operationError != nil {
		return nil, operationError
	}
	openedInfo, operationError := file.Stat()
	if operationError != nil || !openedInfo.Mode().IsRegular() || !os.SameFile(fileInfo, openedInfo) {
		_ = file.Close()
		return nil, errors.New("file changed while opening")
	}
	return file, nil
}

func openStableSubroot(parentRoot *os.Root, relativePath string) (*os.Root, error) {
	beforeInfo, operationError := parentRoot.Lstat(relativePath)
	if operationError != nil {
		return nil, operationError
	}
	if beforeInfo.Mode()&os.ModeSymlink != 0 || !beforeInfo.IsDir() {
		return nil, errors.New("session root must be a directory")
	}
	root, operationError := parentRoot.OpenRoot(relativePath)
	if operationError != nil {
		return nil, operationError
	}
	openedInfo, operationError := root.Stat(".")
	if operationError != nil {
		_ = root.Close()
		return nil, operationError
	}
	afterInfo, operationError := parentRoot.Lstat(relativePath)
	if operationError != nil || afterInfo.Mode()&os.ModeSymlink != 0 || !os.SameFile(beforeInfo, openedInfo) || !os.SameFile(openedInfo, afterInfo) {
		_ = root.Close()
		return nil, errors.New("session root changed while opening")
	}
	return root, nil
}

func validateDirectoryComponents(root *os.Root, relativePath string) error {
	currentRelative := "."
	for _, component := range strings.Split(filepath.Clean(relativePath), string(filepath.Separator)) {
		if component == "" || component == "." {
			continue
		}
		currentRelative = filepath.Join(currentRelative, component)
		fileInfo, operationError := root.Lstat(currentRelative)
		if operationError != nil {
			return operationError
		}
		if fileInfo.Mode()&os.ModeSymlink != 0 || !fileInfo.IsDir() {
			return errors.New("Grok session path contains a symbolic link")
		}
	}
	return nil
}

func createTemporarySummary(root *os.Root) (string, *os.File, error) {
	for attempt := 0; attempt < temporaryFileCreationAttempts; attempt++ {
		randomBytes := make([]byte, randomSuffixBytes)
		if _, operationError := rand.Read(randomBytes); operationError != nil {
			return "", nil, operationError
		}
		name := ".summary.json.any-aicli-remote-" + hex.EncodeToString(randomBytes) + ".tmp"
		file, operationError := root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if errors.Is(operationError, os.ErrExist) {
			continue
		}
		if operationError != nil {
			return "", nil, operationError
		}
		return name, file, nil
	}
	return "", nil, errors.New("could not create unique summary file")
}

func (providerInstance *GrokProvider) readMetadataFile(root *os.Root, name string) ([]byte, error) {
	file, operationError := openRegularFile(root, name)
	if operationError != nil {
		return nil, operationError
	}
	defer file.Close()
	limit := providerInstance.historyPolicy.AdapterReadBytes
	if limit < 1 || limit >= int64(math.MaxInt64) {
		return nil, MetadataFileTooLargeError
	}
	data, operationError := io.ReadAll(io.LimitReader(file, limit+1))
	if operationError != nil {
		return nil, operationError
	}
	if int64(len(data)) > limit {
		return nil, MetadataFileTooLargeError
	}
	return data, nil
}

func collectSummaryPaths(operationContext context.Context, root string) ([]string, error) {
	paths := make([]string, 0)
	operationError := filepath.WalkDir(root, func(path string, entry os.DirEntry, traversalError error) error {
		if traversalError != nil {
			if errors.Is(traversalError, os.ErrNotExist) {
				return nil
			}
			return traversalError
		}
		if operationError := operationContext.Err(); operationError != nil {
			return operationError
		}
		if entry.Type()&os.ModeSymlink != 0 && entry.IsDir() {
			return filepath.SkipDir
		}
		if !entry.IsDir() && entry.Name() == "summary.json" {
			paths = append(paths, path)
		}
		return nil
	})
	if errors.Is(operationError, os.ErrNotExist) {
		return paths, nil
	}
	return paths, operationError
}
