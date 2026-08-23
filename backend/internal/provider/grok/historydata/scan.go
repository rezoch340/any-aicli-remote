// Byte-window scanning over the update file, plus the bounds arithmetic the
// readers share.

package historydata

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"os"
)

func splitLinesKeepEnds(blob []byte) [][]byte {
	if len(blob) == 0 {
		return nil
	}
	var lines [][]byte
	start := 0
	for itemIndex, byteValue := range blob {
		if byteValue == '\n' {
			lines = append(lines, blob[start:itemIndex+1])
			start = itemIndex + 1
		}
	}
	if start < len(blob) {
		lines = append(lines, blob[start:])
	}
	return lines
}

func readRange(updatesFile *os.File, start, end, maxBytes int64) ([]byte, error) {
	if start < 0 || end < start || maxBytes <= 0 {
		return nil, errors.New("invalid byte range")
	}
	if end-start > maxBytes {
		return nil, errors.New("byte range exceeds maximum")
	}
	buffer := make([]byte, end-start)
	readLength, operationError := updatesFile.ReadAt(buffer, start)
	if operationError != nil && operationError != io.EOF {
		return nil, operationError
	}
	return buffer[:readLength], nil
}

func advanceAfterLine(updatesFile *os.File, start, end int64) int64 {
	if start <= 0 || start >= end {
		return start
	}
	remainingBytes := bufio.NewReader(io.NewSectionReader(updatesFile, start, end-start))
	line, operationError := remainingBytes.ReadBytes('\n')
	if operationError != nil && len(line) == 0 {
		return end
	}
	return start + int64(len(line))
}

func containsAnyBytes(haystack []byte, needles [][]byte) bool {
	for _, needle := range needles {
		if bytes.Contains(haystack, needle) {
			return true
		}
	}
	return false
}

func maxInt(leftValue, rightValue int) int {
	if leftValue > rightValue {
		return leftValue
	}
	return rightValue
}

func min64(leftValue, rightValue int64) int64 {
	if leftValue < rightValue {
		return leftValue
	}
	return rightValue
}

func max64(leftValue, rightValue int64) int64 {
	if leftValue > rightValue {
		return leftValue
	}
	return rightValue
}
