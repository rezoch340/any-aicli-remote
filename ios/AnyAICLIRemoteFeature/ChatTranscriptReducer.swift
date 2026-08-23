import AnyAICLIRemoteCore
import Foundation

enum ChatTranscriptReducer {
  static func shouldMarkTurnBusy(activeTurnID: UUID?) -> Bool { activeTurnID != nil }

  static func apply(
    update: [String: Any], to blocks: inout [ChatBlock],
    pendingUserEchoTracker: PendingUserEchoTracker
  ) -> TranscriptUpdate {
    let type = update.string("sessionUpdate") ?? ""
    let text = update.object("content")?.string("text") ?? update.string("text") ?? ""
    switch type {
    case "user_message_chunk":
      if !pendingUserEchoTracker.consume(chunk: text) {
        appendChunk(kind: .user, text: text, to: &blocks)
      }
      return .none
    case "agent_message_chunk":
      appendChunk(kind: .assistant, text: text, to: &blocks)
      return .busy("正在回复")
    case "agent_thought_chunk":
      appendChunk(kind: .thinking, text: text, to: &blocks)
      return .busy("正在思考")
    case "tool_call", "tool_call_update":
      upsertTool(update, in: &blocks)
      return .busy(update.string("title", "toolName", "kind") ?? "正在使用工具")
    case "plan":
      appendChunk(
        kind: .plan, text: text.isEmpty ? String(describing: update["entries"] ?? "Plan") : text,
        to: &blocks)
      return .none
    case "session_recap":
      appendChunk(kind: .system, text: text, to: &blocks)
      return .none
    case "turn_completed", "task_completed": return .finished(.success, "完成")
    case "cancelled", "turn_cancelled", "task_cancelled": return .finished(.cancelled, "已停止")
    case "turn_failed", "task_failed", "failed", "error": return .finished(.failed, "执行失败")
    default: return .none
    }
  }

  static func finalizeActiveTools(in blocks: [ChatBlock], as finalState: ToolRunState)
    -> [ChatBlock] {
    blocks.map { block in
      guard block.kind == .tool, [.pending, .running].contains(block.toolState) else {
        return block
      }
      var finalizedBlock = block
      finalizedBlock.toolState = finalState
      return finalizedBlock
    }
  }

  private static func appendChunk(kind: ChatBlockKind, text: String, to blocks: inout [ChatBlock]) {
    guard !text.isEmpty else { return }
    if let lastIndex = blocks.indices.last, blocks[lastIndex].kind == kind,
      [.user, .assistant, .thinking].contains(kind) {
      blocks[lastIndex].text += text
    } else {
      blocks.append(ChatBlock(id: UUID().uuidString, kind: kind, text: text))
    }
  }

  private static func upsertTool(_ update: [String: Any], in blocks: inout [ChatBlock]) {
    let callID = update.string("toolCallId", "tool_call_id", "id") ?? UUID().uuidString
    let title = update.string("title", "toolName", "kind")
    let status = update.string("status", "toolStatus")
    let detail =
      update.string("content", "result") ?? update.object("content")?.string("text") ?? ""
    if let index = blocks.firstIndex(where: { $0.id == "tool-\(callID)" }) {
      if let title { blocks[index].title = title }
      if status != nil { blocks[index].toolState = ToolRunState(raw: status) }
      if !detail.isEmpty { blocks[index].detail = detail }
    } else {
      blocks.append(
        ChatBlock(
          id: "tool-\(callID)", kind: .tool, title: title ?? "工具", detail: detail,
          toolState: ToolRunState(raw: status)))
    }
  }
}

enum TranscriptUpdate {
  case none
  case busy(String)
  case finished(ToolRunState, String)
}
