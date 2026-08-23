package historydata

import (
	"encoding/json"
	"math"
	"strconv"
	"strings"
)

var historyKeep = map[string]struct{}{
	"user_message_chunk":        {},
	"agent_message_chunk":       {},
	"agent_thought_chunk":       {},
	"tool_call":                 {},
	"tool_call_update":          {},
	"plan":                      {},
	"session_recap":             {},
	"turn_completed":            {},
	"task_completed":            {},
	"available_commands_update": {},
	"subagent_spawned":          {},
	"subagent_progress":         {},
	"subagent_finished":         {},
}

var terminalToolStatuses = map[string]struct{}{
	"completed": {},
	"failed":    {},
	"cancelled": {},
	"error":     {},
	"done":      {},
	"success":   {},
}

func parseUpdateLine(line string, live bool) Event {
	line = strings.TrimSpace(line)
	if line == "" {
		return nil
	}
	var obj map[string]any
	dec := json.NewDecoder(strings.NewReader(line))
	dec.UseNumber()
	if operationError := dec.Decode(&obj); operationError != nil {
		return nil
	}
	params, valid := asMap(obj["params"])
	if !valid {
		return nil
	}
	update, valid := asMap(params["update"])
	if !valid || update == nil {
		return nil
	}
	kind := toString(update["sessionUpdate"])
	if _, valid := historyKeep[kind]; !valid {
		return nil
	}
	content, _ := asMap(update["content"])
	text := ""
	if content != nil {
		text = toString(content["text"])
	}
	if (kind == "user_message_chunk" || kind == "agent_message_chunk" || kind == "agent_thought_chunk") && strings.TrimSpace(text) == "" {
		return nil
	}
	if kind == "tool_call_update" && !live {
		status := strings.ToLower(toString(update["status"]))
		_, terminal := terminalToolStatuses[status]
		_, hasContent := update["content"]
		_, hasRaw := update["rawOutput"]
		if status != "" && !terminal && !hasContent && !hasRaw {
			return nil
		}
	}
	meta := map[string]any{}
	if pmeta, valid := asMap(params["_meta"]); valid {
		for key, value := range pmeta {
			meta[key] = value
		}
	}
	if umeta, valid := asMap(update["_meta"]); valid {
		for key, value := range umeta {
			if _, exists := meta[key]; !exists {
				meta[key] = value
			}
		}
	}
	method := obj["method"]
	if toString(method) == "" {
		method = "session/update"
	}
	eid := ""
	if value, valid := meta["eventId"]; valid {
		eid = toString(value)
	}
	if eid == "" {
		if umeta, valid := asMap(update["_meta"]); valid {
			eid = toString(umeta["eventId"])
		}
	}
	return Event{
		"method": method,
		"params": map[string]any{
			"sessionId": params["sessionId"],
			"update":    update,
			"_meta":     meta,
		},
		"_kind": kind,
		"_eid":  eid,
	}
}

func stripAndTrim(events []Event, chatTextMaxRunes int) []Event {
	out := make([]Event, 0, len(events))
	for _, event := range events {
		clean := Event{}
		for key, value := range event {
			if strings.HasPrefix(key, "_") {
				continue
			}
			clean[key] = value
		}
		out = append(out, trimChatText(clean, chatTextMaxRunes))
	}
	return out
}

func trimChatText(event Event, maximum int) Event {
	params, valid := asMap(event["params"])
	if !valid {
		return event
	}
	update, valid := asMap(params["update"])
	if !valid {
		return event
	}
	content, valid := asMap(update["content"])
	if !valid || content == nil {
		return event
	}
	text, valid := content["text"].(string)
	if !valid || len([]rune(text)) <= maximum {
		return event
	}
	content2 := cloneMap(content)
	content2["text"] = string([]rune(text)[:maximum]) + "\n…[truncated for load speed]"
	update2 := cloneMap(update)
	update2["content"] = content2
	params2 := cloneMap(params)
	params2["update"] = update2
	clean := cloneMap(event)
	clean["params"] = params2
	return clean
}

func coalesceChat(events []Event) []Event {
	out := make([]Event, 0, len(events))
	for _, event := range events {
		kind := toString(event["_kind"])
		if (kind != "user_message_chunk" && kind != "agent_message_chunk") || len(out) == 0 {
			out = append(out, event)
			continue
		}
		prev := out[len(out)-1]
		if toString(prev["_kind"]) != kind {
			out = append(out, event)
			continue
		}
		pparams, _ := asMap(prev["params"])
		pupdate, _ := asMap(pparams["update"])
		cparams, _ := asMap(event["params"])
		cupdate, _ := asMap(cparams["update"])
		pcontent, valid := asMap(pupdate["content"])
		if !valid || pcontent == nil {
			pcontent = map[string]any{"type": "text", "text": ""}
			pupdate["content"] = pcontent
		}
		ccontent, _ := asMap(cupdate["content"])
		pcontent["text"] = toString(pcontent["text"]) + toString(ccontent["text"])
		pparams["update"] = pupdate
		prev["params"] = pparams
	}
	return out
}

func asMap(value any) (map[string]any, bool) {
	mapping, valid := value.(map[string]any)
	return mapping, valid
}

func cloneMap[ValueType any](mapping map[string]ValueType) map[string]any {
	out := make(map[string]any, len(mapping))
	for key, value := range mapping {
		out[key] = value
	}
	return out
}

func toString(value any) string {
	switch typedValue := value.(type) {
	case nil:
		return ""
	case string:
		return typedValue
	case json.Number:
		return typedValue.String()
	case float64:
		if math.Trunc(typedValue) == typedValue {
			return strconv.FormatInt(int64(typedValue), 10)
		}
		return strconv.FormatFloat(typedValue, 'f', -1, 64)
	case int64:
		return strconv.FormatInt(typedValue, 10)
	case int:
		return strconv.Itoa(typedValue)
	case bool:
		if typedValue {
			return "true"
		}
		return "false"
	default:
		return ""
	}
}

func isMessageKind(kind string) bool {
	return kind == "user_message_chunk" || kind == "agent_message_chunk"
}
