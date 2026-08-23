package terminalexec_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/rezoch340/any-aicli-remote/backend/internal/fsapi"
	"github.com/rezoch340/any-aicli-remote/backend/internal/terminalexec"
)

func TestCommandPinsOpenedDirectory(testInstance *testing.T) {
	parentDirectory := testInstance.TempDir()
	workspaceDirectory := filepath.Join(parentDirectory, "workspace")
	insideDirectory := filepath.Join(workspaceDirectory, "inside")
	outsideDirectory := filepath.Join(parentDirectory, "outside")
	for _, directoryPath := range []string{workspaceDirectory, insideDirectory, outsideDirectory} {
		if operationError := os.Mkdir(directoryPath, 0o755); operationError != nil {
			testInstance.Fatal(operationError)
		}
	}
	linkPath := filepath.Join(workspaceDirectory, "cwd")
	if operationError := os.Symlink("inside", linkPath); operationError != nil {
		testInstance.Fatal(operationError)
	}
	filesystem, operationError := fsapi.New(workspaceDirectory)
	if operationError != nil {
		testInstance.Fatal(operationError)
	}
	defer filesystem.Close()
	directoryFile, operationError := filesystem.OpenDirectory("cwd")
	if operationError != nil {
		testInstance.Fatal(operationError)
	}
	defer directoryFile.Close()
	if operationError := os.Remove(linkPath); operationError != nil {
		testInstance.Fatal(operationError)
	}
	if operationError := os.Symlink("../outside", linkPath); operationError != nil {
		testInstance.Fatal(operationError)
	}
	commandProcess, operationError := terminalexec.Command("printf pinned > marker.txt", nil, directoryFile)
	if operationError != nil {
		testInstance.Fatal(operationError)
	}
	if operationError := commandProcess.Run(); operationError != nil {
		testInstance.Fatal(operationError)
	}
	assertFileExists(testInstance, filepath.Join(insideDirectory, "marker.txt"))
	assertFileAbsent(testInstance, filepath.Join(outsideDirectory, "marker.txt"))

	argumentProcess, operationError := terminalexec.Command("/bin/sh", []string{"-c", "printf direct > direct.txt"}, directoryFile)
	if operationError != nil {
		testInstance.Fatal(operationError)
	}
	if operationError := argumentProcess.Run(); operationError != nil {
		testInstance.Fatal(operationError)
	}
	assertFileExists(testInstance, filepath.Join(insideDirectory, "direct.txt"))
}

func assertFileExists(testInstance *testing.T, filePath string) {
	testInstance.Helper()
	if _, operationError := os.Stat(filePath); operationError != nil {
		testInstance.Fatal(operationError)
	}
}

func assertFileAbsent(testInstance *testing.T, filePath string) {
	testInstance.Helper()
	if _, operationError := os.Stat(filePath); !os.IsNotExist(operationError) {
		testInstance.Fatalf("unexpected file %s: %v", filePath, operationError)
	}
}
