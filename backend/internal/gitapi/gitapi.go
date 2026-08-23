package gitapi

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/rezoch340/any-aicli-remote/backend/internal/fsapi"
)

var (
	GitNotFoundError          = errors.New("git not found")
	WorkspaceUnavailableError = errors.New("workspace unavailable")
	PathOutsideWorkspaceError = errors.New("path outside workspace")
)

type Policy struct {
	CommandTimeout        time.Duration
	DiffTimeout           time.Duration
	DirtyFileLimit        int
	DiffRuneLimit         int
	LogDefaultLimit       int
	LogMaxLimit           int
	ContextFileReadBytes  int64
	ContextPreviewRunes   int
	CommandOutputMaxBytes int64
}

func (policy Policy) Validate() error {
	switch {
	case policy.CommandTimeout <= 0:
		return errors.New("git command timeout must be positive")
	case policy.DiffTimeout <= 0:
		return errors.New("git diff timeout must be positive")
	case policy.DirtyFileLimit <= 0:
		return errors.New("git dirty file limit must be positive")
	case policy.DiffRuneLimit <= 0:
		return errors.New("git diff rune limit must be positive")
	case policy.LogDefaultLimit <= 0:
		return errors.New("git log default limit must be positive")
	case policy.LogMaxLimit < policy.LogDefaultLimit:
		return errors.New("git log maximum limit must not be below default")
	case policy.ContextFileReadBytes <= 0:
		return errors.New("git context file read bytes must be positive")
	case policy.ContextFileReadBytes >= math.MaxInt64:
		return errors.New("git context file read bytes must leave room for sentinel")
	case policy.ContextPreviewRunes <= 0:
		return errors.New("git context preview runes must be positive")
	case policy.CommandOutputMaxBytes <= 0:
		return errors.New("git command output max bytes must be positive")
	case policy.CommandOutputMaxBytes >= math.MaxInt64:
		return errors.New("git command output max bytes must leave room for sentinel")
	}
	return nil
}

type Service struct {
	workspace    func() string
	rootIdentity *fsapi.RootIdentity
	gitPath      string
	policy       Policy
}

type DirtyFile struct {
	Code string `json:"code"`
	Path string `json:"path"`
}

type StatusResult struct {
	OK         bool        `json:"ok"`
	Error      string      `json:"error,omitempty"`
	Git        bool        `json:"git"`
	Root       string      `json:"root,omitempty"`
	Branch     string      `json:"branch,omitempty"`
	CommitHash string      `json:"sha,omitempty"`
	Ahead      int         `json:"ahead"`
	Behind     int         `json:"behind"`
	Dirty      int         `json:"dirty"`
	Files      []DirtyFile `json:"files"`
	Head       string      `json:"head,omitempty"`
}

type DiffResult struct {
	OK     bool   `json:"ok"`
	Path   string `json:"path"`
	Staged bool   `json:"staged"`
	Diff   string `json:"diff"`
	Code   int    `json:"code"`
}

type Commit struct {
	Hash    string `json:"hash"`
	Date    string `json:"date"`
	Subject string `json:"subject"`
}

type LogResult struct {
	OK      bool     `json:"ok"`
	Commits []Commit `json:"commits"`
}

type ContextFile struct {
	Name         string `json:"name"`
	RelativePath string `json:"rel"`
	Size         int64  `json:"size"`
	Preview      string `json:"preview"`
}

type ProjectContext struct {
	OK     bool          `json:"ok"`
	Root   string        `json:"root"`
	Branch *string       `json:"branch"`
	Files  []ContextFile `json:"files"`
}

func New(workspace func() string, policy Policy) (*Service, error) {
	if validationError := policy.Validate(); validationError != nil {
		return nil, validationError
	}
	gitPath, _ := exec.LookPath("git")
	return &Service{workspace: workspace, gitPath: gitPath, policy: policy}, nil
}

func NewWithGit(workspace func() string, gitPath string, policy Policy) (*Service, error) {
	if validationError := policy.Validate(); validationError != nil {
		return nil, validationError
	}
	return &Service{workspace: workspace, gitPath: gitPath, policy: policy}, nil
}

