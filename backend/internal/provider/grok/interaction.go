// Structured interaction wire handling. Grok blocks a turn on two reverse
// requests: `_x.ai/ask_user_question` and `_x.ai/exit_plan_mode`. They are
// mapped to the provider-neutral typed interaction, and the operator's neutral
// answer is mapped back to the exact response shapes the Grok agent expects.
//
// Response shapes are taken from xai-org/grok-build:
//   - exit_plan_mode: {outcome: approved|cancelled|abandoned, feedback?}
//   - ask_user_question: internally-tagged by "outcome":
//       accepted        {answers: {question text: [labels]}, annotations?}
//       chat_about_this {partial_answers: {question text: text}}
//       skip_interview  {partial_answers: {question text: text}}
//     answers must be a JSON object (map), never an array.

package grok

import (
	"errors"
	"strconv"
	"strings"

	providerapi "github.com/rezoch340/any-aicli-remote/backend/internal/provider"
)

const (
	grokMethodAskUserQuestion = "_x.ai/ask_user_question"
	grokMethodExitPlanMode    = "_x.ai/exit_plan_mode"
)

// NormalizeInteractionRequest converts the Grok reverse request into the neutral
// typed interaction. The second result is false for anything that is not a
// recognized, well-formed interaction, so the hub fails closed.
func (providerInstance *GrokProvider) NormalizeInteractionRequest(method string, params map[string]any) (providerapi.InteractionRequest, bool) {
	method, params, wrapped, valid := unwrapInteraction(method, params)
	if !valid {
		return providerapi.InteractionRequest{}, false
	}
	sessionID, valid := requiredString(params, "sessionId")
	if !valid {
		return providerapi.InteractionRequest{}, false
	}
	toolCallID, valid := requiredString(params, "toolCallId")
	if !valid {
		return providerapi.InteractionRequest{}, false
	}
	request := providerapi.InteractionRequest{SessionID: sessionID, ToolCallID: toolCallID}
	switch method {
	case grokMethodExitPlanMode:
		planContent, valid := optionalString(params, "planContent")
		if !valid {
			return providerapi.InteractionRequest{}, false
		}
		request.Kind = providerapi.InteractionKindExitPlan
		request.PlanContent = planContent
		return request, true
	case grokMethodAskUserQuestion:
		questions, valid := normalizeQuestions(params["questions"])
		if !valid {
			return providerapi.InteractionRequest{}, false
		}
		request.Kind = providerapi.InteractionKindAskQuestion
		request.Questions = questions
		mode, valid := interactionMode(params, wrapped)
		if !valid {
			return providerapi.InteractionRequest{}, false
		}
		request.Mode = mode
		return request, true
	default:
		return providerapi.InteractionRequest{}, false
	}
}

func normalizeQuestions(raw any) ([]providerapi.InteractionQuestion, bool) {
	list, isList := raw.([]any)
	if !isList || len(list) == 0 {
		return nil, false
	}
	questions := make([]providerapi.InteractionQuestion, 0, len(list))
	seenQuestions := make(map[string]struct{}, len(list))
	for _, entry := range list {
		questionMap, isMap := entry.(map[string]any)
		if !isMap {
			return nil, false
		}
		text, isString := questionMap["question"].(string)
		if !isString || strings.TrimSpace(text) == "" {
			return nil, false
		}
		if _, duplicate := seenQuestions[text]; duplicate {
			return nil, false
		}
		seenQuestions[text] = struct{}{}
		rawOptions, isList := questionMap["options"].([]any)
		if !isList {
			return nil, false
		}
		options := make([]providerapi.InteractionOption, 0, len(rawOptions))
		for _, option := range rawOptions {
			optionMap, isOptionMap := option.(map[string]any)
			if !isOptionMap {
				return nil, false
			}
			label, labelIsString := optionMap["label"].(string)
			description, descriptionIsString := optionMap["description"].(string)
			if !labelIsString || strings.TrimSpace(label) == "" || !descriptionIsString {
				return nil, false
			}
			options = append(options, providerapi.InteractionOption{Label: label, Description: description})
		}
		multiSelect, valid := optionalBoolean(questionMap, "multiSelect")
		if !valid {
			return nil, false
		}
		questions = append(questions, providerapi.InteractionQuestion{
			Question:    text,
			Options:     options,
			MultiSelect: multiSelect,
		})
	}
	return questions, true
}

// DenormalizeInteractionResponse maps a neutral answer to the Grok result. An
// outcome that does not belong to the interaction kind is rejected so a
// malformed client answer never reaches the agent.
func (providerInstance *GrokProvider) DenormalizeInteractionResponse(request providerapi.InteractionRequest, response providerapi.InteractionResponse) (map[string]any, error) {
	switch request.Kind {
	case providerapi.InteractionKindExitPlan:
		return denormalizeExitPlanResponse(response)
	case providerapi.InteractionKindAskQuestion:
		return denormalizeAskResponse(request, response)
	default:
		return nil, errors.New("unknown interaction kind")
	}
}

