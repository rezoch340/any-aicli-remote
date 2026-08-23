package grok

import (
	"os"
	"path/filepath"
	"strings"

	providerapi "github.com/rezoch340/any-aicli-remote/backend/internal/provider"
)

// SkillRoots keeps Grok's global and workspace path conventions inside the
// provider adapter. The scanner receives only explicit directories.
func (providerInstance *GrokProvider) SkillRoots(workingDirectory string) providerapi.SkillRoots {
	providerDirectory := filepath.Dir(providerInstance.activeRoot)
	roots := []providerapi.SkillRoot{
		newSkillRoot(filepath.Join(providerDirectory, "skills"), providerapi.SkillRootSourceUser),
		newSkillRoot(filepath.Join(providerDirectory, "bundled", "skills"), providerapi.SkillRootSourceBundled),
		newSkillRoot(filepath.Join(providerDirectory, "plugins"), providerapi.SkillRootSourcePlugin),
		newSkillRoot(filepath.Join(providerDirectory, "installed-plugins"), providerapi.SkillRootSourcePlugin),
		newSkillRoot(filepath.Join(providerDirectory, "marketplace-cache"), providerapi.SkillRootSourceMarketplace),
	}
	roots = append(roots, discoverCommandRoots(
		filepath.Join(providerDirectory, "plugins"),
		filepath.Join(providerDirectory, "installed-plugins"),
		filepath.Join(providerDirectory, "marketplace-cache"),
	)...)
	if strings.TrimSpace(workingDirectory) != "" {
		roots = append(roots,
			newSkillRoot(filepath.Join(workingDirectory, ".grok", "skills"), providerapi.SkillRootSourceUser),
			newSkillRoot(filepath.Join(workingDirectory, ".agents", "skills"), providerapi.SkillRootSourceUser),
			newSkillRoot(filepath.Join(workingDirectory, "skills"), providerapi.SkillRootSourceUser),
			newCommandRoot(filepath.Join(workingDirectory, ".grok", "commands")),
			newCommandRoot(filepath.Join(workingDirectory, "commands")),
		)
	}
	return providerapi.SkillRoots{Roots: roots}
}

func newSkillRoot(path string, source providerapi.SkillRootSource) providerapi.SkillRoot {
	return providerapi.SkillRoot{Kind: providerapi.SkillRootKindSkill, Source: source, Path: path}
}

func newCommandRoot(path string) providerapi.SkillRoot {
	return providerapi.SkillRoot{
		Kind: providerapi.SkillRootKindCommand, Source: providerapi.SkillRootSourceCommand, Path: path,
	}
}

// discoverCommandRoots understands Grok's plugin storage layout so the
// provider-neutral scanner never needs to infer command semantics from path
// segments. WalkDir deliberately does not follow nested directory symlinks.
func discoverCommandRoots(baseDirectories ...string) []providerapi.SkillRoot {
	roots := make([]providerapi.SkillRoot, 0)
	seen := make(map[string]struct{})
	for _, baseDirectory := range baseDirectories {
		absoluteDirectory, operationError := filepath.Abs(baseDirectory)
		if operationError != nil {
			continue
		}
		canonicalDirectory, operationError := filepath.EvalSymlinks(absoluteDirectory)
		if operationError != nil {
			continue
		}
		_ = filepath.WalkDir(canonicalDirectory, func(path string, directoryEntry os.DirEntry, walkError error) error {
			if walkError != nil {
				return nil
			}
			if !directoryEntry.IsDir() {
				return nil
			}
			if directoryEntry.Name() == ".git" || directoryEntry.Name() == "node_modules" {
				return filepath.SkipDir
			}
			if directoryEntry.Name() != "commands" {
				return nil
			}
			canonicalPath := filepath.Clean(path)
			if _, present := seen[canonicalPath]; !present {
				seen[canonicalPath] = struct{}{}
				roots = append(roots, newCommandRoot(canonicalPath))
			}
			return filepath.SkipDir
		})
	}
	return roots
}

var _ providerapi.SkillRootProvider = (*GrokProvider)(nil)
