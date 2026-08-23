package gitapi

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/rezoch340/any-aicli-remote/backend/internal/fsapi"
)

const boundedOutputFixtureTimeout = 15 * time.Second

var testGitPolicy = Policy{CommandTimeout: time.Second, DiffTimeout: time.Second, DirtyFileLimit: 80, DiffRuneLimit: 200000, LogDefaultLimit: 12, LogMaxLimit: 30, ContextFileReadBytes: 16000, ContextPreviewRunes: 4000, CommandOutputMaxBytes: 16 << 20}

func mustNewWithGit(testContext *testing.T, workspace func() string, gitPath string) *Service {
	testContext.Helper()
	service, operationError := NewWithGit(workspace, gitPath, testGitPolicy)
	if operationError != nil {
		testContext.Fatal(operationError)
	}
	return service
}

func mustNewWithPinnedGit(testContext *testing.T, identity *fsapi.RootIdentity, gitPath string) *Service {
	testContext.Helper()
	service, operationError := NewWithPinnedGit(identity, gitPath, testGitPolicy)
	if operationError != nil {
		testContext.Fatal(operationError)
	}
	return service
}

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

	service := mustNewWithGit(testContext, func() string { return root }, git)
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
	status, operationError := mustNewWithGit(testContext, func() string { return root }, git).Status(context.Background())
	if operationError != nil {
		testContext.Fatal(operationError)
	}
	if !status.OK || status.Git || status.Root != root {
		testContext.Fatalf("status = %#v", status)
	}
}

