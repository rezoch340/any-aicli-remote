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

func TestNormalizeWrapperExitPlanInteraction(testContext *testing.T) {
	providerInstance := newInteractionProvider(testContext)
	request, handled := providerInstance.NormalizeInteractionRequest(grokMethodExitPlanMode, map[string]any{
		"method": "x.ai/exit_plan_mode",
		"params": map[string]any{
			"sessionId": "session-1", "toolCallId": "call-12", "planContent": "# Plan\n进程内 LRU。",
		},
	})
	if !handled {
		testContext.Fatal("exit plan not normalized")
	}
	if request.Kind != providerapi.InteractionKindExitPlan || request.SessionID != "session-1" ||
		request.ToolCallID != "call-12" || request.PlanContent == "" {
		testContext.Fatalf("exit plan request = %#v", request)
	}
}

func TestNormalizeExitPlanAllowsMissingOrNullPlanContent(testContext *testing.T) {
	providerInstance := newInteractionProvider(testContext)
	for _, params := range []map[string]any{
		{"method": "x.ai/exit_plan_mode", "params": map[string]any{"sessionId": "session-1", "toolCallId": "call-12"}},
		{"method": "x.ai/exit_plan_mode", "params": map[string]any{"sessionId": "session-1", "toolCallId": "call-12", "planContent": nil}},
	} {
		request, handled := providerInstance.NormalizeInteractionRequest(grokMethodExitPlanMode, params)
		if !handled || request.PlanContent != "" {
			testContext.Fatalf("optional plan content = %#v handled=%v", request, handled)
		}
	}
}

