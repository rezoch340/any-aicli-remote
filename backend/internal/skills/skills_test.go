package skills

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	providerapi "github.com/rezoch340/any-aicli-remote/backend/internal/provider"
)

func write(testContext *testing.T, path, body string) {
	testContext.Helper()
	if operationError := os.MkdirAll(filepath.Dir(path), 0o755); operationError != nil {
		testContext.Fatal(operationError)
	}
	if operationError := os.WriteFile(path, []byte(body), 0o644); operationError != nil {
		testContext.Fatal(operationError)
	}
}

func declaredRoot(path string, kind providerapi.SkillRootKind, source providerapi.SkillRootSource) providerapi.SkillRoot {
	return providerapi.SkillRoot{Path: path, Kind: kind, Source: source}
}

func TestParseFrontmatterUsesYAMLDecoder(testContext *testing.T) {
	metadata, body, operationError := ParseFrontmatter("---\nname: 'remote'\ndescription: |\n  first\n  second\nuser-invocable: false\n---\n# Body")
	if operationError != nil {
		testContext.Fatal(operationError)
	}
	if metadata.Name != "remote" || metadata.Description != "first\nsecond\n" || metadata.UserInvocable == nil || *metadata.UserInvocable || body != "# Body" {
		testContext.Fatalf("metadata=%#v body=%q", metadata, body)
	}
}

func TestParseFrontmatterRejectsMalformedYAML(testContext *testing.T) {
	_, body, operationError := ParseFrontmatter("---\nname: [broken\n---\nbody")
	if operationError == nil || body != "body" {
		testContext.Fatalf("error=%v body=%q", operationError, body)
	}
}

func TestScanSkillsAndCommands(testContext *testing.T) {
	root := testContext.TempDir()
	globalDirectory := filepath.Join(root, "provider")
	workingDirectory := filepath.Join(root, "work")

	write(testContext, filepath.Join(globalDirectory, "bundled", "skills", "b", "SKILL.md"), "---\nname: bundled-one\ndescription: b\n---\n")
	write(testContext, filepath.Join(globalDirectory, "installed-plugins", "plug", "skills", "p", "SKILL.md"), "---\nname: plug-one\ndescription: "+strings.Repeat("x", 250)+"\nwhen-to-use: now\nargument-hint: arg\nuser-invocable: no\n---\n")
	write(testContext, filepath.Join(workingDirectory, "skills", "local", "SKILL.md"), "---\ndescription: local skill\n---\n")
	write(testContext, filepath.Join(workingDirectory, "commands", "remote.md"), "---\nname: /remote\ndescription: command\nargument-hint: cmdarg\n---\n")
	write(testContext, filepath.Join(workingDirectory, "skills", "node_modules", "bad", "SKILL.md"), "---\nname: bad\n---\n")

	configuredRoots := []providerapi.SkillRoot{
		declaredRoot(filepath.Join(globalDirectory, "bundled", "skills"), providerapi.SkillRootKindSkill, providerapi.SkillRootSourceBundled),
		declaredRoot(filepath.Join(workingDirectory, "skills"), providerapi.SkillRootKindSkill, providerapi.SkillRootSourceUser),
		declaredRoot(filepath.Join(globalDirectory, "installed-plugins"), providerapi.SkillRootKindSkill, providerapi.SkillRootSourcePlugin),
		declaredRoot(filepath.Join(workingDirectory, "commands"), providerapi.SkillRootKindCommand, providerapi.SkillRootSourceCommand),
	}
	items, operationError := Scan(configuredRoots)
	if operationError != nil {
		testContext.Fatal(operationError)
	}
	if len(items) != 4 {
		testContext.Fatalf("len=%d items=%#v", len(items), items)
	}
	names := []string{items[0].Name, items[1].Name, items[2].Name, items[3].Name}
	wantedNames := []string{"bundled-one", "local", "plug-one", "remote"}
	for itemIndex := range wantedNames {
		if names[itemIndex] != wantedNames[itemIndex] {
			testContext.Fatalf("order/name got %v want %v items=%#v", names, wantedNames, items)
		}
	}
	plugin := items[2]
	if plugin.Source != "plugin" || plugin.UserInvocable || len([]rune(plugin.Description)) != 240 || !strings.HasSuffix(plugin.Description, "...") || plugin.When != "now" || plugin.Hint != "arg" {
		testContext.Fatalf("bad plugin item: %#v", plugin)
	}
	command := items[3]
	if command.Kind != "command" || command.Source != "command" || command.Invoke != "/remote" || command.Hint != "cmdarg" {
		testContext.Fatalf("bad command: %#v", command)
	}
}

