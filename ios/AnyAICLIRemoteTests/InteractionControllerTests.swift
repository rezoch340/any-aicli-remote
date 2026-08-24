import XCTest

@testable import AnyAICLIRemoteCore
@testable import AnyAICLIRemoteFeature

@MainActor
final class InteractionControllerTests: XCTestCase {
  private let identity = SessionIdentity(providerID: "provider-a", sessionID: "session-a")

  private func interaction(rpcID: Int, identity: SessionIdentity? = nil) -> PendingInteraction {
    PendingInteraction(
      rpcID: rpcID, kind: .askQuestion, sessionIdentity: identity ?? self.identity,
      toolCallID: "tool-1", questions: [InteractionQuestion(question: "Pick one")])
  }

  private func store() throws -> ChatStore {
    let store = ChatStore()
    store.selectedSession = try XCTUnwrap(
      SessionSummary(json: ["providerId": identity.providerID, "sessionId": identity.sessionID]))
    return store
  }

  func testReceiveMatchingAndSameRPCIDIsIdempotent() throws {
    let store = try store()
    let first = interaction(rpcID: 7)
    store.interactionController.receive(first)
    store.interactionController.receive(interaction(rpcID: 7, identity: identity))
    XCTAssertEqual(store.pendingInteraction, first)
  }

  func testReceiveWrongIdentityIsIgnored() throws {
    let store = try store()
    store.interactionController.receive(interaction(
      rpcID: 8, identity: SessionIdentity(providerID: "other", sessionID: "session")))
    XCTAssertNil(store.pendingInteraction)
  }

  func testClearRemovesPendingInteraction() throws {
    let store = try store()
    store.interactionController.receive(interaction(rpcID: 9))
    store.interactionController.clear()
    XCTAssertNil(store.pendingInteraction)
  }

  func testAnswerWithoutOwnedSessionRetainsPendingInteraction() throws {
    let store = try store()
    let pending = interaction(rpcID: 10)
    store.interactionController.receive(pending)
    store.interactionController.answer(pending, answer: .approve)
    XCTAssertEqual(store.pendingInteraction, pending)
  }

  func testClearSessionStateClearsInteraction() throws {
    let store = try store()
    store.interactionController.receive(interaction(rpcID: 11))
    store.ownership.clearSessionState()
    XCTAssertNil(store.pendingInteraction)
  }

  func testCancelClearsPendingInteractionWithoutOwnedSession() throws {
    let store = try store()
    let pending = interaction(rpcID: 12)
    store.interactionController.receive(pending)

    store.turnCoordinator.cancel()

    XCTAssertNil(store.pendingInteraction)
  }
}
