import XCTest
@testable import AnyAICLIRemoteCore

final class ChildAgentPayloadMapperTests: XCTestCase {
  private let complete: [String: Any] = ["providerChildId": " child-1 ", "childSessionId": "s1",
    "agentType": "research", "description": "find", "status": "completed", "startedAt": "10",
    "completedAt": NSNumber(value: 20), "toolCallCount": NSNumber(value: 3), "turnCount": "4",
    "modelId": "model", "tokensUsed": "50", "contextUsagePercent": NSNumber(value: 12.5), "sequence": 7]

  func testMapsCompleteCanonicalPayloadAndNumericForms() {
    let card = ChildAgentPayloadMapper.card(from: complete)
    XCTAssertEqual(card?.providerChildID, "child-1"); XCTAssertEqual(card?.status, .completed)
    XCTAssertEqual(card?.startedAt, 10); XCTAssertEqual(card?.completedAt, 20)
    XCTAssertEqual(card?.toolCallCount, 3); XCTAssertEqual(card?.turnCount, 4)
    XCTAssertEqual(card?.tokensUsed, 50); XCTAssertEqual(card?.contextUsagePercent, 12.5)
    XCTAssertEqual(card?.sequence, 7)
  }
  func testUnknownStatusAndEventSequenceOverride() {
    let card = ChildAgentPayloadMapper.card(from: ["providerChildId": "x", "status": "future", "sequence": 1], eventSequence: 9)
    XCTAssertEqual(card?.status, .unknown); XCTAssertEqual(card?.sequence, 9)
  }
  func testRejectsMissingBlankAndAliasIDs() {
    XCTAssertNil(ChildAgentPayloadMapper.card(from: [:]))
    XCTAssertNil(ChildAgentPayloadMapper.card(from: ["providerChildId": "  "]))
    XCTAssertNil(ChildAgentPayloadMapper.card(from: ["providerChildID": "alias"]))
    XCTAssertNil(ChildAgentPayloadMapper.card(from: ["childId": "alias"]))
  }

  func testAcceptsEventNSNumberAndStringSequencesAndNoSequence() {
    let agent: [String: Any] = ["providerChildId": "x"]
    XCTAssertEqual(ChildAgentPayloadMapper.card(fromEvent: ["agent": agent, "sequence": NSNumber(value: 4)])?.sequence, 4)
    XCTAssertEqual(ChildAgentPayloadMapper.card(fromEvent: ["agent": agent, "sequence": "5"])?.sequence, 5)
    XCTAssertNil(ChildAgentPayloadMapper.card(fromEvent: ["agent": agent])?.sequence)
  }

  func testRejectsMalformedSequenceAndMissingAgent() {
    XCTAssertNil(ChildAgentPayloadMapper.card(fromEvent: ["agent": ["providerChildId": "x"], "sequence": "bad"]))
    XCTAssertNil(ChildAgentPayloadMapper.card(fromEvent: ["sequence": 1]))
  }

}
