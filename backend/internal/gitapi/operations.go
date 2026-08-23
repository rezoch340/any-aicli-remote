// Git read operations over the bound session workspace.

package gitapi

import (
	"context"
	"io"
	"os"
	"strconv"
	"strings"
)

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
