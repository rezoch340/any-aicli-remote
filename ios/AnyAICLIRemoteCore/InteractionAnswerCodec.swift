import Foundation

public enum InteractionAnswerCodec {
  public static func result(_ answer: InteractionAnswer) -> [String: Any] {
    switch answer {
    case .accept(let answers, let annotations):
      var payload: [String: Any] = ["outcome": "accepted", "answers": answers]
      let encodedAnnotations = annotations.reduce(into: [String: [String: String]]()) { result, entry in
        let notes = entry.value.notes.trimmingCharacters(in: .whitespacesAndNewlines)
        if !notes.isEmpty { result[entry.key] = ["notes": notes] }
      }
      if !encodedAnnotations.isEmpty { payload["annotations"] = encodedAnnotations }
      return payload
    case .chatAbout(let values):
      return ["outcome": "chat_about_this", "partialAnswers": values]
    case .skipInterview(let values):
      return ["outcome": "skip_interview", "partialAnswers": values]
    case .approve:
      return ["outcome": "approved"]
    case .cancel(let feedback):
      var payload: [String: Any] = ["outcome": "cancelled"]
      let trimmedFeedback = feedback.trimmingCharacters(in: .whitespacesAndNewlines)
      if !trimmedFeedback.isEmpty {
        payload["feedback"] = trimmedFeedback
      }
      return payload
    case .cancelAsk:
      return ["outcome": "cancelled"]
    case .abandon:
      return ["outcome": "abandoned"]
    }
  }
}