func TestNormalizeWrapperAskInteractionWithOptions(testContext *testing.T) {
	providerInstance := newInteractionProvider(testContext)
	request, handled := providerInstance.NormalizeInteractionRequest(grokMethodAskUserQuestion, map[string]any{
		"method": "x.ai/ask_user_question",
		"params": map[string]any{
			"sessionId": "session-1", "toolCallId": "call-4", "mode": "plan",
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

func TestNormalizeInteractionAcceptsDirectOuterParamsForCompatibility(testContext *testing.T) {
	providerInstance := newInteractionProvider(testContext)
	request, handled := providerInstance.NormalizeInteractionRequest(grokMethodAskUserQuestion, map[string]any{
		"sessionId": "session-1", "toolCallId": "call-4", "questions": []any{map[string]any{"question": "兼容问题", "options": []any{}}},
	})
	if !handled || request.Questions[0].Question != "兼容问题" || len(request.Questions[0].Options) != 0 {
		testContext.Fatalf("direct outer params = %#v handled=%v", request, handled)
	}
}

func TestNormalizeInteractionPreservesMultiSelect(testContext *testing.T) {
	providerInstance := newInteractionProvider(testContext)
	request, handled := providerInstance.NormalizeInteractionRequest(grokMethodAskUserQuestion, map[string]any{
		"method": "x.ai/ask_user_question",
		"params": map[string]any{
			"sessionId": "session-1", "toolCallId": "call-4", "mode": "default",
			"questions": []any{map[string]any{"question": "可多选", "options": []any{}, "multiSelect": true}},
		},
	})
	if !handled || !request.Questions[0].MultiSelect {
		testContext.Fatalf("multiSelect = %#v handled=%v", request, handled)
	}
}

func TestNormalizeInteractionRejectsInvalidTypedWireFields(testContext *testing.T) {
	providerInstance := newInteractionProvider(testContext)
	validAsk := func() map[string]any {
		return map[string]any{
			"method": "x.ai/ask_user_question",
			"params": map[string]any{
				"sessionId": "session-1", "toolCallId": "call-4", "mode": "default",
				"questions": []any{map[string]any{
					"question": "问题", "options": []any{map[string]any{"label": "选项", "description": "说明"}},
				}},
			},
		}
	}
	cases := []struct {
		name   string
		method string
		params map[string]any
	}{
		{"question wrong type", grokMethodAskUserQuestion, func() map[string]any {
			params := validAsk()
			params["params"].(map[string]any)["questions"].([]any)[0].(map[string]any)["question"] = 1
			return params
		}()},
		{"options wrong type", grokMethodAskUserQuestion, func() map[string]any {
			params := validAsk()
			params["params"].(map[string]any)["questions"].([]any)[0].(map[string]any)["options"] = map[string]any{}
			return params
		}()},
		{"option label wrong type", grokMethodAskUserQuestion, func() map[string]any {
			params := validAsk()
			params["params"].(map[string]any)["questions"].([]any)[0].(map[string]any)["options"].([]any)[0].(map[string]any)["label"] = 1
			return params
		}()},
		{"option description wrong type", grokMethodAskUserQuestion, func() map[string]any {
			params := validAsk()
			params["params"].(map[string]any)["questions"].([]any)[0].(map[string]any)["options"].([]any)[0].(map[string]any)["description"] = 1
			return params
		}()},
		{"multiSelect string", grokMethodAskUserQuestion, func() map[string]any {
			params := validAsk()
			params["params"].(map[string]any)["questions"].([]any)[0].(map[string]any)["multiSelect"] = "true"
			return params
		}()},
		{"multiSelect number", grokMethodAskUserQuestion, func() map[string]any {
			params := validAsk()
			params["params"].(map[string]any)["questions"].([]any)[0].(map[string]any)["multiSelect"] = 1
			return params
		}()},
		{"mode wrong type", grokMethodAskUserQuestion, func() map[string]any {
			params := validAsk()
			params["params"].(map[string]any)["mode"] = 1
			return params
		}()},
		{"mode invalid value", grokMethodAskUserQuestion, func() map[string]any {
			params := validAsk()
			params["params"].(map[string]any)["mode"] = "other"
			return params
		}()},
		{"wrapper mode missing", grokMethodAskUserQuestion, func() map[string]any {
			params := validAsk()
			delete(params["params"].(map[string]any), "mode")
			return params
		}()},
		{"direct mode invalid", grokMethodAskUserQuestion, map[string]any{"sessionId": "s", "toolCallId": "c", "mode": "other", "questions": []any{map[string]any{"question": "问题", "options": []any{}}}}},
		{"session wrong type", grokMethodAskUserQuestion, func() map[string]any {
			params := validAsk()
			params["params"].(map[string]any)["sessionId"] = 1
			return params
		}()},
		{"tool call wrong type", grokMethodAskUserQuestion, func() map[string]any {
			params := validAsk()
			params["params"].(map[string]any)["toolCallId"] = 1
			return params
		}()},
		{"plan content wrong type", grokMethodExitPlanMode, map[string]any{"method": "x.ai/exit_plan_mode", "params": map[string]any{"sessionId": "s", "toolCallId": "c", "planContent": 1}}},
		{"duplicate questions", grokMethodAskUserQuestion, func() map[string]any {
			params := validAsk()
			questions := params["params"].(map[string]any)["questions"].([]any)
			params["params"].(map[string]any)["questions"] = append(questions, map[string]any{"question": "问题", "options": []any{}})
			return params
		}()},
	}
	for _, testCase := range cases {
		if _, handled := providerInstance.NormalizeInteractionRequest(testCase.method, testCase.params); handled {
			testContext.Fatalf("%s did not fail closed", testCase.name)
		}
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

func TestNormalizeInteractionRejectsMalformedWrapper(testContext *testing.T) {
	providerInstance := newInteractionProvider(testContext)
	cases := []struct {
		name   string
		method string
		params map[string]any
	}{
		{"missing inner method", grokMethodAskUserQuestion, map[string]any{"params": map[string]any{}}},
		{"missing inner params", grokMethodAskUserQuestion, map[string]any{"method": "x.ai/ask_user_question"}},
		{"inner method wrong type", grokMethodAskUserQuestion, map[string]any{"method": 1, "params": map[string]any{}}},
		{"inner params wrong type", grokMethodAskUserQuestion, map[string]any{"method": "x.ai/ask_user_question", "params": "bad"}},
		{"inner method mismatch", grokMethodAskUserQuestion, map[string]any{"method": "x.ai/exit_plan_mode", "params": map[string]any{}}},
	}
	for _, testCase := range cases {
		if _, handled := providerInstance.NormalizeInteractionRequest(testCase.method, testCase.params); handled {
			testContext.Fatalf("%s did not fail closed", testCase.name)
		}
	}
}

func TestDenormalizeExitPlanOutcomes(testContext *testing.T) {
	providerInstance := newInteractionProvider(testContext)
	approved, operationError := providerInstance.DenormalizeInteractionResponse(providerapi.InteractionRequest{Kind: providerapi.InteractionKindExitPlan},
		providerapi.InteractionResponse{Outcome: providerapi.InteractionOutcomeApproved})
	if operationError != nil || approved["outcome"] != "approved" || approved["feedback"] != nil {
		testContext.Fatalf("approved = %#v operationError=%v", approved, operationError)
	}
	cancelled, operationError := providerInstance.DenormalizeInteractionResponse(providerapi.InteractionRequest{Kind: providerapi.InteractionKindExitPlan},
		providerapi.InteractionResponse{Outcome: providerapi.InteractionOutcomeCancelled, Feedback: "加错误处理"})
	if operationError != nil || cancelled["outcome"] != "cancelled" || cancelled["feedback"] != "加错误处理" {
		testContext.Fatalf("cancelled = %#v operationError=%v", cancelled, operationError)
	}
	abandoned, operationError := providerInstance.DenormalizeInteractionResponse(providerapi.InteractionRequest{Kind: providerapi.InteractionKindExitPlan},
		providerapi.InteractionResponse{Outcome: providerapi.InteractionOutcomeAbandoned})
	if operationError != nil || abandoned["outcome"] != "abandoned" {
		testContext.Fatalf("abandoned = %#v operationError=%v", abandoned, operationError)
	}
	if _, operationError := providerInstance.DenormalizeInteractionResponse(providerapi.InteractionRequest{Kind: providerapi.InteractionKindExitPlan},
		providerapi.InteractionResponse{Outcome: providerapi.InteractionOutcomeAccepted}); operationError == nil {
		testContext.Fatal("ask outcome accepted on exit-plan must be rejected")
	}
}

func TestDenormalizeAskAnswersAndAnnotationsUseQuestionText(testContext *testing.T) {
	providerInstance := newInteractionProvider(testContext)
	result, operationError := providerInstance.DenormalizeInteractionResponse(providerapi.InteractionRequest{Kind: providerapi.InteractionKindAskQuestion, Questions: []providerapi.InteractionQuestion{{Question: "问题"}}},
		providerapi.InteractionResponse{
			Outcome:     providerapi.InteractionOutcomeAccepted,
			Answers:     map[string][]string{"0": {"进程内 LRU"}},
			Annotations: map[string]providerapi.InteractionAnnotation{"0": {Notes: "原因"}},
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
	if _, present := probe.Answers["问题"]; !present {
		testContext.Fatalf("answers not keyed by question text: %s", encoded)
	}
	annotations, valid := result["annotations"].(map[string]any)
	if !valid || annotations["问题"] == nil {
		testContext.Fatalf("annotations not keyed by question text: %#v", result)
	}
}

func TestDenormalizeAskCancelled(testContext *testing.T) {
	providerInstance := newInteractionProvider(testContext)
	result, operationError := providerInstance.DenormalizeInteractionResponse(
		providerapi.InteractionRequest{Kind: providerapi.InteractionKindAskQuestion},
		providerapi.InteractionResponse{Outcome: providerapi.InteractionOutcomeCancelled, Feedback: "不得透传"},
	)
	if operationError != nil || result["outcome"] != "cancelled" || len(result) != 1 {
		testContext.Fatalf("cancelled ask = %#v operationError=%v", result, operationError)
	}
}

func TestDenormalizeAskRejectsBadShapes(testContext *testing.T) {
	providerInstance := newInteractionProvider(testContext)
	request := providerapi.InteractionRequest{Kind: providerapi.InteractionKindAskQuestion, Questions: []providerapi.InteractionQuestion{{Question: "问题"}}}
	cases := []providerapi.InteractionResponse{
		{Outcome: providerapi.InteractionOutcomeAccepted},                                           // no answers
		{Outcome: providerapi.InteractionOutcomeAccepted, Answers: map[string][]string{"x": {"a"}}}, // non-index key
		{Outcome: providerapi.InteractionOutcomeAccepted, Answers: map[string][]string{"00": {"a"}}},
		{Outcome: providerapi.InteractionOutcomeAccepted, Answers: map[string][]string{"1": {"a"}}},
		{Outcome: providerapi.InteractionOutcomeAccepted, Answers: map[string][]string{"0": {"a"}}, Annotations: map[string]providerapi.InteractionAnnotation{"1": {}}},
		{Outcome: providerapi.InteractionOutcomeApproved}, // exit outcome on ask
	}
	for index, response := range cases {
		if _, operationError := providerInstance.DenormalizeInteractionResponse(request, response); operationError == nil {
			testContext.Fatalf("case %d accepted a bad ask answer", index)
		}
	}
}

func TestDenormalizeAskPartialOutcomes(testContext *testing.T) {
	providerInstance := newInteractionProvider(testContext)
	for _, outcome := range []providerapi.InteractionOutcome{providerapi.InteractionOutcomeChatAbout, providerapi.InteractionOutcomeSkip} {
		result, operationError := providerInstance.DenormalizeInteractionResponse(providerapi.InteractionRequest{Kind: providerapi.InteractionKindAskQuestion, Questions: []providerapi.InteractionQuestion{{Question: "问题"}}},
			providerapi.InteractionResponse{Outcome: outcome, PartialAnswers: map[string]string{"0": "看情况"}})
		if operationError != nil || result["outcome"] != string(outcome) {
			testContext.Fatalf("%s = %#v operationError=%v", outcome, result, operationError)
		}
		if _, present := result["partial_answers"]; !present {
			testContext.Fatalf("%s missing partial_answers", outcome)
		}
		partial := result["partial_answers"].(map[string]any)
		if partial["问题"] != "看情况" {
			testContext.Fatalf("%s partial answers = %#v", outcome, partial)
		}
	}
}
