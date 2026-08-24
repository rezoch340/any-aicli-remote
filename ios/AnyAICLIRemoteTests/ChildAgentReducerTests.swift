import XCTest
@testable import AnyAICLIRemoteCore
@testable import AnyAICLIRemoteFeature

final class ChildAgentReducerTests: XCTestCase {
  private func card(
    _ id: String, sequence: Int64? = nil, status: ChildAgentStatus = .running,
    description: String = "old", tool: Int = 1, turns: Int = 1,
    tokens: Int64 = 10, context: Double = 10
  ) -> ChildAgentCard {
    ChildAgentCard(
      providerChildID: id, description: description, status: status,
      toolCallCount: tool, turnCount: turns, tokensUsed: tokens,
      contextUsagePercent: context, sequence: sequence
    )
  }

  func testFirstConcurrentCardsKeepStableOrder() {
    var cards: [ChildAgentCard] = []
    cards = ChildAgentReducer.apply(cards, incoming: card("a"))
    cards = ChildAgentReducer.apply(cards, incoming: card("b"))
    XCTAssertEqual(cards.map(\.providerChildID), ["a", "b"])
  }

  func testSameIDMergesInPlaceAndMonotonicFields() {
    let old = card("a", sequence: 1, description: "old", tool: 3, turns: 4, tokens: 50, context: 20)
    let incoming = card("a", sequence: 2, status: .completed, description: "", tool: 2, turns: 8, tokens: 40, context: 30)
    let result = ChildAgentReducer.apply([old, card("b")], incoming: incoming)
    XCTAssertEqual(result.map(\.providerChildID), ["a", "b"])
    XCTAssertEqual(result[0].description, "old"); XCTAssertEqual(result[0].status, .completed)
    XCTAssertEqual(result[0].toolCallCount, 3); XCTAssertEqual(result[0].turnCount, 8)
    XCTAssertEqual(result[0].tokensUsed, 50); XCTAssertEqual(result[0].contextUsagePercent, 30)
  }

  func testOlderSequenceIsDiscarded() {
    let old = card("a", sequence: 5, status: .completed)
    XCTAssertEqual(ChildAgentReducer.apply([old], incoming: card("a", sequence: 4, status: .failed)), [old])
  }

  func testStatusesApplyForAllTerminalAndUnknownValues() {
    var cards = [card("a", sequence: 1)]
    for (sequence, status) in [(2, ChildAgentStatus.completed), (3, .failed), (4, .cancelled), (5, .unknown)] {
      cards = ChildAgentReducer.apply(cards, incoming: card("a", sequence: Int64(sequence), status: status))
      XCTAssertEqual(cards[0].status, status)
    }
  }
}