func denormalizeExitPlanResponse(response providerapi.InteractionResponse) (map[string]any, error) {
	switch response.Outcome {
	case providerapi.InteractionOutcomeApproved, providerapi.InteractionOutcomeAbandoned:
		return map[string]any{"outcome": string(response.Outcome)}, nil
	case providerapi.InteractionOutcomeCancelled:
		result := map[string]any{"outcome": string(response.Outcome)}
		if feedback := strings.TrimSpace(response.Feedback); feedback != "" {
			result["feedback"] = feedback
		}
		return result, nil
	default:
		return nil, errors.New("invalid exit-plan outcome")
	}
}

func denormalizeAskResponse(request providerapi.InteractionRequest, response providerapi.InteractionResponse) (map[string]any, error) {
	switch response.Outcome {
	case providerapi.InteractionOutcomeCancelled:
		return map[string]any{"outcome": string(providerapi.InteractionOutcomeCancelled)}, nil
	case providerapi.InteractionOutcomeAccepted:
		// Grok requires a JSON object keyed by the original question text; an
		// array is rejected by the agent ("expected a map").
		answers := map[string]any{}
		for index, labels := range response.Answers {
			question, valid := questionForIndex(request, index)
			if !valid {
				return nil, errors.New("answer key is not a question index")
			}
			selected := make([]string, 0, len(labels))
			for _, label := range labels {
				if trimmed := strings.TrimSpace(label); trimmed != "" {
					selected = append(selected, trimmed)
				}
			}
			answers[question] = selected
		}
		if len(answers) == 0 {
			return nil, errors.New("accepted answer has no selections")
		}
		result := map[string]any{"outcome": string(providerapi.InteractionOutcomeAccepted), "answers": answers}
		if len(response.Annotations) > 0 {
			annotations := map[string]any{}
			for index, annotation := range response.Annotations {
				question, valid := questionForIndex(request, index)
				if !valid {
					return nil, errors.New("annotation key is not a question index")
				}
				entry := map[string]any{}
				if preview := strings.TrimSpace(annotation.Preview); preview != "" {
					entry["preview"] = preview
				}
				if notes := strings.TrimSpace(annotation.Notes); notes != "" {
					entry["notes"] = notes
				}
				if len(entry) > 0 {
					annotations[question] = entry
				}
			}
			if len(annotations) > 0 {
				result["annotations"] = annotations
			}
		}
		return result, nil
	case providerapi.InteractionOutcomeChatAbout, providerapi.InteractionOutcomeSkip:
		partial := map[string]any{}
		for index, text := range response.PartialAnswers {
			question, valid := questionForIndex(request, index)
			if !valid {
				return nil, errors.New("partial answer key is not a question index")
			}
			partial[question] = text
		}
		return map[string]any{"outcome": string(response.Outcome), "partial_answers": partial}, nil
	default:
		return nil, errors.New("invalid ask outcome")
	}
}

func unwrapInteraction(method string, params map[string]any) (string, map[string]any, bool, bool) {
	innerMethod, isWrapper := map[string]string{
		grokMethodAskUserQuestion: "x.ai/ask_user_question",
		grokMethodExitPlanMode:    "x.ai/exit_plan_mode",
	}[method]
	if !isWrapper {
		return "", nil, false, false
	}
	_, hasMethod := params["method"]
	_, hasParams := params["params"]
	if !hasMethod && !hasParams {
		// Compatibility with older Grok frames whose outer params directly held
		// the interaction payload.
		return method, params, false, true
	}
	if !hasMethod || !hasParams {
		return "", nil, false, false
	}
	nestedMethod, methodTypeValid := params["method"].(string)
	nested, paramsTypeValid := params["params"].(map[string]any)
	if !methodTypeValid || !paramsTypeValid || nestedMethod != innerMethod {
		return "", nil, false, false
	}
	return method, nested, true, true
}

func requiredString(params map[string]any, key string) (string, bool) {
	value, isString := params[key].(string)
	return value, isString && strings.TrimSpace(value) != ""
}

func optionalString(params map[string]any, key string) (string, bool) {
	value, present := params[key]
	if !present || value == nil {
		return "", true
	}
	text, isString := value.(string)
	return text, isString
}

func optionalBoolean(params map[string]any, key string) (bool, bool) {
	value, present := params[key]
	if !present || value == nil {
		return false, true
	}
	boolean, isBoolean := value.(bool)
	return boolean, isBoolean
}

func interactionMode(params map[string]any, required bool) (string, bool) {
	value, present := params["mode"]
	if !present && !required {
		return "", true
	}
	mode, isString := value.(string)
	if !isString || (mode != "default" && mode != "plan") {
		return "", false
	}
	return mode, true
}

func questionForIndex(request providerapi.InteractionRequest, key string) (string, bool) {
	index, valid := parseQuestionIndex(key)
	if !valid || index >= uint64(len(request.Questions)) {
		return "", false
	}
	return request.Questions[int(index)].Question, true
}

// parseQuestionIndex accepts only canonical decimal indices used by the neutral
// protocol: zero, or a non-zero digit followed by decimal digits.
func parseQuestionIndex(value string) (uint64, bool) {
	if value == "" || (value != "0" && (value[0] < '1' || value[0] > '9')) {
		return 0, false
	}
	for _, character := range value {
		if character < '0' || character > '9' {
			return 0, false
		}
	}
	index, parseError := strconv.ParseUint(value, 10, 64)
	return index, parseError == nil
}
