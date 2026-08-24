import AnyAICLIRemoteCore

enum ChildAgentReducer {
  static func apply(_ cards: [ChildAgentCard], incoming: ChildAgentCard) -> [ChildAgentCard] {
    guard let index = cards.firstIndex(where: { $0.providerChildID == incoming.providerChildID }) else {
      return cards + [incoming]
    }
    let existing = cards[index]
    if let old = existing.sequence, let new = incoming.sequence, new < old { return cards }
    var result = cards
    result[index] = merge(existing, incoming)
    return result
  }

  private static func merge(_ old: ChildAgentCard, _ new: ChildAgentCard) -> ChildAgentCard {
    ChildAgentCard(
      providerChildID: old.providerChildID,
      childSessionID: new.childSessionID.isEmpty ? old.childSessionID : new.childSessionID,
      agentType: new.agentType.isEmpty ? old.agentType : new.agentType,
      description: new.description.isEmpty ? old.description : new.description,
      status: new.status,
      startedAt: max(old.startedAt, new.startedAt), completedAt: max(old.completedAt, new.completedAt),
      toolCallCount: max(old.toolCallCount, new.toolCallCount), turnCount: max(old.turnCount, new.turnCount),
      modelID: new.modelID.isEmpty ? old.modelID : new.modelID,
      tokensUsed: max(old.tokensUsed, new.tokensUsed),
      contextUsagePercent: max(old.contextUsagePercent, new.contextUsagePercent),
      sequence: new.sequence ?? old.sequence)
  }
}
