// Package skills scans Grok/Codex-style SKILL.md and command markdown metadata.
package skills

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Item is the JSON shape exposed by the old Python /api/skills/list endpoint.
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

// Frontmatter is a deliberately small YAML-frontmatter parser matching server.py.
type Frontmatter map[string]string

// ParseFrontmatter parses a leading --- frontmatter block and returns metadata plus body.
// It supports simple key: value lines and indented block-scalar continuations.
func ParseFrontmatter(text string) (Frontmatter, string) {
	meta := Frontmatter{}
	body := text
	if !strings.HasPrefix(text, "---") {
		return meta, body
	}
	parts := strings.SplitN(text, "---", 3)
	if len(parts) < 3 {
		return meta, body
	}
	raw := parts[1]
	body = strings.TrimLeft(parts[2], "\n")

	key := ""
	acc := []string{}
	flush := func() {
		if key == "" {
			return
		}
		meta[key] = stripQuotes(strings.TrimSpace(strings.Join(acc, " ")))
		key = ""
		acc = nil
	}
	for _, line := range strings.Split(raw, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		if (strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t")) && key != "" {
			acc = append(acc, strings.TrimSpace(line))
			continue
		}
		flush()
		if lineIndex := strings.Index(line, ":"); lineIndex >= 0 {
			parsedKey := strings.TrimSpace(line[:lineIndex])
			value := strings.TrimSpace(line[lineIndex+1:])
			switch value {
			case "|", ">", ">-", "|-", "":
				key = parsedKey
				acc = []string{}
			default:
				meta[parsedKey] = stripQuotes(value)
			}
		}
	}
	flush()
	return meta, body
}

func stripQuotes(value string) string {
	value = strings.TrimSpace(value)
	if len(value) >= 2 {
		if (value[0] == '"' && value[len(value)-1] == '"') || (value[0] == '\'' && value[len(value)-1] == '\'') {
			return value[1 : len(value)-1]
		}
	}
	return value
}

// Roots returns skill roots in the same precedence as server.py.
func Roots(workingDirectory string) []string {
	home, _ := os.UserHomeDir()
	roots := []string{}
	if home != "" {
		grok := filepath.Join(home, ".grok")
		for _, root := range []string{"skills", "bundled/skills", "plugins", "installed-plugins", "marketplace-cache"} {
			path := filepath.Join(grok, filepath.FromSlash(root))
			if isDirectory(path) {
				roots = append(roots, path)
			}
		}
	}
	if strings.TrimSpace(workingDirectory) != "" {
		for _, root := range []string{".grok/skills", ".agents/skills", "skills"} {
			path := filepath.Join(workingDirectory, filepath.FromSlash(root))
			if isDirectory(path) {
				roots = append(roots, path)
			}
		}
	}
	return roots
}

func commandRoots(workingDirectory string) []string {
	home, _ := os.UserHomeDir()
	roots := []string{}
	if home != "" {
		grok := filepath.Join(home, ".grok")
		for _, root := range []string{"plugins", "installed-plugins", "marketplace-cache"} {
			path := filepath.Join(grok, root)
			if isDirectory(path) {
				roots = append(roots, path)
			}
		}
	}
	if strings.TrimSpace(workingDirectory) != "" {
		for _, root := range []string{".grok/commands", "commands"} {
			path := filepath.Join(workingDirectory, filepath.FromSlash(root))
			if isDirectory(path) {
				roots = append(roots, path)
			}
		}
	}
	return roots
}

func isDirectory(path string) bool {
	fileInfo, operationError := os.Stat(path)
	return operationError == nil && fileInfo.IsDir()
}

func hasSkippedPart(path string) bool {
	for _, part := range strings.Split(filepath.ToSlash(path), "/") {
		if part == "node_modules" || part == ".git" {
			return true
		}
	}
	return false
}

func hasPart(path, want string) bool {
	for _, part := range strings.Split(filepath.ToSlash(path), "/") {
		if part == want {
			return true
		}
	}
	return false
}

