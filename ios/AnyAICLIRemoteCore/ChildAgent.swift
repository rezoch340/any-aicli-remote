import Foundation

public enum ChildAgentStatus: String, Equatable {
  case running
  case completed
  case failed
  case cancelled
  case unknown
}

public struct ChildAgentCard: Identifiable, Equatable {
  public let providerChildID: String
  public let childSessionID: String
  public let agentType: String
  public let description: String
  public let status: ChildAgentStatus
  public let startedAt: Int64
  public let completedAt: Int64
  public let toolCallCount: Int
  public let turnCount: Int
  public let modelID: String
  public let tokensUsed: Int64
  public let contextUsagePercent: Double
  public let sequence: Int64?

  public var id: String { providerChildID }

  public init(
    providerChildID: String,
    childSessionID: String = "",
    agentType: String = "",
    description: String = "",
    status: ChildAgentStatus = .running,
    startedAt: Int64 = 0,
    completedAt: Int64 = 0,
    toolCallCount: Int = 0,
    turnCount: Int = 0,
    modelID: String = "",
    tokensUsed: Int64 = 0,
    contextUsagePercent: Double = 0,
    sequence: Int64? = nil
  ) {
    self.providerChildID = providerChildID; self.childSessionID = childSessionID; self.agentType = agentType
    self.description = description; self.status = status; self.startedAt = startedAt; self.completedAt = completedAt
    self.toolCallCount = toolCallCount; self.turnCount = turnCount; self.modelID = modelID
    self.tokensUsed = tokensUsed; self.contextUsagePercent = contextUsagePercent; self.sequence = sequence
  }
}
