import AnyAICLIRemoteCore
import SwiftUI

struct InteractionSheet: View {
  let interaction: PendingInteraction
  let onAnswer: (InteractionAnswer) -> Void

  var body: some View {
    // Group does not create an accessibility node. A concrete container is
    // required so XCTest can locate the presented interaction root reliably.
    ZStack {
      switch interaction.kind {
      case .askQuestion:
        AskInteractionView(interaction: interaction, onAnswer: onAnswer)
      case .exitPlan:
        PlanApprovalView(interaction: interaction, onAnswer: onAnswer)
      }
    }
    .id(interaction.rpcID)
    .accessibilityElement(children: .contain)
    .accessibilityIdentifier("interaction-sheet")
    .presentationDetents(interaction.kind == .askQuestion ? [.medium, .large] : [.large])
    .presentationDragIndicator(.visible)
    .interactiveDismissDisabled()
  }
}