func NewPinned(rootIdentity *fsapi.RootIdentity, policy Policy) (*Service, error) {
	if validationError := policy.Validate(); validationError != nil {
		return nil, validationError
	}
	gitPath, _ := exec.LookPath("git")
	return &Service{rootIdentity: rootIdentity, gitPath: gitPath, policy: policy}, nil
}

func NewWithPinnedGit(rootIdentity *fsapi.RootIdentity, gitPath string, policy Policy) (*Service, error) {
	if validationError := policy.Validate(); validationError != nil {
		return nil, validationError
	}
	return &Service{rootIdentity: rootIdentity, gitPath: gitPath, policy: policy}, nil
}

func (service *Service) Status(operationContext context.Context) (StatusResult, error) {
	root, operationError := service.root()
	if operationError != nil {
		return StatusResult{}, operationError
	}
	result := StatusResult{OK: true, Root: root, Files: []DirtyFile{}}
	inside, operationError := service.run(operationContext, service.policy.CommandTimeout, root, "rev-parse", "--is-inside-work-tree")
	if operationError != nil {
		return StatusResult{}, operationError
	}
	if inside.code != 0 || strings.TrimSpace(inside.stdout) != "true" {
		return result, nil
	}
	result.Git = true

	if branch, runError := service.run(operationContext, service.policy.CommandTimeout, root, "rev-parse", "--abbrev-ref", "HEAD"); runError == nil && branch.code == 0 {
		result.Branch = strings.TrimSpace(branch.stdout)
	} else {
		result.Branch = "?"
	}
	if commitHash, runError := service.run(operationContext, service.policy.CommandTimeout, root, "rev-parse", "--short", "HEAD"); runError == nil && commitHash.code == 0 {
		result.CommitHash = strings.TrimSpace(commitHash.stdout)
	}
	status, runError := service.run(operationContext, service.policy.CommandTimeout, root, "status", "--porcelain", "-b")
	if runError != nil {
		return StatusResult{}, runError
	}
	lines := splitNonFinalEmpty(status.stdout)
	if len(lines) > 0 {
		result.Head = lines[0]
		lines = lines[1:]
	}
	result.Dirty = len(lines)
	for _, line := range lines {
		if len(result.Files) >= service.policy.DirtyFileLimit {
			break
		}
		if strings.TrimSpace(line) == "" {
			continue
		}
		code := line
		path := ""
		if len(line) >= 2 {
			code = line[:2]
		}
		if len(line) > 3 {
			path = strings.TrimSpace(line[3:])
		}
		result.Files = append(result.Files, DirtyFile{Code: code, Path: path})
	}
	result.Ahead = parseStatusCount(result.Head, `ahead\s+(\d+)`)
	result.Behind = parseStatusCount(result.Head, `behind\s+(\d+)`)
	return result, nil
}

func (service *Service) Diff(operationContext context.Context, path string, staged bool) (DiffResult, error) {
	root, operationError := service.root()
	if operationError != nil {
		return DiffResult{}, operationError
	}
	path, operationError = safePathspec(root, path)
	if operationError != nil {
		return DiffResult{}, operationError
	}
	arguments := []string{"diff", "--no-color"}
	if staged {
		arguments = append(arguments, "--cached")
	}
	if path != "" {
		arguments = append(arguments, "--", path)
	}
	output, operationError := service.run(operationContext, service.policy.DiffTimeout, root, arguments...)
	if operationError != nil {
		return DiffResult{}, operationError
	}
	return DiffResult{
		OK:     true,
		Path:   path,
		Staged: staged,
		Diff:   truncateRunes(output.stdout, service.policy.DiffRuneLimit),
		Code:   output.code,
	}, nil
}

