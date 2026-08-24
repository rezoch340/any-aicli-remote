import AnyAICLIRemoteCore
import SwiftUI

struct ChildAgentStripView: View {
  let cards: [ChildAgentCard]
  var body: some View {
    if cards.isEmpty { EmptyView() } else {
    ScrollView(.horizontal, showsIndicators: false) {
      LazyHStack(spacing: 12) {
        ForEach(cards) { card in
          VStack(alignment: .leading, spacing: 4) {
            Text(card.agentType.isEmpty ? "子 Agent" : card.agentType)
            if !card.description.isEmpty { Text(card.description).lineLimit(2) }
            Text(status(card.status)).font(.caption)
            let metrics = metricText(card)
            if !metrics.isEmpty { Text(metrics).font(.caption2) }
          }
          .frame(width: 190, alignment: .leading)
          .padding(8).background(.thinMaterial).clipShape(RoundedRectangle(cornerRadius: 8))
          .accessibilityIdentifier("child-agent-card-\(card.providerChildID)")
          .accessibilityLabel("\(card.agentType.isEmpty ? "子 Agent" : card.agentType)\(card.description.isEmpty ? "" : " \(card.description)")")
          .accessibilityValue(status(card.status))
        }
      }.padding(.horizontal)
    }.accessibilityIdentifier("child-agent-strip") }
  }
  private func metricText(_ card: ChildAgentCard) -> String {
    var values: [String] = []
    if card.turnCount > 0 { values.append("轮次 \(card.turnCount)") }
    if card.toolCallCount > 0 { values.append("工具 \(card.toolCallCount)") }
    if card.tokensUsed > 0 { values.append("tokens \(card.tokensUsed)") }
    if card.contextUsagePercent > 0 { values.append("上下文 \(card.contextUsagePercent)%") }
    return values.isEmpty ? "等待中" : values.joined(separator: " · ")
  }
  private func status(_ value: ChildAgentStatus) -> String {
    switch value {
    case .running: return "运行中"
    case .completed: return "已完成"
    case .failed: return "失败"
    case .cancelled: return "已取消"
    case .unknown: return "未知"
    }
  }
}
