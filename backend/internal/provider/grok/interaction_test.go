package grok

import (
	"encoding/json"
	"path/filepath"
	"testing"

	providerapi "github.com/rezoch340/any-aicli-remote/backend/internal/provider"
)

func newInteractionProvider(testContext *testing.T) *GrokProvider {
	return mustNew(testContext, Config{SessionsDirectory: filepath.Join(testContext.TempDir(), "sessions")})
}

func TestClassifyReverseRequestMarksInteractions(testContext *testing.T) {
	providerInstance := newInteractionProvider(testContext)
	for _, method := range []string{grokMethodAskUserQuestion, grokMethodExitPlanMode} {
		request, known := providerInstance.ClassifyReverseRequest(method, map[string]any{"sessionId": "s"})
		if !known || request.Operation != providerapi.InteractionOperation {
			testContext.Fatalf("%s classified as %#v known=%v", method, request, known)
		}
	}
}

func TestClassifyReverseRequestNoLongerMatchesAskUserSubstring(testContext *testing.T) {
	providerInstance := newInteractionProvider(testContext)
	// An unrelated method that merely contains "ask_user" must not be swept into
	// the permission path any more.
	if _, known := providerInstance.ClassifyReverseRequest("x.ai/ask_user_metrics", map[string]any{"sessionId": "s"}); known {
		testContext.Fatal("dead ask_user substring match still classifies unrelated method")
	}
}

func TestNormalizeExitPlanInteraction(testContext *testing.T) {
	providerInstance := newInteractionProvider(testContext)
	request, handled := providerInstance.NormalizeInteractionRequest(grokMethodExitPlanMode, map[string]any{
		"sessionId":   "session-1",
		"toolCallId":  "call-12",
		"planContent": "# Plan\n进程内 LRU。",
	})
	if !handled {
		testContext.Fatal("exit plan not normalized")
	}
	if request.Kind != providerapi.InteractionKindExitPlan || request.SessionID != "session-1" ||
		request.ToolCallID != "call-12" || request.PlanContent == "" {
		testContext.Fatalf("exit plan request = %#v", request)
	}
}

func TestNormalizeAskInteractionWithOptions(testContext *testing.T) {
	providerInstance := newInteractionProvider(testContext)
	request, handled := providerInstance.NormalizeInteractionRequest(grokMethodAskUserQuestion, map[string]any{
		"sessionId":  "session-1",
		"toolCallId": "call-4",
		"mode":       "plan",
		"questions": []any{
			map[string]any{
				"question": "缓存用 Redis 还是进程内 LRU？",
				"options": []any{
					map[string]any{"label": "Redis", "description": "跨进程共享"},
					map[string]any{"label": "进程内 LRU", "description": "无外部依赖"},
				},
				"multiSelect": nil,
			},
		},
	})
	if !handled {
		testContext.Fatal("ask not normalized")
	}
	if request.Kind != providerapi.InteractionKindAskQuestion || request.Mode != "plan" || len(request.Questions) != 1 {
		testContext.Fatalf("ask request = %#v", request)
	}
	question := request.Questions[0]
	if len(question.Options) != 2 || question.Options[0].Label != "Redis" || question.Options[1].Label != "进程内 LRU" || question.MultiSelect {
		testContext.Fatalf("question = %#v", question)
	}
}

func TestNormalizeInteractionFailsClosed(testContext *testing.T) {
	providerInstance := newInteractionProvider(testContext)
	cases := []struct {
		name   string
		method string
		params map[string]any
	}{
		{"missing sessionId", grokMethodExitPlanMode, map[string]any{"toolCallId": "c"}},
		{"missing toolCallId", grokMethodExitPlanMode, map[string]any{"sessionId": "s"}},
		{"ask without questions", grokMethodAskUserQuestion, map[string]any{"sessionId": "s", "toolCallId": "c"}},
		{"ask empty questions", grokMethodAskUserQuestion, map[string]any{"sessionId": "s", "toolCallId": "c", "questions": []any{}}},
		{"ask blank question text", grokMethodAskUserQuestion, map[string]any{"sessionId": "s", "toolCallId": "c", "questions": []any{map[string]any{"question": "  "}}}},
		{"unrelated method", "x.ai/other", map[string]any{"sessionId": "s", "toolCallId": "c"}},
	}
	for _, testCase := range cases {
		if _, handled := providerInstance.NormalizeInteractionRequest(testCase.method, testCase.params); handled {
			testContext.Fatalf("%s did not fail closed", testCase.name)
		}
	}
}

