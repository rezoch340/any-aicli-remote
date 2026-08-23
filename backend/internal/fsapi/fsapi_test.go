package fsapi

import (
	"errors"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var testFilesystemPolicy = Policy{MaxReadBytes: 1024, MaxWriteBytes: 1024, MaxListItems: 10}

func TestServiceReadWriteListAndMkdir(testContext *testing.T) {
	root := testContext.TempDir()
	mustWrite(testContext, filepath.Join(root, "README.md"), []byte("hello"))
	mustWrite(testContext, filepath.Join(root, "image.png"), []byte{0, 1, 2})
	mustWrite(testContext, filepath.Join(root, ".env"), []byte("A=1"))
	mustWrite(testContext, filepath.Join(root, ".hidden"), []byte("no"))
	if operationError := os.Mkdir(filepath.Join(root, "src"), 0o755); operationError != nil {
		testContext.Fatal(operationError)
	}
	if operationError := os.Mkdir(filepath.Join(root, "node_modules"), 0o755); operationError != nil {
		testContext.Fatal(operationError)
	}

	service, operationError := New(root, testFilesystemPolicy)
	if operationError != nil {
		testContext.Fatal(operationError)
	}
	testContext.Cleanup(func() { _ = service.Close() })

	listing, operationError := service.List(".")
	if operationError != nil {
		testContext.Fatal(operationError)
	}
	if len(listing.Directories) != 1 || listing.Directories[0].Name != "src" {
		testContext.Fatalf("dirs = %#v", listing.Directories)
	}
	if len(listing.Files) != 3 {
		testContext.Fatalf("files = %#v", listing.Files)
	}
	if listing.Files[0].Name != ".env" || listing.Files[1].Name != "image.png" || listing.Files[2].Name != "README.md" {
		testContext.Fatalf("file order = %#v", listing.Files)
	}

	text, operationError := service.Read("README.md")
	if operationError != nil {
		testContext.Fatal(operationError)
	}
	if !text.Text || text.Content != "hello" || text.Binary {
		testContext.Fatalf("text result = %#v", text)
	}
	binary, operationError := service.Read("image.png")
	if operationError != nil {
		testContext.Fatal(operationError)
	}
	if binary.Text || !binary.Binary || binary.Content != "" {
		testContext.Fatalf("binary result = %#v", binary)
	}

	written, operationError := service.Write("nested/note.txt", "new\ntext")
	if operationError != nil {
		testContext.Fatal(operationError)
	}
	if !written.OK || written.RelativePath != "nested/note.txt" || written.Size != 8 {
		testContext.Fatalf("write result = %#v", written)
	}
	if got, operationError := os.ReadFile(filepath.Join(root, "nested", "note.txt")); operationError != nil || string(got) != "new\ntext" {
		testContext.Fatalf("written data = %q, %v", got, operationError)
	}
	created, operationError := service.Mkdir("nested/deeper")
	if operationError != nil {
		testContext.Fatal(operationError)
	}
	if created != filepath.Join(root, "nested", "deeper") {
		testContext.Fatal(created)
	}

	nested, operationError := service.List("nested")
	if operationError != nil {
		testContext.Fatal(operationError)
	}
	if nested.RelativePath != "nested" || nested.Parent == nil || *nested.Parent != "." {
		testContext.Fatalf("nested listing = %#v", nested)
	}
}

func TestServiceRejectsWorkspaceEscapeIncludingSymlink(testContext *testing.T) {
	root := testContext.TempDir()
	outside := testContext.TempDir()
	mustWrite(testContext, filepath.Join(outside, "secret.txt"), []byte("secret"))
	if operationError := os.Symlink(outside, filepath.Join(root, "escape")); operationError != nil {
		testContext.Fatal(operationError)
	}
	service, operationError := New(root, testFilesystemPolicy)
	if operationError != nil {
		testContext.Fatal(operationError)
	}
	testContext.Cleanup(func() { _ = service.Close() })

	for _, path := range []string{"../secret.txt", filepath.Join(outside, "secret.txt")} {
		if _, operationError := service.Read(path); !errors.Is(operationError, OutsideWorkspaceError) {
			testContext.Fatalf("Read(%q) error = %v", path, operationError)
		}
	}
	if _, operationError := service.Read("escape/secret.txt"); operationError == nil {
		testContext.Fatal("read through escaping symlink succeeded")
	}
	if _, operationError := service.Write("escape/new.txt", "bad"); operationError == nil {
		testContext.Fatal("write through escaping symlink succeeded")
	}
	if _, operationError := service.OpenRead("escape/secret.txt"); operationError == nil {
		testContext.Fatal("open through escaping symlink succeeded")
	}
	if _, operationError := service.OpenDirectory("escape"); operationError == nil {
		testContext.Fatal("open directory through escaping symlink succeeded")
	}
	listing, operationError := service.List(".")
	if operationError != nil {
		testContext.Fatal(operationError)
	}
	for _, item := range append(listing.Directories, listing.Files...) {
		if item.Name == "escape" {
			testContext.Fatal("escaping symlink was exposed in listing")
		}
	}
}

func TestServiceSizeLimitsAndDecoding(testContext *testing.T) {
	root := testContext.TempDir()
	mustWrite(testContext, filepath.Join(root, "bom.txt"), append([]byte{0xef, 0xbb, 0xbf}, []byte("hello")...))
	mustWrite(testContext, filepath.Join(root, "latin.txt"), []byte{'c', 'a', 'f', 0xe9})
	mustWrite(testContext, filepath.Join(root, "large.txt"), make([]byte, testFilesystemPolicy.MaxReadBytes+1))
	service, operationError := New(root, testFilesystemPolicy)
	if operationError != nil {
		testContext.Fatal(operationError)
	}
	testContext.Cleanup(func() { _ = service.Close() })

	result, operationError := service.Read("bom.txt")
	if operationError != nil || result.Content != "hello" {
		testContext.Fatalf("BOM decode = %#v, %v", result, operationError)
	}
	result, operationError = service.Read("latin.txt")
	if operationError != nil || result.Content != "café" {
		testContext.Fatalf("latin-1 decode = %#v, %v", result, operationError)
	}
	if _, operationError := service.Read("large.txt"); !errors.Is(operationError, FileTooLargeError) {
		testContext.Fatalf("large read error = %v", operationError)
	}
	if _, operationError := service.Write("too-large.txt", strings.Repeat("x", int(testFilesystemPolicy.MaxWriteBytes+1))); !errors.Is(operationError, ContentTooLargeError) {
		testContext.Fatalf("large write error = %v", operationError)
	}
}

func TestSetRoot(testContext *testing.T) {
	first := testContext.TempDir()
	second := testContext.TempDir()
	service, operationError := New(first, testFilesystemPolicy)
	if operationError != nil {
		testContext.Fatal(operationError)
	}
	testContext.Cleanup(func() { _ = service.Close() })
	info, operationError := service.SetRoot(second)
	if operationError != nil {
		testContext.Fatal(operationError)
	}
	if info.Root != second || service.Root() != second || !service.Info().Exists {
		testContext.Fatalf("root = %#v / %q", info, service.Root())
	}
	file := filepath.Join(first, "file")
	mustWrite(testContext, file, []byte("old"))
	if _, operationError := service.Read(file); !errors.Is(operationError, OutsideWorkspaceError) {
		testContext.Fatalf("old absolute root error = %v", operationError)
	}
}

func TestPinnedWorkspaceFailsClosedAfterRootReplacement(testContext *testing.T) {
	parentDirectory := testContext.TempDir()
	workspacePath := filepath.Join(parentDirectory, "workspace")
	originalPath := filepath.Join(parentDirectory, "workspace-original")
	outsidePath := testContext.TempDir()
	if operationError := os.Mkdir(workspacePath, 0o755); operationError != nil {
		testContext.Fatal(operationError)
	}
	mustWrite(testContext, filepath.Join(workspacePath, "inside.txt"), []byte("inside"))
	mustWrite(testContext, filepath.Join(outsidePath, "secret.txt"), []byte("secret"))
	rootIdentity, operationError := PinRoot(workspacePath)
	if operationError != nil {
		testContext.Fatal(operationError)
	}
	service, operationError := NewPinned(rootIdentity, testFilesystemPolicy)
	if operationError != nil {
		testContext.Fatal(operationError)
	}
	defer service.Close()
	if operationError := os.Rename(workspacePath, originalPath); operationError != nil {
		testContext.Fatal(operationError)
	}
	if operationError := os.Symlink(outsidePath, workspacePath); operationError != nil {
		testContext.Fatal(operationError)
	}
	if _, operationError := service.Read("secret.txt"); !errors.Is(operationError, WorkspaceChangedError) {
		testContext.Fatalf("symlink replacement read error = %v", operationError)
	}
	if _, operationError := service.Write("written.txt", "bad"); !errors.Is(operationError, WorkspaceChangedError) {
		testContext.Fatalf("symlink replacement write error = %v", operationError)
	}
	if _, operationError := os.Stat(filepath.Join(outsidePath, "written.txt")); !errors.Is(operationError, os.ErrNotExist) {
		testContext.Fatalf("replacement target was modified: %v", operationError)
	}
	if operationError := os.Remove(workspacePath); operationError != nil {
		testContext.Fatal(operationError)
	}
	if operationError := os.Mkdir(workspacePath, 0o755); operationError != nil {
		testContext.Fatal(operationError)
	}
	if _, operationError := service.List("."); !errors.Is(operationError, WorkspaceChangedError) {
		testContext.Fatalf("different-directory replacement list error = %v", operationError)
	}
}

func mustWrite(testContext *testing.T, path string, data []byte) {
	testContext.Helper()
	if operationError := os.WriteFile(path, data, 0o644); operationError != nil {
		testContext.Fatal(operationError)
	}
}

func TestPolicyRejectsInvalidSentinelLimits(testContext *testing.T) {
	if operationError := (Policy{MaxReadBytes: 0, MaxWriteBytes: 1, MaxListItems: 1}).Validate(); operationError == nil {
		testContext.Fatal("zero read limit accepted")
	}
	if operationError := (Policy{MaxReadBytes: 1, MaxWriteBytes: math.MaxInt64, MaxListItems: 1}).Validate(); operationError == nil {
		testContext.Fatal("write sentinel overflow accepted")
	}
	if _, operationError := New(testContext.TempDir(), Policy{MaxReadBytes: math.MaxInt64, MaxWriteBytes: 1, MaxListItems: 1}); operationError == nil {
		testContext.Fatal("read sentinel overflow accepted")
	}
	if operationError := (Policy{MaxReadBytes: 1, MaxWriteBytes: 1, MaxListItems: 0}).Validate(); operationError == nil {
		testContext.Fatal("zero list limit accepted")
	}
	if operationError := (Policy{MaxReadBytes: 1, MaxWriteBytes: 1, MaxListItems: int(^uint(0) >> 1)}).Validate(); operationError == nil {
		testContext.Fatal("list sentinel overflow accepted")
	}
}

func TestListEnforcesItemLimit(testContext *testing.T) {
	root := testContext.TempDir()
	mustWrite(testContext, filepath.Join(root, "one.txt"), []byte("one"))
	service, operationError := New(root, Policy{MaxReadBytes: 1024, MaxWriteBytes: 1024, MaxListItems: 1})
	if operationError != nil {
		testContext.Fatal(operationError)
	}
	testContext.Cleanup(func() { _ = service.Close() })
	if listing, operationError := service.List("."); operationError != nil || len(listing.Files) != 1 {
		testContext.Fatalf("one entry listing = %#v, %v", listing, operationError)
	}
	mustWrite(testContext, filepath.Join(root, "two.txt"), []byte("two"))
	if _, operationError := service.List("."); !errors.Is(operationError, DirectoryListingTooLargeError) {
		testContext.Fatalf("two entry listing error = %v", operationError)
	}
}
