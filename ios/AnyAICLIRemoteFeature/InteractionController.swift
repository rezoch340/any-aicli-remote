import AnyAICLIRemoteCore
import Foundation

@MainActor
final class InteractionController {
  private unowned let store: ChatStore
  private let ownership: ChatOwnership

  init(store: ChatStore, ownership: ChatOwnership) {
    self.store = store
    self.ownership = ownership
  }

  func receive(_ interaction: PendingInteraction) {
    guard store.selectedSession?.id == interaction.sessionIdentity else { return }
    if store.pendingInteraction?.rpcID == interaction.rpcID { return }
    store.pendingInteraction = interaction
  }

  func clear() { store.pendingInteraction = nil }

  func answer(_ interaction: PendingInteraction, answer: InteractionAnswer) {
    guard store.pendingInteraction?.rpcID == interaction.rpcID else { return }
    guard let context = ownership.currentSessionContext(sessionIdentity: interaction.sessionIdentity)
    else { return }
    clear()
    Task { [weak self] in
      guard let self else { return }
      guard self.ownership.ownsSession(context) else { return }
      do {
        try await self.store.client.reply(
          id: interaction.rpcID, result: InteractionAnswerCodec.result(answer))
      } catch {
        guard !Task.isCancelled, self.ownership.ownsSession(context),
          self.store.pendingInteraction == nil else { return }
        self.store.pendingInteraction = interaction
        self.store.statusMessage = "交互回复失败：\(error.localizedDescription)"
      }
    }
  }
}
