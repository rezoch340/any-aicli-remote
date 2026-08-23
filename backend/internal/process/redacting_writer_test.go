package process

import (
	"bytes"
	"strings"
	"testing"
)

func TestLiteralRedactingWriterHidesValuesAcrossWriteBoundaries(testContext *testing.T) {
	var output bytes.Buffer
	writer := newLiteralRedactingWriter(&output, []string{"transport-secret", "short"})
	chunks := []string{
		"before transport-",
		"secret middle sh",
		"ort after",
	}
	for _, chunk := range chunks {
		if _, operationError := writer.Write([]byte(chunk)); operationError != nil {
			testContext.Fatal(operationError)
		}
	}
	if operationError := writer.Close(); operationError != nil {
		testContext.Fatal(operationError)
	}
	result := output.String()
	if result != "before [REDACTED] middle [REDACTED] after" {
		testContext.Fatalf("redacted output = %q", result)
	}
	if strings.Contains(result, "transport-secret") || strings.Contains(result, "short") {
		testContext.Fatalf("sensitive value remained in output: %q", result)
	}
}

func TestLiteralRedactingWriterUsesLongestOverlappingValue(testContext *testing.T) {
	var output bytes.Buffer
	writer := newLiteralRedactingWriter(&output, []string{"secret", "secret-value"})
	if _, operationError := writer.Write([]byte("secret-value")); operationError != nil {
		testContext.Fatal(operationError)
	}
	if operationError := writer.Close(); operationError != nil {
		testContext.Fatal(operationError)
	}
	if output.String() != "[REDACTED]" {
		testContext.Fatalf("overlapping redaction = %q", output.String())
	}
}
