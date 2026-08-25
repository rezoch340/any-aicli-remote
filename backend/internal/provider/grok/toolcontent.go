// Normalization of Grok's tool-call content into the ACP-standard shape. ACP
// defines ToolCall(Update).content as an array of ToolCallContent, each tagged
// by "type": "content" wraps a ContentBlock, "diff"/"terminal" carry their own.
// Grok emits the bare ContentBlock (e.g. {"type":"text","text":"..."}) directly
// as an array item, skipping the "content" wrapper. Clients speak standard ACP,
// so the wrapper is restored here at the adapter boundary and the deviation
// never reaches a client.

package grok

// contentBlockTypes are the ACP ContentBlock discriminators. An array item with
// one of these types is a bare block that must be wrapped as ToolCallContent.
var contentBlockTypes = map[string]bool{
	"text": true, "image": true, "audio": true, "resource": true, "resource_link": true,
}

// normalizeToolContent rewrites the tool content array of a session/update in
// place, wrapping Grok's bare ContentBlock items into ACP ToolCallContent. It is
// a no-op for non-tool updates and for content already in standard form.
func (providerInstance *GrokProvider) normalizeToolContent(method string, params map[string]any) {
	envelope, handled := parseChildAgentEnvelope(method, params)
	if !handled {
		return
	}
	switch stringValue(envelope.update["sessionUpdate"]) {
	case "tool_call", "tool_call_update":
	default:
		return
	}
	items, isArray := envelope.update["content"].([]any)
	if !isArray {
		return
	}
	for index, raw := range items {
		item, isObject := raw.(map[string]any)
		if !isObject {
			continue
		}
		if contentBlockTypes[stringValue(item["type"])] {
			items[index] = map[string]any{"type": "content", "content": item}
		}
	}
}
