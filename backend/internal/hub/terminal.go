package hub

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"sync"
	"sync/atomic"
	"syscall"
)

type limitedBuffer struct {
	accessMutex sync.Mutex
	data        []byte
	limit       int
	truncated   bool
}

func newLimitedBuffer(limit int) *limitedBuffer {
	if limit < 1024 {
		limit = 1024
	}
	return &limitedBuffer{limit: limit}
}

func (bufferInstance *limitedBuffer) Write(payload []byte) (int, error) {
	bufferInstance.accessMutex.Lock()
	defer bufferInstance.accessMutex.Unlock()
	original := len(payload)
	bufferInstance.data = append(bufferInstance.data, payload...)
	if overflow := len(bufferInstance.data) - bufferInstance.limit; overflow > 0 {
		copy(bufferInstance.data, bufferInstance.data[overflow:])
		bufferInstance.data = bufferInstance.data[:bufferInstance.limit]
		bufferInstance.truncated = true
	}
	return original, nil
}

func (bufferInstance *limitedBuffer) snapshot() (string, bool) {
	bufferInstance.accessMutex.Lock()
	defer bufferInstance.accessMutex.Unlock()
	return string(bytes.Clone(bufferInstance.data)), bufferInstance.truncated
}

type terminal struct {
	terminalIdentifier string
	commandProcess     *exec.Cmd
	output             *limitedBuffer
	done               chan struct{}
	waitOnce           sync.Once
	accessMutex        sync.RWMutex
	exitCode           int
	signal             string
}

func (terminalInstance *terminal) wait() {
	terminalInstance.waitOnce.Do(func() {
		operationError := terminalInstance.commandProcess.Wait()
		code := 0
		signal := ""
		if operationError != nil {
			var exitFailure *exec.ExitError
			if errors.As(operationError, &exitFailure) {
				code = exitFailure.ExitCode()
				if status, present := exitFailure.Sys().(syscall.WaitStatus); present && status.Signaled() {
					signal = status.Signal().String()
				}
			} else {
				code = -1
			}
		}
		terminalInstance.accessMutex.Lock()
		terminalInstance.exitCode = code
		terminalInstance.signal = signal
		terminalInstance.accessMutex.Unlock()
		close(terminalInstance.done)
	})
}

func (terminalInstance *terminal) result() map[string]any {
	text, truncated := terminalInstance.output.snapshot()
	result := map[string]any{"output": text, "truncated": truncated, "exitStatus": nil}
	select {
	case <-terminalInstance.done:
		terminalInstance.accessMutex.RLock()
		status := map[string]any{"exitCode": terminalInstance.exitCode, "signal": nil}
		if terminalInstance.signal != "" {
			status["signal"] = terminalInstance.signal
		}
		terminalInstance.accessMutex.RUnlock()
		result["exitStatus"] = status
	default:
	}
	return result
}

func (terminalInstance *terminal) kill() {
	if terminalInstance.commandProcess.Process == nil {
		return
	}
	select {
	case <-terminalInstance.done:
		return
	default:
	}
	if operationError := syscall.Kill(-terminalInstance.commandProcess.Process.Pid, syscall.SIGKILL); operationError != nil {
		_ = terminalInstance.commandProcess.Process.Kill()
	}
	// The wait goroutine reaps the child and closes done. Returning only after
	// that point gives terminalManager.close a real process-lifecycle barrier.
	<-terminalInstance.done
}

type terminalManager struct {
	accessMutex sync.RWMutex
	next        uint64
	entries     map[string]*terminal
	closed      atomic.Bool
}

func newTerminalManager() *terminalManager {
	return &terminalManager{entries: make(map[string]*terminal)}
}

