package gitapi

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rezoch340/any-aicli-remote/backend/internal/fsapi"
)

func TestGitStatusDiffLogAndProject(testContext *testing.T) {
	git, operationError := exec.LookPath("git")
	if operationError != nil {
		testContext.Skip("git not installed")
	}
	root := testContext.TempDir()
	runGit(testContext, git, root, "init", "-q")
	runGit(testContext, git, root, "config", "user.email", "test@example.com")
	runGit(testContext, git, root, "config", "user.name", "Test")
	writeFile(testContext, filepath.Join(root, "README.md"), "# Project\n")
	runGit(testContext, git, root, "add", "README.md")
	runGit(testContext, git, root, "commit", "-q", "-m", "initial commit")
	writeFile(testContext, filepath.Join(root, "README.md"), "# Project\nchanged\n")
	writeFile(testContext, filepath.Join(root, "new.txt"), "new\n")

	service := NewWithGit(func() string { return root }, git)
	operationContext := context.Background()
	status, operationError := service.Status(operationContext)
	if operationError != nil {
		testContext.Fatal(operationError)
	}
	if !status.OK || !status.Git || status.Branch == "" || status.CommitHash == "" || status.Dirty != 2 {
		testContext.Fatalf("status = %#v", status)
	}
	if len(status.Files) != 2 {
		testContext.Fatalf("dirty files = %#v", status.Files)
	}

	diff, operationError := service.Diff(operationContext, "README.md", false)
	if operationError != nil {
		testContext.Fatal(operationError)
	}
	if !diff.OK || diff.Code != 0 || !strings.Contains(diff.Diff, "+changed") {
		testContext.Fatalf("diff = %#v", diff)
	}
	runGit(testContext, git, root, "add", "README.md")
	staged, operationError := service.Diff(operationContext, filepath.Join(root, "README.md"), true)
	if operationError != nil {
		testContext.Fatal(operationError)
	}
	if staged.Path != "README.md" || !strings.Contains(staged.Diff, "+changed") {
		testContext.Fatalf("staged diff = %#v", staged)
	}

	log, operationError := service.Log(operationContext, 100)
	if operationError != nil {
		testContext.Fatal(operationError)
	}
	if len(log.Commits) != 1 || log.Commits[0].Subject != "initial commit" {
		testContext.Fatalf("log = %#v", log)
	}
	project, operationError := service.Project(operationContext)
	if operationError != nil {
		testContext.Fatal(operationError)
	}
	if !project.OK || project.Root != root || project.Branch == nil || len(project.Files) != 1 {
		testContext.Fatalf("project = %#v", project)
	}
	if project.Files[0].Name != "README.md" || project.Files[0].Preview != "# Project\nchanged\n" {
		testContext.Fatalf("context file = %#v", project.Files[0])
	}
}

func TestStatusOutsideGitRepository(testContext *testing.T) {
	git, operationError := exec.LookPath("git")
	if operationError != nil {
		testContext.Skip("git not installed")
	}
	root := testContext.TempDir()
	status, operationError := NewWithGit(func() string { return root }, git).Status(context.Background())
	if operationError != nil {
		testContext.Fatal(operationError)
	}
	if !status.OK || status.Git || status.Root != root {
		testContext.Fatalf("status = %#v", status)
	}
}

func TestGitPathAndWorkspaceValidation(testContext *testing.T) {
	root := testContext.TempDir()
	service := NewWithGit(func() string { return root }, "/missing/git")
	if _, operationError := service.Status(context.Background()); operationError == nil {
		testContext.Fatal("missing git executable succeeded")
	}
	if _, operationError := service.Diff(context.Background(), "../outside", false); !errors.Is(operationError, PathOutsideWorkspaceError) {
		testContext.Fatalf("outside diff error = %v", operationError)
	}
	if _, operationError := service.Diff(context.Background(), filepath.Join(testContext.TempDir(), "outside"), false); !errors.Is(operationError, PathOutsideWorkspaceError) {
		testContext.Fatalf("absolute outside diff error = %v", operationError)
	}
	if _, operationError := NewWithGit(nil, "git").Status(context.Background()); !errors.Is(operationError, WorkspaceUnavailableError) {
		testContext.Fatalf("nil workspace error = %v", operationError)
	}
}

func TestProjectSkipsEscapingSymlink(testContext *testing.T) {
	git, operationError := exec.LookPath("git")
	if operationError != nil {
		testContext.Skip("git not installed")
	}
	root := testContext.TempDir()
	outside := filepath.Join(testContext.TempDir(), "AGENTS.md")
	writeFile(testContext, outside, "secret")
	if operationError := os.Symlink(outside, filepath.Join(root, "AGENTS.md")); operationError != nil {
		testContext.Fatal(operationError)
	}
	project, operationError := NewWithGit(func() string { return root }, git).Project(context.Background())
	if operationError != nil {
		testContext.Fatal(operationError)
	}
	if len(project.Files) != 0 {
		testContext.Fatalf("files = %#v", project.Files)
	}
}

func TestPinnedGitWorkspaceRejectsReplacement(testContext *testing.T) {
	gitPath, operationError := exec.LookPath("git")
	if operationError != nil {
		testContext.Skip("git not installed")
	}
	parentDirectory := testContext.TempDir()
	workspacePath := filepath.Join(parentDirectory, "workspace")
	originalPath := filepath.Join(parentDirectory, "workspace-original")
	replacementPath := testContext.TempDir()
	if operationError := os.Mkdir(workspacePath, 0o755); operationError != nil {
		testContext.Fatal(operationError)
	}
	rootIdentity, operationError := fsapi.PinRoot(workspacePath)
	if operationError != nil {
		testContext.Fatal(operationError)
	}
	service := NewWithPinnedGit(rootIdentity, gitPath)
	if operationError := os.Rename(workspacePath, originalPath); operationError != nil {
		testContext.Fatal(operationError)
	}
	if operationError := os.Symlink(replacementPath, workspacePath); operationError != nil {
		testContext.Fatal(operationError)
	}
	if _, operationError := service.Status(context.Background()); !errors.Is(operationError, WorkspaceUnavailableError) {
		testContext.Fatalf("git status replacement error = %v", operationError)
	}
	if _, operationError := service.Project(context.Background()); !errors.Is(operationError, WorkspaceUnavailableError) {
		testContext.Fatalf("git project replacement error = %v", operationError)
	}
}

func TestParseStatusAndTruncate(testContext *testing.T) {
	head := "## main...origin/main [ahead 12, behind 3]"
	if got := parseStatusCount(head, `ahead\s+(\d+)`); got != 12 {
		testContext.Fatal(got)
	}
	if got := parseStatusCount(head, `behind\s+(\d+)`); got != 3 {
		testContext.Fatal(got)
	}
	if got := truncateRunes("你好abc", 3); got != "你好a" {
		testContext.Fatal(got)
	}
}

func runGit(testContext *testing.T, git, directory string, arguments ...string) {
	testContext.Helper()
	command := exec.Command(git, arguments...)
	command.Dir = directory
	if out, operationError := command.CombinedOutput(); operationError != nil {
		testContext.Fatalf("git %s: %v\n%s", strings.Join(arguments, " "), operationError, out)
	}
}

func writeFile(testContext *testing.T, path, content string) {
	testContext.Helper()
	if operationError := os.WriteFile(path, []byte(content), 0o644); operationError != nil {
		testContext.Fatal(operationError)
	}
}
