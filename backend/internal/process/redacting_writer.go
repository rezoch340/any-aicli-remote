package process

import (
	"bytes"
	"errors"
	"io"
	"sync"
)

var closedRedactingWriterError = errors.New("redacting writer is closed")

// literalRedactingWriter removes known sensitive literals from raw child
// process output. It retains enough trailing bytes to detect a literal split
// across adjacent Write calls before anything reaches the log file.
type literalRedactingWriter struct {
	accessMutex       sync.Mutex
	destination       io.Writer
	pending           []byte
	sensitiveValues   [][]byte
	maximumValueBytes int
	closed            bool
}

func newLiteralRedactingWriter(destination io.Writer, values []string) *literalRedactingWriter {
	writer := &literalRedactingWriter{destination: destination}
	seenValues := make(map[string]struct{}, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		if _, present := seenValues[value]; present {
			continue
		}
		seenValues[value] = struct{}{}
		encodedValue := []byte(value)
		writer.sensitiveValues = append(writer.sensitiveValues, encodedValue)
		if len(encodedValue) > writer.maximumValueBytes {
			writer.maximumValueBytes = len(encodedValue)
		}
	}
	return writer
}

func (writer *literalRedactingWriter) Write(data []byte) (int, error) {
	writer.accessMutex.Lock()
	defer writer.accessMutex.Unlock()
	if writer.closed {
		return 0, closedRedactingWriterError
	}
	writer.pending = append(writer.pending, data...)
	if operationError := writer.flushSafe(false); operationError != nil {
		return len(data), operationError
	}
	return len(data), nil
}

func (writer *literalRedactingWriter) Close() error {
	writer.accessMutex.Lock()
	defer writer.accessMutex.Unlock()
	if writer.closed {
		return nil
	}
	writer.closed = true
	return writer.flushSafe(true)
}

func (writer *literalRedactingWriter) flushSafe(final bool) error {
	flushBoundary := len(writer.pending)
	if !final && writer.maximumValueBytes > 1 {
		flushBoundary -= writer.maximumValueBytes - 1
	}
	if flushBoundary <= 0 {
		return nil
	}
	consumedBytes := 0
	for consumedBytes < flushBoundary {
		matchStart, matchEnd, found := writer.nextMatch(consumedBytes)
		if !found || matchStart >= flushBoundary {
			if operationError := writeComplete(writer.destination, writer.pending[consumedBytes:flushBoundary]); operationError != nil {
				return operationError
			}
			consumedBytes = flushBoundary
			break
		}
		if operationError := writeComplete(writer.destination, writer.pending[consumedBytes:matchStart]); operationError != nil {
			return operationError
		}
		if operationError := writeComplete(writer.destination, []byte("[REDACTED]")); operationError != nil {
			return operationError
		}
		consumedBytes = matchEnd
	}
	remainingBytes := len(writer.pending) - consumedBytes
	copy(writer.pending, writer.pending[consumedBytes:])
	writer.pending = writer.pending[:remainingBytes]
	return nil
}

func (writer *literalRedactingWriter) nextMatch(searchStart int) (int, int, bool) {
	earliestStart := len(writer.pending)
	earliestEnd := 0
	found := false
	for _, sensitiveValue := range writer.sensitiveValues {
		relativeStart := bytes.Index(writer.pending[searchStart:], sensitiveValue)
		if relativeStart < 0 {
			continue
		}
		absoluteStart := searchStart + relativeStart
		absoluteEnd := absoluteStart + len(sensitiveValue)
		if !found || absoluteStart < earliestStart || (absoluteStart == earliestStart && absoluteEnd > earliestEnd) {
			earliestStart = absoluteStart
			earliestEnd = absoluteEnd
			found = true
		}
	}
	return earliestStart, earliestEnd, found
}

func writeComplete(destination io.Writer, data []byte) error {
	if len(data) == 0 {
		return nil
	}
	writtenBytes, operationError := destination.Write(data)
	if operationError != nil {
		return operationError
	}
	if writtenBytes != len(data) {
		return io.ErrShortWrite
	}
	return nil
}