func TestGitPathAndWorkspaceValidation(testContext *testing.T) {
	root := testContext.TempDir()
	service := mustNewWithGit(testContext, func() string { return root }, "/missing/git")
	if _, operationError := service.Status(context.Background()); operationError == nil {
		testContext.Fatal("missing git executable succeeded")
	}
	if _, operationError := service.Diff(context.Background(), "../outside", false); !errors.Is(operationError, PathOutsideWorkspaceError) {
		testContext.Fatalf("outside diff error = %v", operationError)
	}
	if _, operationError := service.Diff(context.Background(), filepath.Join(testContext.TempDir(), "outside"), false); !errors.Is(operationError, PathOutsideWorkspaceError) {
		testContext.Fatalf("absolute outside diff error = %v", operationError)
	}
	if _, operationError := mustNewWithGit(testContext, nil, "git").Status(context.Background()); !errors.Is(operationError, WorkspaceUnavailableError) {
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
	project, operationError := mustNewWithGit(testContext, func() string { return root }, git).Project(context.Background())
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
	service := mustNewWithPinnedGit(testContext, rootIdentity, gitPath)
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

func TestCustomPolicyEnforcesGitLimits(testContext *testing.T) {
	gitPath, operationError := exec.LookPath("git")
	if operationError != nil {
		testContext.Skip("git not installed")
	}
	root := testContext.TempDir()
	runGit(testContext, gitPath, root, "init", "-q")
	runGit(testContext, gitPath, root, "config", "user.email", "test@example.com")
	runGit(testContext, gitPath, root, "config", "user.name", "Test")
	for index := 0; index < 3; index++ {
		writeFile(testContext, filepath.Join(root, "README.md"), strings.Repeat("a", index+1))
		runGit(testContext, gitPath, root, "add", "README.md")
		runGit(testContext, gitPath, root, "commit", "-q", "-m", "commit")
	}
	writeFile(testContext, filepath.Join(root, "README.md"), strings.Repeat("change\n", 20))
	writeFile(testContext, filepath.Join(root, "other.txt"), "dirty")
	writeFile(testContext, filepath.Join(root, "package.json"), strings.Repeat("x", 9))
	policy := testGitPolicy
	policy.DirtyFileLimit, policy.DiffRuneLimit = 1, 12
	policy.LogDefaultLimit, policy.LogMaxLimit = 1, 2
	policy.ContextFileReadBytes, policy.ContextPreviewRunes = 8, 3
	service := mustNewWithGitPolicy(testContext, func() string { return root }, gitPath, policy)
	status, operationError := service.Status(context.Background())
	if operationError != nil || len(status.Files) != 1 {
		testContext.Fatalf("dirty cap = %#v, %v", status, operationError)
	}
	diff, operationError := service.Diff(context.Background(), "README.md", false)
	if operationError != nil || len([]rune(diff.Diff)) > policy.DiffRuneLimit {
		testContext.Fatalf("diff cap = %#v, %v", diff, operationError)
	}
	defaultLog, operationError := service.Log(context.Background(), 0)
	if operationError != nil || len(defaultLog.Commits) != 1 {
		testContext.Fatalf("default log cap = %#v, %v", defaultLog, operationError)
	}
	maximumLog, operationError := service.Log(context.Background(), 99)
	if operationError != nil || len(maximumLog.Commits) != 2 {
		testContext.Fatalf("maximum log cap = %#v, %v", maximumLog, operationError)
	}
	project, operationError := service.Project(context.Background())
	if operationError != nil {
		testContext.Fatal(operationError)
	}
	for _, file := range project.Files {
		if file.Name == "package.json" {
			testContext.Fatal("oversized context file leaked")
		}
	}
	writeFile(testContext, filepath.Join(root, "README.md"), "abcdef")
	policy.ContextFileReadBytes, policy.ContextPreviewRunes = 8, 3
	project, operationError = mustNewWithGitPolicy(testContext, func() string { return root }, gitPath, policy).Project(context.Background())
	if operationError != nil {
		testContext.Fatal(operationError)
	}
	for _, file := range project.Files {
		if file.Name == "README.md" && file.Preview != "abc" {
			testContext.Fatalf("preview cap = %#v", file)
		}
	}
}

func TestGitPolicyRejectsInvalidAndTimesOut(testContext *testing.T) {
	if _, operationError := NewWithGit(func() string { return testContext.TempDir() }, "git", Policy{}); operationError == nil {
		testContext.Fatal("zero policy accepted")
	}
	if _, operationError := NewWithGit(func() string { return testContext.TempDir() }, "git", Policy{CommandTimeout: time.Second, DiffTimeout: time.Second, DirtyFileLimit: 1, DiffRuneLimit: 1, LogDefaultLimit: 2, LogMaxLimit: 1, ContextFileReadBytes: 1, ContextPreviewRunes: 1, CommandOutputMaxBytes: 1}); operationError == nil {
		testContext.Fatal("invalid policy accepted")
	}
	if runtime.GOOS == "windows" {
		testContext.Skip("shell fixture is Unix-only")
	}
	root := testContext.TempDir()
	scriptPath := filepath.Join(root, "slow-git")
	writeFile(testContext, scriptPath, "#!/bin/sh\nsleep 1\n")
	if operationError := os.Chmod(scriptPath, 0o755); operationError != nil {
		testContext.Fatal(operationError)
	}
	policy := testGitPolicy
	policy.CommandTimeout = 20 * time.Millisecond
	service := mustNewWithGitPolicy(testContext, func() string { return root }, scriptPath, policy)
	if _, operationError := service.Status(context.Background()); !errors.Is(operationError, context.DeadlineExceeded) {
		testContext.Fatalf("timeout error = %v", operationError)
	}
}

func mustNewWithGitPolicy(testContext *testing.T, workspace func() string, gitPath string, policy Policy) *Service {
	testContext.Helper()
	service, operationError := NewWithGit(workspace, gitPath, policy)
	if operationError != nil {
		testContext.Fatal(operationError)
	}
	return service
}

func TestCommandOutputIsBounded(testContext *testing.T) {
	if runtime.GOOS == "windows" {
		testContext.Skip("shell fixture is Unix-only")
	}
	root := testContext.TempDir()
	scriptPath := filepath.Join(root, "fake-git")
	writeFile(testContext, scriptPath, "#!/bin/sh\nprintf '0123456789abcdef'\n")
	if operationError := os.Chmod(scriptPath, 0o755); operationError != nil {
		testContext.Fatal(operationError)
	}
	policy := testGitPolicy
	policy.CommandOutputMaxBytes = 5
	policy.CommandTimeout = boundedOutputFixtureTimeout
	service := mustNewWithGitPolicy(testContext, func() string { return root }, scriptPath, policy)
	result, operationError := service.run(context.Background(), policy.CommandTimeout, root, "diff")
	if operationError != nil || result.stdout != "01234" {
		testContext.Fatalf("bounded command result = %#v, %v", result, operationError)
	}
	diff, operationError := service.Diff(context.Background(), "", false)
	if operationError != nil || len(diff.Diff) > int(policy.CommandOutputMaxBytes) || diff.Diff != "01234" {
		testContext.Fatalf("bounded public diff = %#v, %v", diff, operationError)
	}
}
