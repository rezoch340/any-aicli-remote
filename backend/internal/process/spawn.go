// Process launch: environment merging, argument redaction, and the detached
// spawn. Secrets never reach process arguments or the log file.

package process

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
)

func mergeEnvironment(baseEnvironment, overrides []string) []string {
	overriddenKeys := make(map[string]struct{}, len(overrides))
	for _, entry := range overrides {
		key, _, present := strings.Cut(entry, "=")
		if present {
			overriddenKeys[normalizedEnvironmentKey(key)] = struct{}{}
		}
	}
	merged := make([]string, 0, len(baseEnvironment)+len(overrides))
	for _, entry := range baseEnvironment {
		key, _, present := strings.Cut(entry, "=")
		if present {
			if _, overridden := overriddenKeys[normalizedEnvironmentKey(key)]; overridden {
				continue
			}
		}
		merged = append(merged, entry)
	}
	return append(merged, overrides...)
}

func normalizedEnvironmentKey(key string) string {
	if runtime.GOOS == "windows" {
		return strings.ToUpper(key)
	}
	return key
}

func redactArguments(arguments []string, secret string) []string {
	output := append([]string(nil), arguments...)
	if secret == "" {
		return output
	}
	for argumentIndex, argument := range output {
		if argument == secret {
			output[argumentIndex] = "***"
		} else if strings.Contains(argument, secret) {
			output[argumentIndex] = strings.ReplaceAll(argument, secret, "***")
		}
	}
	return output
}

// StartProcess starts the process detached enough for a macOS menu-bar/daemon parent.
func StartProcess(specification StartSpecification) (int, error) {
	logPath := specification.LogPath
	if logPath == "" {
		logPath = filepath.Join(os.TempDir(), "provider-agent.spawn.log")
	}
	if errorValue := os.MkdirAll(filepath.Dir(logPath), 0o700); errorValue != nil {
		return 0, errorValue
	}
	logFile, errorValue := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if errorValue != nil {
		return 0, errorValue
	}
	if errorValue := os.Chmod(logPath, 0o600); errorValue != nil {
		_ = logFile.Close()
		return 0, errorValue
	}
	logWriter := newLiteralRedactingWriter(logFile, specification.SensitiveValues)
	commandValue := exec.Command(specification.Path, specification.Arguments...)
	commandValue.Dir = specification.WorkingDirectory
	commandValue.Env = specification.Environment
	commandValue.Stdout = logWriter
	commandValue.Stderr = logWriter
	commandValue.Stdin = nil
	if runtime.GOOS != "windows" {
		commandValue.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	}
	if errorValue := commandValue.Start(); errorValue != nil {
		_ = logWriter.Close()
		_ = logFile.Close()
		return 0, errorValue
	}
	go func() {
		_ = commandValue.Wait()
		_ = logWriter.Close()
		_ = logFile.Close()
	}()
	return commandValue.Process.Pid, nil
}
