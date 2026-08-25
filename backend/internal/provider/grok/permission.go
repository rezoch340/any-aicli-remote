// Neutral display title for permission requests. Grok leaves the ACP-standard
// toolCall.title empty and carries the tool name/kind/input in the private
// _meta["x.ai/tool"] object. ACP forbids clients from interpreting _meta, so the
// command is summarized here into a human-readable title that the hub writes back
// into toolCall.title; clients then read only the standard field.

package grok

import (
	"sort"
	"strings"
)

// salientInputKeys are the tool argument fields most worth showing, in priority
// order — the command or path a permission prompt is really about.
var salientInputKeys = []string{"command", "cmd", "script", "path", "file_path", "filePath", "pattern", "query", "url"}

func permissionDisplayTitle(params map[string]any) string {
	toolCall, isObject := params["toolCall"].(map[string]any)
	if !isObject {
		return ""
	}
	// Prefer any title the agent already set; otherwise summarize from _meta.
	if title := strings.TrimSpace(stringValue(toolCall["title"])); title != "" {
		return title
	}
	meta, hasMeta := toolCall["_meta"].(map[string]any)
	if !hasMeta {
		return ""
	}
	tool, hasTool := meta["x.ai/tool"].(map[string]any)
	if !hasTool {
		return ""
	}
	name := strings.TrimSpace(stringValue(tool["name"]))
	detail := salientInput(tool["input"])
	switch {
	case name != "" && detail != "":
		return name + ": " + detail
	case detail != "":
		return detail
	default:
		return name
	}
}

func salientInput(value any) string {
	input, isObject := value.(map[string]any)
	if !isObject {
		return ""
	}
	for _, key := range salientInputKeys {
		if text := strings.TrimSpace(stringValue(input[key])); text != "" {
			return truncate(text, 200)
		}
	}
	// No known key: show the single field if there is exactly one, else nothing.
	if len(input) == 1 {
		for _, only := range input {
			return truncate(strings.TrimSpace(stringValue(only)), 200)
		}
	}
	// Deterministic fallback for multi-field inputs: the first key's value.
	keys := make([]string, 0, len(input))
	for key := range input {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if text := strings.TrimSpace(stringValue(input[key])); text != "" {
			return truncate(text, 200)
		}
	}
	return ""
}