func (managerInstance *terminalManager) create(params map[string]any) (map[string]any, error) {
	if managerInstance.closed.Load() {
		return nil, errors.New("terminal manager closed")
	}
	command := stringValue(params["command"])
	if command == "" {
		return nil, errors.New("command required")
	}
	arguments := stringSlice(params["args"])
	workingDirectory := stringValue(params["cwd"])
	limit := intValue(params["outputByteLimit"], 1_048_576)

	var commandProcess *exec.Cmd
	if len(arguments) > 0 {
		commandProcess = exec.Command(command, arguments...)
	} else {
		commandProcess = exec.Command("/bin/sh", "-lc", command)
	}
	if workingDirectory != "" {
		if info, operationError := os.Stat(workingDirectory); operationError == nil && info.IsDir() {
			commandProcess.Dir = workingDirectory
		}
	}
	commandProcess.Env = os.Environ()
	commandProcess.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if raw, present := params["env"].([]any); present {
		for _, item := range raw {
			entry, present := item.(map[string]any)
			if !present {
				continue
			}
			name := stringValue(entry["name"])
			if name != "" {
				commandProcess.Env = append(commandProcess.Env, name+"="+stringValue(entry["value"]))
			}
		}
	}
	buffer := newLimitedBuffer(limit)
	commandProcess.Stdout = buffer
	commandProcess.Stderr = buffer
	commandProcess.Stdin = nil

	// Starting and registering a terminal is one transaction with respect to
	// close. If close wins the mutex, no process is started; if create wins, close
	// observes the registered process and kills it before returning.
	managerInstance.accessMutex.Lock()
	if managerInstance.closed.Load() {
		managerInstance.accessMutex.Unlock()
		return nil, errors.New("terminal manager closed")
	}
	if operationError := commandProcess.Start(); operationError != nil {
		managerInstance.accessMutex.Unlock()
		return nil, operationError
	}
	managerInstance.next++
	terminalIdentifier := fmt.Sprintf("term-%d-%d", managerInstance.next, commandProcess.Process.Pid)
	term := &terminal{terminalIdentifier: terminalIdentifier, commandProcess: commandProcess, output: buffer, done: make(chan struct{})}
	managerInstance.entries[terminalIdentifier] = term
	go term.wait()
	managerInstance.accessMutex.Unlock()
	return map[string]any{"terminalId": terminalIdentifier}, nil
}

func (managerInstance *terminalManager) get(params map[string]any) (*terminal, error) {
	terminalIdentifier := stringValue(params["terminalId"])
	managerInstance.accessMutex.RLock()
	term := managerInstance.entries[terminalIdentifier]
	managerInstance.accessMutex.RUnlock()
	if term == nil {
		return nil, fmt.Errorf("unknown terminal %q", terminalIdentifier)
	}
	return term, nil
}

func (managerInstance *terminalManager) output(params map[string]any) (map[string]any, error) {
	term, operationError := managerInstance.get(params)
	if operationError != nil {
		return nil, operationError
	}
	return term.result(), nil
}

func (managerInstance *terminalManager) waitForExit(operationContext context.Context, params map[string]any) (map[string]any, error) {
	term, operationError := managerInstance.get(params)
	if operationError != nil {
		return nil, operationError
	}
	select {
	case <-operationContext.Done():
		return nil, operationContext.Err()
	case <-term.done:
		term.accessMutex.RLock()
		defer term.accessMutex.RUnlock()
		result := map[string]any{"exitCode": term.exitCode, "signal": nil}
		if term.signal != "" {
			result["signal"] = term.signal
		}
		return result, nil
	}
}

func (managerInstance *terminalManager) kill(params map[string]any) (map[string]any, error) {
	term, operationError := managerInstance.get(params)
	if operationError != nil {
		return nil, operationError
	}
	term.kill()
	return map[string]any{}, nil
}

func (managerInstance *terminalManager) release(params map[string]any) (map[string]any, error) {
	terminalIdentifier := stringValue(params["terminalId"])
	managerInstance.accessMutex.Lock()
	term := managerInstance.entries[terminalIdentifier]
	delete(managerInstance.entries, terminalIdentifier)
	managerInstance.accessMutex.Unlock()
	if term != nil {
		term.kill()
	}
	return map[string]any{}, nil
}

func (managerInstance *terminalManager) close() {
	managerInstance.closed.Store(true)
	managerInstance.accessMutex.Lock()
	entries := managerInstance.entries
	managerInstance.entries = make(map[string]*terminal)
	managerInstance.accessMutex.Unlock()
	for _, term := range entries {
		term.kill()
	}
}

func (managerInstance *terminalManager) count() int {
	managerInstance.accessMutex.RLock()
	defer managerInstance.accessMutex.RUnlock()
	return len(managerInstance.entries)
}

func stringValue(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case fmt.Stringer:
		return typed.String()
	case nil:
		return ""
	default:
		return fmt.Sprint(typed)
	}
}

func stringSlice(value any) []string {
	items, present := value.([]any)
	if !present {
		return nil
	}
	result := make([]string, 0, len(items))
	for _, item := range items {
		result = append(result, stringValue(item))
	}
	return result
}

func intValue(value any, fallback int) int {
	switch typed := value.(type) {
	case float64:
		return int(typed)
	case int:
		return typed
	case jsonNumber:
		if parsed, operationError := strconv.Atoi(string(typed)); operationError == nil {
			return parsed
		}
	case string:
		if parsed, operationError := strconv.Atoi(typed); operationError == nil {
			return parsed
		}
	}
	return fallback
}

type jsonNumber string

var _ io.Writer = (*limitedBuffer)(nil)
