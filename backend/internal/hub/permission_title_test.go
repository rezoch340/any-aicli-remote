package hub

import "testing"

func TestApplyPermissionTitleWritesToolCallTitle(testContext *testing.T) {
	params := map[string]any{"toolCall": map[string]any{}}
	applyPermissionTitle(params, "bash: ls")
	if params["toolCall"].(map[string]any)["title"] != "bash: ls" {
		testContext.Fatalf("title = %v", params["toolCall"])
	}
}

func TestApplyPermissionTitleNeverOverwritesAgentTitle(testContext *testing.T) {
	params := map[string]any{"toolCall": map[string]any{"title": "agent title"}}
	applyPermissionTitle(params, "derived")
	if params["toolCall"].(map[string]any)["title"] != "agent title" {
		testContext.Fatalf("agent title overwritten: %v", params["toolCall"])
	}
}

func TestApplyPermissionTitleIgnoresEmptyTitle(testContext *testing.T) {
	params := map[string]any{"toolCall": map[string]any{}}
	applyPermissionTitle(params, "   ")
	if _, present := params["toolCall"].(map[string]any)["title"]; present {
		testContext.Fatalf("empty title should not be written")
	}
}
