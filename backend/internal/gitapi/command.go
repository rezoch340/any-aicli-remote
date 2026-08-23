// Git command execution. Every invocation runs with a bounded timeout and
// bounded output inside the resolved workspace root.

package gitapi

import (
	"context"
	"errors"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type commandResult struct {
	stdout string
	code   int
}

type boundedOutputWriter struct {
	limit int64
	data  []byte
}

func (writer *boundedOutputWriter) Write(data []byte) (int, error) {
	remaining := writer.limit - int64(len(writer.data))
	if remaining > 0 {
		copyLength := min(int64(len(data)), remaining)
		writer.data = append(writer.data, data[:int(copyLength)]...)
	}
	return len(data), nil
}

func (service *Service) run(operationContext context.Context, timeout time.Duration, root string, arguments ...string) (commandResult, error) {
	if strings.TrimSpace(service.gitPath) == "" {
		return commandResult{}, GitNotFoundError
	}
	runContext, cancel := context.WithTimeout(operationContext, timeout)
	defer cancel()
	command := exec.CommandContext(runContext, service.gitPath, arguments...)
	command.Dir = root
	if service.rootIdentity != nil {
		if operationError := service.rootIdentity.Validate(); operationError != nil {
			return commandResult{}, WorkspaceUnavailableError
		}
	}
	stdoutWriter := &boundedOutputWriter{limit: service.policy.CommandOutputMaxBytes}
	command.Stdout = stdoutWriter
	operationError := command.Run()
	if runContext.Err() != nil {
		return commandResult{}, runContext.Err()
	}
	if operationError == nil {
		return commandResult{stdout: decodeUTF8(stdoutWriter.data), code: 0}, nil
	}
	var exitError *exec.ExitError
	if errors.As(operationError, &exitError) {
		return commandResult{stdout: decodeUTF8(stdoutWriter.data), code: exitError.ExitCode()}, nil
	}
	return commandResult{}, operationError
}

func safePathspec(root, raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "." {
		return "", nil
	}
	var relativePath string
	if filepath.IsAbs(raw) {
		var operationError error
		relativePath, operationError = filepath.Rel(root, filepath.Clean(raw))
		if operationError != nil {
			return "", PathOutsideWorkspaceError
		}
	} else {
		relativePath = filepath.Clean(raw)
	}
	if relativePath == ".." || strings.HasPrefix(relativePath, ".."+string(filepath.Separator)) || filepath.IsAbs(relativePath) {
		return "", PathOutsideWorkspaceError
	}
	return filepath.ToSlash(relativePath), nil
}
