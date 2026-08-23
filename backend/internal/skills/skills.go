// Package skills scans explicitly configured SKILL.md and command markdown roots.
package skills

import (
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"

	providerapi "github.com/rezoch340/any-aicli-remote/backend/internal/provider"
	"go.yaml.in/yaml/v4"
)

const (
	descriptionEllipsis = "..."
	unknownSourceOrder  = 9
)

// Item is the JSON shape exposed by the compatibility /api/skills/list endpoint.
var MetadataFileTooLargeError = errors.New("skill metadata file exceeds policy limit")

type Policy struct {
	MaxFileBytes        int64
	DescriptionMaxRunes int
	MaxItems            int
}

func (policy Policy) Validate() error {
	if policy.MaxFileBytes <= 0 || policy.MaxFileBytes >= math.MaxInt64 || policy.DescriptionMaxRunes <= 0 || policy.MaxItems <= 0 {
		return errors.New("invalid skills policy")
	}
	return nil
}

type Item struct {
	Name          string `json:"name"`
	Description   string `json:"description"`
	When          string `json:"when"`
	Hint          string `json:"hint"`
	Source        string `json:"source"`
	Path          string `json:"path"`
	Kind          string `json:"kind"`
	UserInvocable bool   `json:"userInvocable"`
	Invoke        string `json:"invoke"`
}

// Frontmatter contains the metadata fields consumed by the skills endpoint.
// YAML syntax and scalar decoding are delegated to go-yaml.
type Frontmatter struct {
	Name          string `yaml:"name"`
	Description   string `yaml:"description"`
	When          string `yaml:"when-to-use"`
	Hint          string `yaml:"argument-hint"`
	UserInvocable *bool  `yaml:"user-invocable"`
}

// ParseFrontmatter extracts a leading YAML frontmatter block. Only delimiter
// detection is local; go-yaml parses all metadata values and block scalars.
func ParseFrontmatter(text string) (Frontmatter, string, error) {
	rawFrontmatter, body, present := extractFrontmatter(text)
	if !present {
		return Frontmatter{}, text, nil
	}
	var metadata Frontmatter
	if operationError := yaml.Unmarshal([]byte(rawFrontmatter), &metadata); operationError != nil {
		return Frontmatter{}, body, operationError
	}
	return metadata, body, nil
}

func extractFrontmatter(text string) (string, string, bool) {
	lines := strings.Split(text, "\n")
	if len(lines) == 0 || strings.TrimSuffix(lines[0], "\r") != "---" {
		return "", text, false
	}
	for lineIndex := 1; lineIndex < len(lines); lineIndex++ {
		if strings.TrimSuffix(lines[lineIndex], "\r") != "---" {
			continue
		}
		frontmatter := strings.Join(lines[1:lineIndex], "\n")
		body := strings.Join(lines[lineIndex+1:], "\n")
		return frontmatter, strings.TrimLeft(body, "\r\n"), true
	}
	return "", text, false
}

func hasSkippedPart(path string) bool {
	for _, part := range strings.Split(filepath.ToSlash(path), "/") {
		if part == "node_modules" || part == ".git" {
			return true
		}
	}
	return false
}

func shortDescription(value string, maximum int) string {
	runes := []rune(value)
	if len(runes) <= maximum {
		return value
	}
	if maximum <= len(descriptionEllipsis) {
		return string(runes[:maximum])
	}
	return string(runes[:maximum-len(descriptionEllipsis)]) + descriptionEllipsis
}

func invocable(metadata Frontmatter) bool {
	return metadata.UserInvocable == nil || *metadata.UserInvocable
}

type scanRoot struct {
	canonicalPath string
	kind          providerapi.SkillRootKind
	source        providerapi.SkillRootSource
}

// canonicalRoots deliberately resolves a configured root symlink once before
// traversal. Nested symlinks and symlinked metadata files remain forbidden.
func canonicalRoots(configuredRoots []providerapi.SkillRoot) []scanRoot {
	roots := make([]scanRoot, 0, len(configuredRoots))
	seen := make(map[string]struct{}, len(configuredRoots))
	for _, configuredRoot := range configuredRoots {
		if strings.TrimSpace(configuredRoot.Path) == "" || strings.TrimSpace(string(configuredRoot.Source)) == "" ||
			(configuredRoot.Kind != providerapi.SkillRootKindSkill && configuredRoot.Kind != providerapi.SkillRootKindCommand) {
			continue
		}
		absolutePath, operationError := filepath.Abs(configuredRoot.Path)
		if operationError != nil {
			continue
		}
		canonicalPath, operationError := filepath.EvalSymlinks(absolutePath)
		if operationError != nil {
			continue
		}
		fileInfo, operationError := os.Stat(canonicalPath)
		if operationError != nil || !fileInfo.IsDir() {
			continue
		}
		cacheKey := string(configuredRoot.Kind) + "\x00" + canonicalPath
		if _, present := seen[cacheKey]; present {
			continue
		}
		seen[cacheKey] = struct{}{}
		roots = append(roots, scanRoot{
			canonicalPath: canonicalPath,
			kind:          configuredRoot.Kind,
			source:        configuredRoot.Source,
		})
	}
	return roots
}

