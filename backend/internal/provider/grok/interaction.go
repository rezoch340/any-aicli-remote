// Structured interaction wire handling. Grok blocks a turn on two reverse
// requests: `_x.ai/ask_user_question` and `_x.ai/exit_plan_mode`. They are
// mapped to the provider-neutral typed interaction, and the operator's neutral
// answer is mapped back to the exact response shapes the Grok agent expects.
//
// Response shapes are taken from xai-org/grok-build:
//   - exit_plan_mode: {outcome: approved|cancelled|abandoned, feedback?}
//   - ask_user_question: internally-tagged by "outcome":
//       accepted        {answers: {index: [labels]}, annotations?}
//       chat_about_this {partial_answers: {index: text}}
//       skip_interview  {partial_answers: {index: text}}
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
	sessionID := strings.TrimSpace(stringValue(params["sessionId"]))
	toolCallID := strings.TrimSpace(stringValue(params["toolCallId"]))
	if sessionID == "" || toolCallID == "" {
		return providerapi.InteractionRequest{}, false
	}
	request := providerapi.InteractionRequest{SessionID: sessionID, ToolCallID: toolCallID}
	switch method {
	case grokMethodExitPlanMode:
		request.Kind = providerapi.InteractionKindExitPlan
		request.PlanContent = stringValue(params["planContent"])
		return request, true
	case grokMethodAskUserQuestion:
		questions, valid := normalizeQuestions(params["questions"])
		if !valid {
			return providerapi.InteractionRequest{}, false
		}
		request.Kind = providerapi.InteractionKindAskQuestion
		request.Questions = questions
		request.Mode = strings.TrimSpace(stringValue(params["mode"]))
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
	for _, entry := range list {
		questionMap, isMap := entry.(map[string]any)
		if !isMap {
			return nil, false
		}
		text := strings.TrimSpace(stringValue(questionMap["question"]))
		if text == "" {
			return nil, false
		}
		options := make([]providerapi.InteractionOption, 0)
		if rawOptions, present := questionMap["options"].([]any); present {
			for _, option := range rawOptions {
				optionMap, isOptionMap := option.(map[string]any)
				if !isOptionMap {
					continue
				}
				label := strings.TrimSpace(stringValue(optionMap["label"]))
				if label == "" {
					continue
				}
				options = append(options, providerapi.InteractionOption{
					Label:       label,
					Description: strings.TrimSpace(stringValue(optionMap["description"])),
				})
			}
		}
		questions = append(questions, providerapi.InteractionQuestion{
			Question:    text,
			Options:     options,
			MultiSelect: boolValue(questionMap["multiSelect"]),
		})
	}
	return questions, true
}

// DenormalizeInteractionResponse maps a neutral answer to the Grok result. An
// outcome that does not belong to the interaction kind is rejected so a
// malformed client answer never reaches the agent.
func (providerInstance *GrokProvider) DenormalizeInteractionResponse(kind providerapi.InteractionKind, response providerapi.InteractionResponse) (map[string]any, error) {
	switch kind {
	case providerapi.InteractionKindExitPlan:
		return denormalizeExitPlanResponse(response)
	case providerapi.InteractionKindAskQuestion:
		return denormalizeAskResponse(response)
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

func denormalizeAskResponse(response providerapi.InteractionResponse) (map[string]any, error) {
	switch response.Outcome {
	case providerapi.InteractionOutcomeAccepted:
		// answers must serialize as a JSON object keyed by question index; an
		// array is rejected by the agent ("expected a map").
		answers := map[string]any{}
		for index, labels := range response.Answers {
			if !isNonNegativeIndex(index) {
				return nil, errors.New("answer key is not a question index")
			}
			selected := make([]string, 0, len(labels))
			for _, label := range labels {
				if trimmed := strings.TrimSpace(label); trimmed != "" {
					selected = append(selected, trimmed)
				}
			}
			answers[index] = selected
		}
		if len(answers) == 0 {
			return nil, errors.New("accepted answer has no selections")
		}
		result := map[string]any{"outcome": string(providerapi.InteractionOutcomeAccepted), "answers": answers}
		if len(response.Annotations) > 0 {
			annotations := map[string]any{}
			for index, annotation := range response.Annotations {
				entry := map[string]any{}
				if preview := strings.TrimSpace(annotation.Preview); preview != "" {
					entry["preview"] = preview
				}
				if notes := strings.TrimSpace(annotation.Notes); notes != "" {
					entry["notes"] = notes
				}
				if len(entry) > 0 {
					annotations[index] = entry
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
			if !isNonNegativeIndex(index) {
				return nil, errors.New("partial answer key is not a question index")
			}
			partial[index] = text
		}
		return map[string]any{"outcome": string(response.Outcome), "partial_answers": partial}, nil
	default:
		return nil, errors.New("invalid ask outcome")
	}
}

func isNonNegativeIndex(value string) bool {
	number, parseError := strconv.Atoi(value)
	return parseError == nil && number >= 0
}