func sourceFor(path string) string {
	slashPath := "/" + filepath.ToSlash(path) + "/"
	switch {
	case strings.Contains(slashPath, "/bundled/"):
		return "bundled"
	case strings.Contains(slashPath, "/marketplace-cache/"):
		return "marketplace"
	case strings.Contains(slashPath, "/plugins/") || strings.Contains(slashPath, "/installed-plugins/"):
		return "plugin"
	default:
		return "user"
	}
}

func shortDesc(value string) string {
	if len([]rune(value)) <= 240 {
		return value
	}
	runes := []rune(value)
	return string(runes[:237]) + "..."
}

func invocable(meta Frontmatter) bool {
	switch strings.ToLower(strings.TrimSpace(meta["user-invocable"])) {
	case "false", "0", "no":
		return false
	default:
		return true
	}
}

// Scan returns all discovered skills and slash commands, de-duplicated case-insensitively.
func Scan(workingDirectory string) ([]Item, error) {
	seen := map[string]bool{}
	out := []Item{}

	for _, root := range Roots(workingDirectory) {
		_ = filepath.WalkDir(root, func(path string, directoryEntry os.DirEntry, operationError error) error {
			if operationError != nil {
				return nil
			}
			if directoryEntry.IsDir() {
				if directoryEntry.Name() == "node_modules" || directoryEntry.Name() == ".git" {
					return filepath.SkipDir
				}
				return nil
			}
			if directoryEntry.Name() != "SKILL.md" || hasSkippedPart(path) {
				return nil
			}
			raw, operationError := os.ReadFile(path)
			if operationError != nil {
				return nil
			}
			meta, _ := ParseFrontmatter(string(raw))
			name := strings.TrimSpace(meta["name"])
			if name == "" {
				name = filepath.Base(filepath.Dir(path))
			}
			key := strings.ToLower(name)
			if name == "" || seen[key] {
				return nil
			}
			seen[key] = true
			source := sourceFor(path)
			out = append(out, Item{
				Name:          name,
				Description:   shortDesc(meta["description"]),
				When:          meta["when-to-use"],
				Hint:          meta["argument-hint"],
				Source:        source,
				Path:          path,
				Kind:          "skill",
				UserInvocable: invocable(meta),
				Invoke:        "/" + name,
			})
			return nil
		})
	}

	for _, root := range commandRoots(workingDirectory) {
		_ = filepath.WalkDir(root, func(path string, directoryEntry os.DirEntry, operationError error) error {
			if operationError != nil {
				return nil
			}
			if directoryEntry.IsDir() {
				if directoryEntry.Name() == "node_modules" || directoryEntry.Name() == ".git" {
					return filepath.SkipDir
				}
				return nil
			}
			lower := strings.ToLower(directoryEntry.Name())
			if filepath.Ext(lower) != ".md" || lower == "readme.md" || lower == "changelog.md" || lower == "license.md" || !hasPart(path, "commands") || hasSkippedPart(path) {
				return nil
			}
			raw, operationError := os.ReadFile(path)
			if operationError != nil {
				return nil
			}
			meta, _ := ParseFrontmatter(string(raw))
			name := strings.TrimLeft(strings.TrimSpace(meta["name"]), "/")
			if name == "" {
				name = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
			}
			key := strings.ToLower(name)
			if name == "" || seen[key] {
				return nil
			}
			seen[key] = true
			out = append(out, Item{
				Name:          name,
				Description:   shortDesc(meta["description"]),
				Hint:          meta["argument-hint"],
				Source:        "command",
				Path:          path,
				Kind:          "command",
				UserInvocable: true,
				Invoke:        "/" + name,
			})
			return nil
		})
	}

	order := map[string]int{"bundled": 0, "user": 1, "plugin": 2, "command": 3, "marketplace": 4, "agent": 5}
	sort.Slice(out, func(leftIndex, rightIndex int) bool {
		leftOrder, valid := order[out[leftIndex].Source]
		if !valid {
			leftOrder = 9
		}
		rightOrder, valid := order[out[rightIndex].Source]
		if !valid {
			rightOrder = 9
		}
		if leftOrder != rightOrder {
			return leftOrder < rightOrder
		}
		return strings.ToLower(out[leftIndex].Name) < strings.ToLower(out[rightIndex].Name)
	})
	return out, nil
}