func readRegularFile(path string, maximum int64) ([]byte, error) {
	pathInfo, operationError := os.Lstat(path)
	if operationError != nil {
		return nil, operationError
	}
	if pathInfo.Mode()&os.ModeSymlink != 0 || !pathInfo.Mode().IsRegular() {
		return nil, errors.New("metadata path is not a regular file")
	}
	file, operationError := os.Open(path)
	if operationError != nil {
		return nil, operationError
	}
	defer file.Close()
	openedInfo, operationError := file.Stat()
	if operationError != nil {
		return nil, operationError
	}
	currentInfo, operationError := os.Lstat(path)
	if operationError != nil {
		return nil, operationError
	}
	if currentInfo.Mode()&os.ModeSymlink != 0 || !currentInfo.Mode().IsRegular() || !os.SameFile(pathInfo, openedInfo) || !os.SameFile(openedInfo, currentInfo) {
		return nil, errors.New("metadata path changed during open")
	}
	data, operationError := io.ReadAll(io.LimitReader(file, maximum+1))
	if operationError != nil {
		return nil, operationError
	}
	if int64(len(data)) > maximum {
		return nil, fmt.Errorf("%w: %s", MetadataFileTooLargeError, path)
	}
	return data, nil
}

func metadataCandidate(root scanRoot, directoryEntry os.DirEntry) bool {
	if root.kind == providerapi.SkillRootKindSkill {
		return directoryEntry.Name() == "SKILL.md"
	}
	lowerName := strings.ToLower(directoryEntry.Name())
	return filepath.Ext(lowerName) == ".md" && lowerName != "readme.md" &&
		lowerName != "changelog.md" && lowerName != "license.md"
}

func readItem(root scanRoot, path string, policy Policy) (Item, bool) {
	raw, operationError := readRegularFile(path, policy.MaxFileBytes)
	if operationError != nil {
		return Item{}, false
	}
	metadata, _, operationError := ParseFrontmatter(string(raw))
	if operationError != nil {
		return Item{}, false
	}
	name := strings.TrimSpace(metadata.Name)
	if root.kind == providerapi.SkillRootKindCommand {
		name = strings.TrimLeft(name, "/")
		if name == "" {
			name = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
		}
	} else if name == "" {
		name = filepath.Base(filepath.Dir(path))
	}
	if name == "" {
		return Item{}, false
	}
	item := Item{
		Name:          name,
		Description:   shortDescription(metadata.Description, policy.DescriptionMaxRunes),
		Hint:          metadata.Hint,
		Source:        string(root.source),
		Path:          path,
		Kind:          string(root.kind),
		UserInvocable: true,
		Invoke:        "/" + name,
	}
	if root.kind == providerapi.SkillRootKindSkill {
		item.When = metadata.When
		item.UserInvocable = invocable(metadata)
	}
	return item, true
}

// Scan returns discovered skills and slash commands from provider-supplied
// roots, de-duplicated case-insensitively.
func Scan(configuredRoots []providerapi.SkillRoot, policy Policy) ([]Item, error) {
	if operationError := policy.Validate(); operationError != nil {
		return nil, operationError
	}
	seen := map[string]bool{}
	items := []Item{}
	for _, root := range canonicalRoots(configuredRoots) {
		if len(items) >= policy.MaxItems {
			break
		}
		_ = filepath.WalkDir(root.canonicalPath, func(path string, directoryEntry os.DirEntry, operationError error) error {
			if len(items) >= policy.MaxItems {
				return filepath.SkipDir
			}
			if operationError != nil {
				return nil
			}
			if directoryEntry.IsDir() {
				if directoryEntry.Name() == "node_modules" || directoryEntry.Name() == ".git" {
					return filepath.SkipDir
				}
				return nil
			}
			if !metadataCandidate(root, directoryEntry) || hasSkippedPart(path) {
				return nil
			}
			item, valid := readItem(root, path, policy)
			if !valid {
				return nil
			}
			key := strings.ToLower(item.Name)
			if seen[key] {
				return nil
			}
			seen[key] = true
			items = append(items, item)
			return nil
		})
	}

	order := map[string]int{"bundled": 0, "user": 1, "plugin": 2, "command": 3, "marketplace": 4, "agent": 5}
	sort.Slice(items, func(leftIndex, rightIndex int) bool {
		leftOrder, valid := order[items[leftIndex].Source]
		if !valid {
			leftOrder = unknownSourceOrder
		}
		rightOrder, valid := order[items[rightIndex].Source]
		if !valid {
			rightOrder = unknownSourceOrder
		}
		if leftOrder != rightOrder {
			return leftOrder < rightOrder
		}
		return strings.ToLower(items[leftIndex].Name) < strings.ToLower(items[rightIndex].Name)
	})
	return items, nil
}
