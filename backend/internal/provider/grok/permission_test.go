package grok

import (
	"testing"

	providerapi "github.com/rezoch340/any-aicli-remote/backend/internal/provider"
)

func permissionParams(toolCall map[string]any) map[string]any {
	return map[string]any{"sessionId": "sess-A", "toolCall": toolCall, "options": []any{}}
}

func TestClassifyPermissionDerivesTitleFromToolMeta(testContext *testing.T) {
	provider := &GrokProvider{}
	params := permissionParams(map[string]any{
		"_meta": map[string]any{
			"x.ai/tool": map[string]any{
				"name":  "bash",
				"kind":  "execute",
				"input": map[string]any{"command": "ls -la"},
			},
		},
	})
	request, known := provider.ClassifyReverseRequest("session/request_permission", params)
	if !known || request.Operation != providerapi.PermissionOperation {
		testContext.Fatalf("permission not classified: %+v", request)
	}
	if request.DisplayTitle != "bash: ls -la" {
		testContext.Fatalf("DisplayTitle = %q, want %q", request.DisplayTitle, "bash: ls -la")
	}
}

func TestClassifyPermissionPrefersExistingTitle(testContext *testing.T) {
	provider := &GrokProvider{}
	params := permissionParams(map[string]any{
		"title": "Run ls",
		"_meta": map[string]any{"x.ai/tool": map[string]any{"name": "bash"}},
	})
	request, _ := provider.ClassifyReverseRequest("session/request_permission", params)
	if request.DisplayTitle != "Run ls" {
		testContext.Fatalf("DisplayTitle = %q, want existing title", request.DisplayTitle)
	}
}

func TestPermissionTitleReadsPathWhenNoCommand(testContext *testing.T) {
	provider := &GrokProvider{}
	params := permissionParams(map[string]any{
		"_meta": map[string]any{
			"x.ai/tool": map[string]any{"name": "read_file", "input": map[string]any{"path": "/tmp/stamp.txt"}},
		},
	})
	request, _ := provider.ClassifyReverseRequest("session/request_permission", params)
	if request.DisplayTitle != "read_file: /tmp/stamp.txt" {
		testContext.Fatalf("DisplayTitle = %q", request.DisplayTitle)
	}
}
