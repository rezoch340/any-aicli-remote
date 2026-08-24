import Foundation

public enum ChildAgentPayloadMapper {
  public static func card(fromEvent event: [String: Any]) -> ChildAgentCard? {
    guard let agent = event.object("agent") else { return nil }
    guard let rawSequence = event["sequence"] else {
      return card(from: agent, eventSequence: nil)
    }
    guard let sequence = sequenceValue(rawSequence) else { return nil }
    return card(from: agent, eventSequence: sequence)
  }
  public static func card(from record: [String: Any], eventSequence: Int64? = nil)
    -> ChildAgentCard? {
    guard let rawID = string(record, "providerChildId"),
      !rawID.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty else { return nil }
    let providerChildID = rawID.trimmingCharacters(in: .whitespacesAndNewlines)
    let status = ChildAgentStatus(rawValue: (string(record, "status") ?? "").lowercased()) ?? .unknown
    return ChildAgentCard(
      providerChildID: providerChildID,
      childSessionID: string(record, "childSessionId") ?? "",
      agentType: string(record, "agentType") ?? "",
      description: string(record, "description") ?? "",
      status: status,
      startedAt: integer(record["startedAt"]), completedAt: integer(record["completedAt"]),
      toolCallCount: Int(integer(record["toolCallCount"])), turnCount: Int(integer(record["turnCount"])),
      modelID: string(record, "modelId") ?? "",
      tokensUsed: integer(record["tokensUsed"]),
      contextUsagePercent: decimal(record["contextUsagePercent"]),
      sequence: eventSequence ?? optionalInteger(record["sequence"])
    )
  }

  private static func string(_ record: [String: Any], _ keys: String...) -> String? {
    keys.lazy.compactMap { record[$0] as? String }.first { !$0.isEmpty }
  }
  private static func integer(_ value: Any?) -> Int64 { (value as? NSNumber)?.int64Value ?? Int64((value as? String) ?? "") ?? 0 }
  private static func optionalInteger(_ value: Any?) -> Int64? { value == nil ? nil : integer(value) }
  private static func sequenceValue(_ value: Any) -> Int64? {
    if let number = value as? NSNumber { return number.int64Value }
    guard let string = value as? String else { return nil }
    return Int64(string.trimmingCharacters(in: .whitespacesAndNewlines))
  }
  private static func decimal(_ value: Any?) -> Double { (value as? NSNumber)?.doubleValue ?? Double((value as? String) ?? "") ?? 0 }
}
