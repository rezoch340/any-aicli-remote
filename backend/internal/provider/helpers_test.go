package provider

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCanonicalPathWithinRootsSelectsMostSpecificRoot(testContext *testing.T) {
	outerRoot := testContext.TempDir()
	innerRoot := filepath.Join(outerRoot, "nested")
	if operationError := os.MkdirAll(innerRoot, 0o755); operationError != nil {
		testContext.Fatal(operationError)
	}
	path := filepath.Join(innerRoot, "summary.json")
	if operationError := os.WriteFile(path, []byte("{}"), 0o644); operationError != nil {
		testContext.Fatal(operationError)
	}
	_, selectedRoot, operationError := CanonicalPathWithinRoots(path, []string{outerRoot, innerRoot})
	if operationError != nil {
		testContext.Fatal(operationError)
	}
	canonicalInnerRoot, _ := filepath.EvalSymlinks(innerRoot)
	if selectedRoot != canonicalInnerRoot {
		testContext.Fatalf("selected root = %q, want %q", selectedRoot, canonicalInnerRoot)
	}
}

func TestExtractTextAndTimestampMatchProviderWireFormats(testContext *testing.T) {
	content := []any{
		map[string]any{"type": "text", "text": "first"},
		map[string]any{"type": "toolCall", "name": "read_file"},
		map[string]any{"type": "tool_result", "content": []any{map[string]any{"type": "output_text", "output_text": "done"}}},
	}
	if actual := ExtractText(content); actual != "first\n[Tool: read_file]\ndone" {
		testContext.Fatalf("text = %q", actual)
	}
	if actual := ParseTimestampMilliseconds("1970-01-01T00:00:01Z"); actual != 1000 {
		testContext.Fatalf("timestamp = %d", actual)
	}
	if actual := ParseTimestampMilliseconds(1_700_000_000); actual != 1_700_000_000_000 {
		testContext.Fatalf("seconds timestamp = %d", actual)
	}
}

func TestMergeSessionMetadataKeepsCatalogDetailsAndActiveWorkspace(testContext *testing.T) {
	catalog := []SessionMetadata{{
		ProviderID: "grok", SessionID: "persisted", Title: "Persisted title",
		ProjectDirectory: "/stale", LastActiveAt: 100,
	}}
	active := []SessionMetadata{
		{ProviderID: "grok", SessionID: "persisted", ProjectDirectory: "/active", LastActiveAt: 200},
		{ProviderID: "grok", SessionID: "new-session", ProjectDirectory: "/new", LastActiveAt: 300},
	}
	merged := MergeSessionMetadata(catalog, active)
	if len(merged) != 2 || merged[0].SessionID != "new-session" || merged[1].SessionID != "persisted" {
		testContext.Fatalf("merged sessions = %#v", merged)
	}
	if merged[1].Title != "Persisted title" || merged[1].ProjectDirectory != "/active" || merged[1].LastActiveAt != 200 {
		testContext.Fatalf("enriched session = %#v", merged[1])
	}
}
