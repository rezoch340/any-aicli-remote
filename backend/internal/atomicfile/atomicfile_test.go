package atomicfile

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWritePrivateCreatesPrivateNestedReplacement(testingContext *testing.T) {
	path := filepath.Join(testingContext.TempDir(), "nested", "state.json")
	if operationError := WritePrivate(path, []byte("first")); operationError != nil {
		testingContext.Fatal(operationError)
	}
	if operationError := WritePrivate(path, []byte("second")); operationError != nil {
		testingContext.Fatal(operationError)
	}
	data, operationError := os.ReadFile(path)
	if operationError != nil {
		testingContext.Fatal(operationError)
	}
	if string(data) != "second" {
		testingContext.Fatalf("data = %q", data)
	}
	information, operationError := os.Stat(path)
	if operationError != nil {
		testingContext.Fatal(operationError)
	}
	if information.Mode().Perm() != 0o600 {
		testingContext.Fatalf("mode = %o", information.Mode().Perm())
	}
	parent, operationError := os.Stat(filepath.Dir(path))
	if operationError != nil {
		testingContext.Fatal(operationError)
	}
	if parent.Mode().Perm() != 0o700 {
		testingContext.Fatalf("parent mode = %o", parent.Mode().Perm())
	}
}

func TestWritePrivateFailurePreservesTargetAndCleansTemporaryFile(testingContext *testing.T) {
	parent := testingContext.TempDir()
	path := filepath.Join(parent, "target")
	if operationError := os.Mkdir(path, 0o700); operationError != nil {
		testingContext.Fatal(operationError)
	}
	if operationError := WritePrivate(path, []byte("new")); operationError == nil {
		testingContext.Fatal("WritePrivate replaced a directory")
	}
	information, operationError := os.Stat(path)
	if operationError != nil {
		testingContext.Fatal(operationError)
	}
	if !information.IsDir() {
		testingContext.Fatal("original target was replaced")
	}
	entries, operationError := os.ReadDir(parent)
	if operationError != nil {
		testingContext.Fatal(operationError)
	}
	if len(entries) != 1 || entries[0].Name() != "target" {
		testingContext.Fatalf("temporary files remain: %#v", entries)
	}
}
