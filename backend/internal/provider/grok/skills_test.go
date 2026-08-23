package grok

import (
	"os"
	"path/filepath"
	"testing"

	providerapi "github.com/rezoch340/any-aicli-remote/backend/internal/provider"
	skillsapi "github.com/rezoch340/any-aicli-remote/backend/internal/skills"
)

func TestSkillRootsUseProviderAndSessionWorkspaceConventions(testContext *testing.T) {
	providerDirectory := testContext.TempDir()
	workingDirectory := testContext.TempDir()
	providerInstance := New(Config{SessionsDirectory: filepath.Join(providerDirectory, "sessions")})
	roots := providerInstance.SkillRoots(workingDirectory)
	if !containsSkillRoot(roots.Roots, filepath.Join(providerDirectory, "skills"), providerapi.SkillRootKindSkill, providerapi.SkillRootSourceUser) ||
		!containsSkillRoot(roots.Roots, filepath.Join(providerDirectory, "bundled", "skills"), providerapi.SkillRootKindSkill, providerapi.SkillRootSourceBundled) ||
		!containsSkillRoot(roots.Roots, filepath.Join(providerDirectory, "plugins"), providerapi.SkillRootKindSkill, providerapi.SkillRootSourcePlugin) ||
		!containsSkillRoot(roots.Roots, filepath.Join(workingDirectory, ".grok", "skills"), providerapi.SkillRootKindSkill, providerapi.SkillRootSourceUser) ||
		!containsSkillRoot(roots.Roots, filepath.Join(workingDirectory, ".agents", "skills"), providerapi.SkillRootKindSkill, providerapi.SkillRootSourceUser) ||
		!containsSkillRoot(roots.Roots, filepath.Join(workingDirectory, ".grok", "commands"), providerapi.SkillRootKindCommand, providerapi.SkillRootSourceCommand) {
		testContext.Fatalf("skill roots = %#v", roots)
	}
}

func TestSkillRootsDeclareOnlyActualPluginCommandDirectories(testContext *testing.T) {
	providerDirectory := testContext.TempDir()
	commandDirectory := filepath.Join(providerDirectory, "installed-plugins", "sample", "commands")
	skillDirectory := filepath.Join(providerDirectory, "installed-plugins", "sample", "skills", "sample-skill")
	documentationDirectory := filepath.Join(providerDirectory, "installed-plugins", "sample", "docs")
	writeSkillFixture(testContext, filepath.Join(commandDirectory, "deploy.md"), "---\nname: deploy\ndescription: Deploy safely\n---\n")
	writeSkillFixture(testContext, filepath.Join(skillDirectory, "SKILL.md"), "---\nname: sample-skill\ndescription: Sample skill\n---\n")
	writeSkillFixture(testContext, filepath.Join(documentationDirectory, "guide.md"), "---\nname: guide\ndescription: Documentation only\n---\n")

	providerInstance := New(Config{SessionsDirectory: filepath.Join(providerDirectory, "sessions")})
	roots := providerInstance.SkillRoots("")
	canonicalCommandDirectory, operationError := filepath.EvalSymlinks(commandDirectory)
	if operationError != nil {
		testContext.Fatalf("canonicalize command directory: %v", operationError)
	}
	if !containsSkillRoot(roots.Roots, canonicalCommandDirectory, providerapi.SkillRootKindCommand, providerapi.SkillRootSourceCommand) {
		testContext.Fatalf("nested command root missing: %#v", roots.Roots)
	}
	if containsSkillRoot(roots.Roots, filepath.Join(providerDirectory, "installed-plugins"), providerapi.SkillRootKindCommand, providerapi.SkillRootSourceCommand) {
		testContext.Fatalf("broad plugin directory must not be a command root: %#v", roots.Roots)
	}

	items, operationError := skillsapi.Scan(roots.Roots)
	if operationError != nil {
		testContext.Fatalf("scan typed roots: %v", operationError)
	}
	if !containsSkillItem(items, "deploy", string(providerapi.SkillRootKindCommand)) ||
		!containsSkillItem(items, "sample-skill", string(providerapi.SkillRootKindSkill)) {
		testContext.Fatalf("expected plugin items missing: %#v", items)
	}
	if containsSkillItem(items, "guide", string(providerapi.SkillRootKindCommand)) {
		testContext.Fatalf("documentation was incorrectly exposed as a command: %#v", items)
	}
}

func writeSkillFixture(testContext *testing.T, path, contents string) {
	testContext.Helper()
	if operationError := os.MkdirAll(filepath.Dir(path), 0o755); operationError != nil {
		testContext.Fatalf("create fixture directory: %v", operationError)
	}
	if operationError := os.WriteFile(path, []byte(contents), 0o644); operationError != nil {
		testContext.Fatalf("write fixture: %v", operationError)
	}
}

func containsSkillItem(items []skillsapi.Item, wantedName, wantedKind string) bool {
	for _, item := range items {
		if item.Name == wantedName && item.Kind == wantedKind {
			return true
		}
	}
	return false
}

func containsSkillRoot(
	roots []providerapi.SkillRoot,
	wantedPath string,
	wantedKind providerapi.SkillRootKind,
	wantedSource providerapi.SkillRootSource,
) bool {
	for _, root := range roots {
		if root.Path == wantedPath && root.Kind == wantedKind && root.Source == wantedSource {
			return true
		}
	}
	return false
}
