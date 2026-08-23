package terminalexec

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/moby/sys/reexec"
	"golang.org/x/sys/unix"
)

const (
	handlerName          = "__any-aicli-remote-terminal-exec-v1"
	inheritedDirectoryFD = 3
	shellPath            = "/bin/sh"
	failureExitCode      = 127
)

func init() {
	reexec.Register(handlerName, runTerminalInitializer)
	if reexec.Init() {
		os.Exit(0)
	}
}

func Command(command string, arguments []string, workingDirectoryFile *os.File) (*exec.Cmd, error) {
	targetExecutable, targetArguments := command, arguments
	if len(arguments) == 0 {
		targetExecutable, targetArguments = shellPath, []string{"-lc", command}
	}
	targetCommand := exec.Command(targetExecutable, targetArguments...)
	if targetCommand.Err != nil {
		return nil, targetCommand.Err
	}
	initializerArguments := append([]string{handlerName, targetCommand.Path, targetCommand.Args[0]}, targetCommand.Args[1:]...)
	commandProcess := reexec.Command(initializerArguments...)
	commandProcess.ExtraFiles = []*os.File{workingDirectoryFile}
	return commandProcess, nil
}

func runTerminalInitializer() {
	if len(os.Args) < 3 {
		fmt.Fprintf(os.Stderr, "terminal exec argument setup: insufficient arguments: %v\n", len(os.Args))
		os.Exit(failureExitCode)
	}
	if operationError := unix.Fchdir(inheritedDirectoryFD); operationError != nil {
		fmt.Fprintf(os.Stderr, "terminal exec directory setup: %v\n", operationError)
		os.Exit(failureExitCode)
	}
	unix.CloseOnExec(inheritedDirectoryFD)
	if operationError := unix.Exec(os.Args[1], os.Args[2:], os.Environ()); operationError != nil {
		fmt.Fprintf(os.Stderr, "terminal exec launch: %v\n", operationError)
		os.Exit(failureExitCode)
	}
}
