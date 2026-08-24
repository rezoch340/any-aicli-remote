import AnyAICLIRemoteCore
import SwiftUI

struct PlanApprovalView: View {
  let interaction: PendingInteraction
  let onAnswer: (InteractionAnswer) -> Void

  @State private var feedback = ""

  var body: some View {
    VStack(alignment: .leading, spacing: 0) {
      Text("计划待批准")
        .font(.title3.weight(.semibold))
        .padding(.horizontal, 20)
        .padding(.top, 16)
        .padding(.bottom, 12)
        .accessibilityIdentifier("interaction-plan-sheet")

      Divider()

      ScrollViewReader { proxy in
        ScrollView {
          VStack(alignment: .leading, spacing: 0) {
            Color.clear
              .frame(height: 0)
              .id("interaction-plan-top")
            Text(planContent)
              .font(.body)
              .textSelection(.enabled)
              .frame(maxWidth: .infinity, alignment: .leading)
              .padding(12)
              .background(Color.secondary.opacity(0.12), in: RoundedRectangle(cornerRadius: 10))
              .accessibilityIdentifier("interaction-plan-content")
          }
          .padding(20)
        }
        .onAppear { scrollToTop(proxy) }
        .onChange(of: interaction.rpcID) { scrollToTop(proxy) }
      }

      Divider()

      VStack(alignment: .leading, spacing: 8) {
        Text("修改意见（可选，用于请求修改）")
          .font(.caption)
          .foregroundStyle(.secondary)
        TextField("修改意见（可选，用于请求修改）", text: $feedback, axis: .vertical)
          .lineLimit(3...8)
          .textFieldStyle(.roundedBorder)
          .accessibilityIdentifier("interaction-plan-feedback")
      }
      .padding(.horizontal, 20)
      .padding(.vertical, 12)

      HStack(spacing: 12) {
        Button("放弃", role: .destructive) {
          onAnswer(.abandon)
        }
        .accessibilityIdentifier("interaction-plan-abandon")

        Spacer()

        Button("请求修改") {
          onAnswer(.cancel(feedback: feedback))
        }
        .accessibilityIdentifier("interaction-plan-revise")

        Button("批准并开始") {
          onAnswer(.approve)
        }
        .accessibilityIdentifier("interaction-plan-approve")
      }
      .padding(.horizontal, 20)
      .padding(.bottom, 12)
    }
  }

  private var planContent: String {
    interaction.planContent.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty
      ? "（尚未写出计划）"
      : interaction.planContent
  }

  private func scrollToTop(_ proxy: ScrollViewProxy) {
    DispatchQueue.main.async {
      proxy.scrollTo("interaction-plan-top", anchor: .top)
    }
  }
}