func TestScanDeduplicatesCaseInsensitiveNames(testContext *testing.T) {
	root := testContext.TempDir()
	firstRoot := filepath.Join(root, "first")
	secondRoot := filepath.Join(root, "second")
	write(testContext, filepath.Join(firstRoot, "a", "SKILL.md"), "---\nname: Same\n---\n")
	write(testContext, filepath.Join(secondRoot, "b", "SKILL.md"), "---\nname: same\n---\n")
	items, operationError := Scan([]providerapi.SkillRoot{
		declaredRoot(firstRoot, providerapi.SkillRootKindSkill, providerapi.SkillRootSourceUser),
		declaredRoot(secondRoot, providerapi.SkillRootKindSkill, providerapi.SkillRootSourceUser),
	})
	if operationError != nil {
		testContext.Fatal(operationError)
	}
	if len(items) != 1 || items[0].Name != "Same" {
		testContext.Fatalf("dedup failed: %#v", items)
	}
}

func TestScanRejectsSymlinkedMetadataFiles(testContext *testing.T) {
	root := testContext.TempDir()
	outsideDirectory := testContext.TempDir()
	outsideSkill := filepath.Join(outsideDirectory, "outside-skill.md")
	write(testContext, outsideSkill, "---\nname: escaped-skill\n---\n")
	skillLink := filepath.Join(root, "skills", "escaped", "SKILL.md")
	if operationError := os.MkdirAll(filepath.Dir(skillLink), 0o755); operationError != nil {
		testContext.Fatal(operationError)
	}
	if operationError := os.Symlink(outsideSkill, skillLink); operationError != nil {
		testContext.Fatal(operationError)
	}

	outsideCommand := filepath.Join(outsideDirectory, "outside-command.md")
	write(testContext, outsideCommand, "---\nname: escaped-command\n---\n")
	commandLink := filepath.Join(root, "commands", "escaped.md")
	if operationError := os.MkdirAll(filepath.Dir(commandLink), 0o755); operationError != nil {
		testContext.Fatal(operationError)
	}
	if operationError := os.Symlink(outsideCommand, commandLink); operationError != nil {
		testContext.Fatal(operationError)
	}

	items, operationError := Scan([]providerapi.SkillRoot{
		declaredRoot(filepath.Join(root, "skills"), providerapi.SkillRootKindSkill, providerapi.SkillRootSourceUser),
		declaredRoot(filepath.Join(root, "commands"), providerapi.SkillRootKindCommand, providerapi.SkillRootSourceCommand),
	})
	if operationError != nil {
		testContext.Fatal(operationError)
	}
	if len(items) != 0 {
		testContext.Fatalf("symlinked metadata escaped roots: %#v", items)
	}
}

func TestScanCanonicalizesConfiguredRootSymlink(testContext *testing.T) {
	root := testContext.TempDir()
	actualRoot := filepath.Join(root, "actual-skills")
	write(testContext, filepath.Join(actualRoot, "local", "SKILL.md"), "---\nname: linked-root-skill\n---\n")
	configuredRoot := filepath.Join(root, "skills")
	if operationError := os.Symlink(actualRoot, configuredRoot); operationError != nil {
		testContext.Fatal(operationError)
	}
	items, operationError := Scan([]providerapi.SkillRoot{
		declaredRoot(configuredRoot, providerapi.SkillRootKindSkill, providerapi.SkillRootSourceUser),
	})
	if operationError != nil {
		testContext.Fatal(operationError)
	}
	canonicalRoot, operationError := filepath.EvalSymlinks(actualRoot)
	if operationError != nil {
		testContext.Fatal(operationError)
	}
	if len(items) != 1 || items[0].Name != "linked-root-skill" || !strings.HasPrefix(items[0].Path, canonicalRoot) {
		testContext.Fatalf("canonical root scan = %#v", items)
	}
}

func TestScanUsesDeclaredSourceInsteadOfWorkspacePathSegments(testContext *testing.T) {
	workspaceRoot := filepath.Join(testContext.TempDir(), "plugins", "project", "skills")
	write(testContext, filepath.Join(workspaceRoot, "local", "SKILL.md"), "---\nname: workspace-skill\n---\n")
	items, operationError := Scan([]providerapi.SkillRoot{
		declaredRoot(workspaceRoot, providerapi.SkillRootKindSkill, providerapi.SkillRootSourceUser),
	})
	if operationError != nil {
		testContext.Fatal(operationError)
	}
	if len(items) != 1 || items[0].Source != string(providerapi.SkillRootSourceUser) {
		testContext.Fatalf("declared workspace source ignored: %#v", items)
	}
}

func TestScanAcceptsDeclaredCommandRootWithoutCommandsPathSegment(testContext *testing.T) {
	commandRoot := filepath.Join(testContext.TempDir(), "slash-actions")
	write(testContext, filepath.Join(commandRoot, "deploy.md"), "---\ndescription: deploy command\n---\n")
	items, operationError := Scan([]providerapi.SkillRoot{
		declaredRoot(commandRoot, providerapi.SkillRootKindCommand, providerapi.SkillRootSourceCommand),
	})
	if operationError != nil {
		testContext.Fatal(operationError)
	}
	if len(items) != 1 || items[0].Name != "deploy" || items[0].Kind != string(providerapi.SkillRootKindCommand) {
		testContext.Fatalf("declared command root was not scanned: %#v", items)
	}
}
