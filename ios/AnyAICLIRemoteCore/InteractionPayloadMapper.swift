import Foundation

public enum InteractionPayloadMapper {
  public static func request(from envelope: [String: Any]) -> PendingInteraction? {
    guard envelope.string("method") == ACPWire.Method.interactionRequest,
      let rpcID = integerID(envelope["id"]),
      let parameters = envelope.object("params"),
      let kind = InteractionKind(rawValue: parameters.string("kind") ?? ""),
      let providerID = nonEmptyString(parameters.string("providerId")),
      let sessionID = nonEmptyString(parameters.string("sessionId")),
      let toolCallID = nonEmptyString(parameters.string("toolCallId"))
    else { return nil }
    let mappedQuestions = questions(from: parameters)
    guard kind != .askQuestion || mappedQuestions?.isEmpty == false,
      let mode = optionalString(parameters, key: "mode"),
      let planContent = optionalString(parameters, key: "planContent")
    else { return nil }
    return PendingInteraction(
      rpcID: rpcID,
      kind: kind,
      sessionIdentity: SessionIdentity(providerID: providerID, sessionID: sessionID),
      toolCallID: toolCallID,
      questions: mappedQuestions ?? [],
      planContent: planContent,
      mode: mode
    )
  }

  private static func questions(from parameters: [String: Any]) -> [InteractionQuestion]? {
    guard let rawQuestions = parameters["questions"] else {
      return nil
    }
    guard let questionObjects = rawQuestions as? [[String: Any]] else { return nil }
    var questions: [InteractionQuestion] = []
    for questionObject in questionObjects {
      guard let question = questionObject.string("question"), nonEmptyString(question) != nil,
        let options = options(from: questionObject),
        let multiSelect = questionObject["multiSelect"] as? Bool
      else { return nil }
      questions.append(InteractionQuestion(
        question: question,
        options: options,
        multiSelect: multiSelect
      ))
    }
    return questions
  }

  private static func options(from question: [String: Any]) -> [InteractionOption]? {
    guard let rawOptions = question["options"] else { return nil }
    guard let optionObjects = rawOptions as? [[String: Any]] else { return nil }
    var options: [InteractionOption] = []
    for optionObject in optionObjects {
      guard let label = optionObject.string("label"), nonEmptyString(label) != nil else { return nil }
      guard let description = description(from: optionObject) else { return nil }
      options.append(InteractionOption(
        label: label,
        description: description
      ))
    }
    return options
  }

  private static func optionalString(_ object: [String: Any], key: String) -> String? {
    guard let value = object[key] else { return "" }
    return value as? String
  }

  private static func description(from option: [String: Any]) -> String? {
    guard let value = option["description"] else { return "" }
    return value as? String
  }

  private static func integerID(_ value: Any?) -> Int? {
    guard let number = value as? NSNumber, CFGetTypeID(number) != CFBooleanGetTypeID() else { return nil }
    let integerValue = number.int64Value
    let doubleValue = number.doubleValue
    guard doubleValue.isFinite, doubleValue == Double(integerValue), integerValue >= 0,
      let result = Int(exactly: integerValue)
    else { return nil }
    return result
  }

  private static func nonEmptyString(_ value: String?) -> String? {
    guard let value else { return nil }
    let trimmed = value.trimmingCharacters(in: .whitespacesAndNewlines)
    return trimmed.isEmpty ? nil : trimmed
  }
}