func TestDenormalizeExitPlanOutcomes(testContext *testing.T) {
	providerInstance := newInteractionProvider(testContext)
	approved, operationError := providerInstance.DenormalizeInteractionResponse(providerapi.InteractionKindExitPlan,
		providerapi.InteractionResponse{Outcome: providerapi.InteractionOutcomeApproved})
	if operationError != nil || approved["outcome"] != "approved" || approved["feedback"] != nil {
		testContext.Fatalf("approved = %#v operationError=%v", approved, operationError)
	}
	cancelled, operationError := providerInstance.DenormalizeInteractionResponse(providerapi.InteractionKindExitPlan,
		providerapi.InteractionResponse{Outcome: providerapi.InteractionOutcomeCancelled, Feedback: "加错误处理"})
	if operationError != nil || cancelled["outcome"] != "cancelled" || cancelled["feedback"] != "加错误处理" {
		testContext.Fatalf("cancelled = %#v operationError=%v", cancelled, operationError)
	}
	abandoned, operationError := providerInstance.DenormalizeInteractionResponse(providerapi.InteractionKindExitPlan,
		providerapi.InteractionResponse{Outcome: providerapi.InteractionOutcomeAbandoned})
	if operationError != nil || abandoned["outcome"] != "abandoned" {
		testContext.Fatalf("abandoned = %#v operationError=%v", abandoned, operationError)
	}
	if _, operationError := providerInstance.DenormalizeInteractionResponse(providerapi.InteractionKindExitPlan,
		providerapi.InteractionResponse{Outcome: providerapi.InteractionOutcomeAccepted}); operationError == nil {
		testContext.Fatal("ask outcome accepted on exit-plan must be rejected")
	}
}

func TestDenormalizeAskAnswersSerializeAsMap(testContext *testing.T) {
	providerInstance := newInteractionProvider(testContext)
	result, operationError := providerInstance.DenormalizeInteractionResponse(providerapi.InteractionKindAskQuestion,
		providerapi.InteractionResponse{
			Outcome: providerapi.InteractionOutcomeAccepted,
			Answers: map[string][]string{"0": {"进程内 LRU"}},
		})
	if operationError != nil {
		testContext.Fatal(operationError)
	}
	encoded, _ := json.Marshal(result)
	// The agent rejects an array; the answers field must be a JSON object.
	var probe struct {
		Outcome string                     `json:"outcome"`
		Answers map[string]json.RawMessage `json:"answers"`
	}
	if json.Unmarshal(encoded, &probe) != nil || probe.Outcome != "accepted" {
		testContext.Fatalf("unexpected result json %s", encoded)
	}
	if _, present := probe.Answers["0"]; !present {
		testContext.Fatalf("answers not keyed by question index: %s", encoded)
	}
}

func TestDenormalizeAskRejectsBadShapes(testContext *testing.T) {
	providerInstance := newInteractionProvider(testContext)
	cases := []providerapi.InteractionResponse{
		{Outcome: providerapi.InteractionOutcomeAccepted},                                           // no answers
		{Outcome: providerapi.InteractionOutcomeAccepted, Answers: map[string][]string{"x": {"a"}}}, // non-index key
		{Outcome: providerapi.InteractionOutcomeApproved},                                           // exit outcome on ask
	}
	for index, response := range cases {
		if _, operationError := providerInstance.DenormalizeInteractionResponse(providerapi.InteractionKindAskQuestion, response); operationError == nil {
			testContext.Fatalf("case %d accepted a bad ask answer", index)
		}
	}
}

func TestDenormalizeAskPartialOutcomes(testContext *testing.T) {
	providerInstance := newInteractionProvider(testContext)
	for _, outcome := range []providerapi.InteractionOutcome{providerapi.InteractionOutcomeChatAbout, providerapi.InteractionOutcomeSkip} {
		result, operationError := providerInstance.DenormalizeInteractionResponse(providerapi.InteractionKindAskQuestion,
			providerapi.InteractionResponse{Outcome: outcome, PartialAnswers: map[string]string{"0": "看情况"}})
		if operationError != nil || result["outcome"] != string(outcome) {
			testContext.Fatalf("%s = %#v operationError=%v", outcome, result, operationError)
		}
		if _, present := result["partial_answers"]; !present {
			testContext.Fatalf("%s missing partial_answers", outcome)
		}
	}
}
