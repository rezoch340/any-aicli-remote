// Chat transcript sanitation for the Grok adapter.
//
// The Grok CLI keeps its own bookkeeping inside chat_history.jsonl: the system
// prompt, a <user_info> block naming the host and workspace, a <system-reminder>
// skill catalogue carrying absolute local paths, an MCP server inventory, and
// the operator's real input wrapped in <user_query>. None of that is
// conversation, and the paths inside it are host-private, so it is stripped at
// the adapter boundary and never reaches a provider-neutral message.

package grok

import "strings"

const (
	userQueryOpenTag  = "<user_query>"
	userQueryCloseTag = "</user_query>"
)

// scaffoldingTagNames are removed with their contents from operator turns. The
// list was taken from the tags actually present in stored sessions, not from
// the wire documentation: <rules> nests <user_rules>/<user_rule>, so removing
// the outer block covers them.
var scaffoldingTagNames = []string{"system-reminder", "user_info", "rules", "git_status"}

// conversationMessage maps one stored record to the text a client may display.
// The second result reports whether the record is conversation at all.
func conversationMessage(role, text string) (string, bool) {
	if role == "system" {
		return "", false
	}
	if role != "user" {
		trimmed := strings.TrimSpace(text)
		return text, trimmed != ""
	}
	cleaned := text
	for _, tagName := range scaffoldingTagNames {
		cleaned = removeTagBlocks(cleaned, tagName)
	}
	if inner, wrapped := unwrapTag(cleaned, userQueryOpenTag, userQueryCloseTag); wrapped {
		cleaned = inner
	}
	cleaned = strings.TrimSpace(cleaned)
	return cleaned, cleaned != ""
}

// removeTagBlocks deletes every <tagName>...</tagName> span. An unterminated
// opening tag discards the remainder: scaffolding never resumes being
// conversation partway through.
func removeTagBlocks(text, tagName string) string {
	openTag := "<" + tagName + ">"
	closeTag := "</" + tagName + ">"
	for {
		start := strings.Index(text, openTag)
		if start < 0 {
			return text
		}
		remainder := text[start+len(openTag):]
		end := strings.Index(remainder, closeTag)
		if end < 0 {
			return text[:start]
		}
		text = text[:start] + remainder[end+len(closeTag):]
	}
}

// unwrapTag returns the content between the first openTag and its closing tag.
// An unterminated opening tag yields everything that follows it.
func unwrapTag(text, openTag, closeTag string) (string, bool) {
	start := strings.Index(text, openTag)
	if start < 0 {
		return text, false
	}
	remainder := text[start+len(openTag):]
	end := strings.Index(remainder, closeTag)
	if end < 0 {
		return remainder, true
	}
	return remainder[:end], true
}