func (service *Service) Log(operationContext context.Context, count int) (LogResult, error) {
	root, operationError := service.root()
	if operationError != nil {
		return LogResult{}, operationError
	}
	if count < 1 {
		count = service.policy.LogDefaultLimit
	}
	if count > service.policy.LogMaxLimit {
		count = service.policy.LogMaxLimit
	}
	output, operationError := service.run(operationContext, service.policy.CommandTimeout, root, "log", "-"+strconv.Itoa(count), "--pretty=format:%h%x09%ad%x09%s", "--date=short")
	if operationError != nil {
		return LogResult{}, operationError
	}
	result := LogResult{OK: true, Commits: []Commit{}}
	for _, line := range splitNonFinalEmpty(output.stdout) {
		parts := strings.SplitN(line, "\t", 3)
		if len(parts) == 3 {
			result.Commits = append(result.Commits, Commit{Hash: parts[0], Date: parts[1], Subject: parts[2]})
		}
	}
	return result, nil
}

func (service *Service) Project(operationContext context.Context) (ProjectContext, error) {
	root, operationError := service.root()
	if operationError != nil {
		return ProjectContext{}, operationError
	}
	result := ProjectContext{OK: true, Root: root, Files: []ContextFile{}}
	var rootFS *os.Root
	if service.rootIdentity != nil {
		rootFS, operationError = service.rootIdentity.OpenRoot()
	} else {
		rootFS, operationError = os.OpenRoot(root)
	}
	if operationError != nil {
		return ProjectContext{}, WorkspaceUnavailableError
	}
	defer rootFS.Close()
	for _, name := range []string{
		"AGENTS.md", "CLAUDE.md", "Claude.md", ".cursorrules", "README.md",
		"package.json", "pyproject.toml", "Cargo.toml", "go.mod",
	} {
		file, openError := rootFS.Open(name)
		if openError != nil {
			continue
		}
		info, statError := file.Stat()
		if statError != nil || !info.Mode().IsRegular() {
			_ = file.Close()
			continue
		}
		data, readError := io.ReadAll(io.LimitReader(file, service.policy.ContextFileReadBytes+1))
		_ = file.Close()
		if readError != nil || int64(len(data)) > service.policy.ContextFileReadBytes {
			continue
		}
		preview := decodeUTF8(data)
		result.Files = append(result.Files, ContextFile{
			Name:         name,
			RelativePath: name,
			Size:         info.Size(),
			Preview:      truncateRunes(preview, service.policy.ContextPreviewRunes),
		})
	}
	if branch, runError := service.run(operationContext, service.policy.CommandTimeout, root, "rev-parse", "--abbrev-ref", "HEAD"); runError == nil && branch.code == 0 {
		name := strings.TrimSpace(branch.stdout)
		result.Branch = &name
	}
	return result, nil
}

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

func (service *Service) root() (string, error) {
	if service.rootIdentity != nil {
		if operationError := service.rootIdentity.Validate(); operationError != nil {
			return "", WorkspaceUnavailableError
		}
		return service.rootIdentity.Path(), nil
	}
	if service.workspace == nil {
		return "", WorkspaceUnavailableError
	}
	raw := strings.TrimSpace(service.workspace())
	if raw == "" {
		return "", WorkspaceUnavailableError
	}
	root, operationError := filepath.Abs(filepath.Clean(raw))
	if operationError != nil {
		return "", fmt.Errorf("resolve workspace: %w", operationError)
	}
	info, operationError := os.Stat(root)
	if operationError != nil || !info.IsDir() {
		return "", WorkspaceUnavailableError
	}
	return root, nil
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

func parseStatusCount(head, pattern string) int {
	statusPattern := regexp.MustCompile(pattern)
	match := statusPattern.FindStringSubmatch(head)
	if len(match) != 2 {
		return 0
	}
	changeCount, _ := strconv.Atoi(match[1])
	return changeCount
}

func splitNonFinalEmpty(text string) []string {
	text = strings.TrimSuffix(text, "\n")
	if text == "" {
		return nil
	}
	return strings.Split(text, "\n")
}

func truncateRunes(text string, max int) string {
	if max < 0 {
		return ""
	}
	runes := []rune(text)
	if len(runes) <= max {
		return text
	}
	return string(runes[:max])
}

func decodeUTF8(data []byte) string {
	if utf8.Valid(data) {
		return string(data)
	}
	return strings.ToValidUTF8(string(data), "�")
}
