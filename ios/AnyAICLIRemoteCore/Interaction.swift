import Foundation

public enum InteractionKind: String, Equatable {
  case askQuestion = "ask_question"
  case exitPlan = "exit_plan"
}

public struct InteractionOption: Equatable {
  public let label: String
  public let description: String

  public init(label: String, description: String = "") {
    self.label = label
    self.description = description
  }
}

public struct InteractionQuestion: Equatable {
  public let question: String
  public let options: [InteractionOption]
  public let multiSelect: Bool

  public init(question: String, options: [InteractionOption] = [], multiSelect: Bool = false) {
    self.question = question
    self.options = options
    self.multiSelect = multiSelect
  }
}

public struct InteractionAnnotation: Equatable {
  public let notes: String

  public init(notes: String) {
    self.notes = notes
  }
}

public struct PendingInteraction: Identifiable, Equatable {
  public let rpcID: Int
  public let kind: InteractionKind
  public let sessionIdentity: SessionIdentity
  public let toolCallID: String
  public let questions: [InteractionQuestion]
  public let planContent: String
  public let mode: String
  public var id: Int { rpcID }

  public init(
    rpcID: Int,
    kind: InteractionKind,
    sessionIdentity: SessionIdentity,
    toolCallID: String,
    questions: [InteractionQuestion] = [],
    planContent: String = "",
    mode: String = ""
  ) {
    self.rpcID = rpcID
    self.kind = kind
    self.sessionIdentity = sessionIdentity
    self.toolCallID = toolCallID
    self.questions = questions
    self.planContent = planContent
    self.mode = mode
  }
}

public enum InteractionAnswer: Equatable {
  case accept(answers: [String: [String]], annotations: [String: InteractionAnnotation])
  case chatAbout(partialAnswers: [String: String])
  case skipInterview(partialAnswers: [String: String])
  case approve
  case cancel(feedback: String)
  case cancelAsk
  case abandon
}
