// Git output parsing: porcelain counts, splitting, and bounded decoding.

package gitapi

import (
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

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
