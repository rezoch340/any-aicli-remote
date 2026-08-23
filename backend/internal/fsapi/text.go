// Text classification for workspace files: which paths are treated as text and
// how their bytes are decoded for transport.

package fsapi

import (
	"path/filepath"
	"strings"
	"unicode/utf8"
)

var textExtensions = map[string]struct{}{
	".py": {}, ".js": {}, ".ts": {}, ".tsx": {}, ".jsx": {}, ".json": {},
	".md": {}, ".txt": {}, ".css": {}, ".html": {}, ".htm": {}, ".xml": {},
	".yml": {}, ".yaml": {}, ".toml": {}, ".ini": {}, ".cfg": {}, ".env": {},
	".sh": {}, ".ps1": {}, ".bat": {}, ".cmd": {}, ".rs": {}, ".go": {},
	".java": {}, ".c": {}, ".h": {}, ".cpp": {}, ".hpp": {}, ".cs": {},
	".rb": {}, ".php": {}, ".sql": {}, ".r": {}, ".swift": {}, ".kt": {},
	".vue": {}, ".svelte": {}, ".scss": {}, ".less": {}, ".svg": {},
	".gitignore": {}, ".dockerfile": {}, ".cmake": {}, ".gradle": {}, ".log": {},
	".diff": {}, ".patch": {}, ".csv": {},
}

func IsTextPath(name string) bool {
	lower := strings.ToLower(filepath.Base(name))
	if _, valid := textExtensions[strings.ToLower(filepath.Ext(lower))]; valid {
		return true
	}
	switch lower {
	case "dockerfile", "makefile", "license", "readme":
		return true
	}
	return filepath.Ext(lower) == ""
}

func decodeText(data []byte) string {
	if len(data) >= 3 && data[0] == 0xef && data[1] == 0xbb && data[2] == 0xbf {
		data = data[3:]
	}
	if utf8.Valid(data) {
		return string(data)
	}
	runes := make([]rune, len(data))
	for byteIndex, byteValue := range data {
		runes[byteIndex] = rune(byteValue)
	}
	return string(runes)
}
