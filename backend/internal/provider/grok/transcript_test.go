package grok

import "strings"

import "testing"

// liveScaffoldingSkillCatalogue reproduces the shape observed in a real
// chat_history.jsonl: an operator turn that is entirely a skill catalogue and
// carries absolute host paths.
const liveScaffoldingSkillCatalogue = "<system-reminder>\nThe following skills are available for use:\n\n" +
	"- using-git-worktrees: Use when starting feature work\n" +
	"  Absolute path: /Users/example/.claude/plugins/cache/skills/using-git-worktrees/SKILL.md\n" +
	"</system-reminder>"

func TestSystemPromptIsNotConversation(testContext *testing.T) {
	if _, conversational := conversationMessage("system", "You are Grok 4.6 released by xAI."); conversational {
		testContext.Fatal("the provider system prompt must not be shown as conversation")
	}
}

func TestSkillCatalogueTurnIsDroppedWithItsHostPaths(testContext *testing.T) {
	text, conversational := conversationMessage("user", liveScaffoldingSkillCatalogue)
	if conversational {
		testContext.Fatalf("scaffolding-only turn must be dropped, kept %q", text)
	}
	if strings.Contains(text, "/Users/") {
		testContext.Fatalf("host path leaked into conversation: %q", text)
	}
}

func TestWorkspaceInfoTurnIsDropped(testContext *testing.T) {
	record := "<user_info>\nOS Version: macos\nWorkspace Path: /private/tmp/workspace\n</user_info>"
	if text, conversational := conversationMessage("user", record); conversational {
		testContext.Fatalf("workspace info turn must be dropped, kept %q", text)
	}
}

func TestOperatorInputIsUnwrappedFromUserQuery(testContext *testing.T) {
	record := "<user_query>\nOutput only a markdown demo.\n</user_query>"
	text, conversational := conversationMessage("user", record)
	if !conversational {
		testContext.Fatal("the operator's own input must remain conversation")
	}
	if text != "Output only a markdown demo." {
		testContext.Fatalf("unexpected unwrapped text %q", text)
	}
}

func TestScaffoldingIsStrippedBeforeUnwrappingOperatorInput(testContext *testing.T) {
	record := "<system-reminder>MCP servers connected:\n- reqable (17 tools)\n</system-reminder>\n" +
		"<user_query>\nReal question.\n</user_query>"
	text, conversational := conversationMessage("user", record)
	if !conversational || text != "Real question." {
		testContext.Fatalf("expected the operator question alone, got %q (conversational=%v)", text, conversational)
	}
}

func TestUnterminatedScaffoldingDiscardsRemainder(testContext *testing.T) {
	record := "Visible question.\n<system-reminder>\ntruncated catalogue with /Users/example/secret"
	text, conversational := conversationMessage("user", record)
	if !conversational || text != "Visible question." {
		testContext.Fatalf("expected the leading question alone, got %q", text)
	}
	if strings.Contains(text, "/Users/") {
		testContext.Fatalf("host path leaked from an unterminated block: %q", text)
	}
}

func TestUnterminatedUserQueryKeepsRemainder(testContext *testing.T) {
	text, conversational := conversationMessage("user", "<user_query>\nTruncated question")
	if !conversational || text != "Truncated question" {
		testContext.Fatalf("expected the truncated question, got %q", text)
	}
}

func TestAssistantAndToolTurnsArePreservedVerbatim(testContext *testing.T) {
	answer := "## Markdown Demo\n\nThis line has **bold text**."
	for _, role := range []string{"assistant", "tool"} {
		text, conversational := conversationMessage(role, answer)
		if !conversational || text != answer {
			testContext.Fatalf("role %s must be preserved verbatim, got %q", role, text)
		}
	}
}

func TestBlankTurnsAreNotConversation(testContext *testing.T) {
	for _, role := range []string{"user", "assistant", "tool"} {
		if _, conversational := conversationMessage(role, "   \n\t"); conversational {
			testContext.Fatalf("role %s with blank content must be dropped", role)
		}
	}
}

func TestInjectedRulesAndRepositoryStateAreDropped(testContext *testing.T) {
	record := "<rules>\n<user_rules description=\"set by the user\">\n<user_rule>Always use Chinese.</user_rule>\n</user_rules>\n</rules>"
	if text, conversational := conversationMessage("user", record); conversational {
		testContext.Fatalf("injected rules must be dropped, kept %q", text)
	}
	if text, conversational := conversationMessage("user", "<git_status>\nbranch main, 3 files changed\n</git_status>"); conversational {
		testContext.Fatalf("injected repository state must be dropped, kept %q", text)
	}
}

// TestPlainOperatorTurnSurvives guards the shape seen in most stored sessions:
// the operator's turn carries no wrapper at all and must pass through.
func TestPlainOperatorTurnSurvives(testContext *testing.T) {
	text, conversational := conversationMessage("user", "继续拆吧")
	if !conversational || text != "继续拆吧" {
		testContext.Fatalf("an unwrapped operator turn must survive, got %q", text)
	}
}

func TestOperatorTextSurvivesAlongsideInjectedRules(testContext *testing.T) {
	record := "<rules>\n<user_rule>Be terse.</user_rule>\n</rules>\nWhat changed in this file?"
	text, conversational := conversationMessage("user", record)
	if !conversational || text != "What changed in this file?" {
		testContext.Fatalf("expected the operator question alone, got %q", text)
	}
}
