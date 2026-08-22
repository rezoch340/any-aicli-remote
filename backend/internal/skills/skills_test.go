package skills

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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

func TestParseFrontmatter(testContext *testing.T) {
	meta, body := ParseFrontmatter("---\nname: 'remote'\ndescription: |\n  first\n  second\nuser-invocable: false\n---\n# Body")
	if meta["name"] != "remote" || meta["description"] != "first second" || meta["user-invocable"] != "false" || body != "# Body" {
		testContext.Fatalf("meta=%#v body=%q", meta, body)
	}
}

func TestScanSkillsAndCommands(testContext *testing.T) {
	root := testContext.TempDir()
	home := filepath.Join(root, "home")
	workingDirectory := filepath.Join(root, "work")
	testContext.Setenv("HOME", home)
	testContext.Setenv("USERPROFILE", home)

	write(testContext, filepath.Join(home, ".grok", "bundled", "skills", "b", "SKILL.md"), "---\nname: bundled-one\ndescription: b\n---\n")
	write(testContext, filepath.Join(home, ".grok", "installed-plugins", "plug", "skills", "p", "SKILL.md"), "---\nname: plug-one\ndescription: "+strings.Repeat("x", 250)+"\nwhen-to-use: now\nargument-hint: arg\nuser-invocable: no\n---\n")
	write(testContext, filepath.Join(workingDirectory, "skills", "local", "SKILL.md"), "---\ndescription: local skill\n---\n")
	write(testContext, filepath.Join(workingDirectory, "commands", "remote.md"), "---\nname: /remote\ndescription: command\nargument-hint: cmdarg\n---\n")
	write(testContext, filepath.Join(workingDirectory, "skills", "node_modules", "bad", "SKILL.md"), "---\nname: bad\n---\n")

	items, operationError := Scan(workingDirectory)
	if operationError != nil {
		testContext.Fatal(operationError)
	}
	if len(items) != 4 {
		testContext.Fatalf("len=%d items=%#v", len(items), items)
	}
	names := []string{items[0].Name, items[1].Name, items[2].Name, items[3].Name}
	want := []string{"bundled-one", "local", "plug-one", "remote"}
	for itemIndex := range want {
		if names[itemIndex] != want[itemIndex] {
			testContext.Fatalf("order/name got %v want %v items=%#v", names, want, items)
		}
	}
	plug := items[2]
	if plug.Source != "plugin" || plug.UserInvocable || len([]rune(plug.Description)) != 240 || !strings.HasSuffix(plug.Description, "...") || plug.When != "now" || plug.Hint != "arg" {
		testContext.Fatalf("bad plugin item: %#v", plug)
	}
	command := items[3]
	if command.Kind != "command" || command.Source != "command" || command.Invoke != "/remote" || command.Hint != "cmdarg" {
		testContext.Fatalf("bad command: %#v", command)
	}
}

func TestScanDedupCaseInsensitive(testContext *testing.T) {
	root := testContext.TempDir()
	home := filepath.Join(root, "home")
	workingDirectory := filepath.Join(root, "work")
	testContext.Setenv("HOME", home)
	write(testContext, filepath.Join(home, ".grok", "skills", "a", "SKILL.md"), "---\nname: Same\n---\n")
	write(testContext, filepath.Join(workingDirectory, "skills", "b", "SKILL.md"), "---\nname: same\n---\n")
	items, operationError := Scan(workingDirectory)
	if operationError != nil {
		testContext.Fatal(operationError)
	}
	if len(items) != 1 || items[0].Name != "Same" {
		testContext.Fatalf("dedup failed: %#v", items)
	}
}
