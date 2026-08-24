import XCTest
@testable import AnyAICLIRemoteCore
@testable import AnyAICLIRemoteFeature

final class ChildAgentNotificationMapperTests: XCTestCase {
  private let identity = SessionIdentity(providerID: "provider-a", sessionID: "session-a")
  private var payload: [String: Any] {
    ["method": ACPWire.Method.childAgentUpdate, "params": [
      "providerId": "provider-a", "sessionId": "session-a",
      "event": ["sequence": "7", "agent": ["providerChildId": "child-1"]]
    ]]
  }

  func testMapsMatchingChildUpdateAndPreservesSequence() {
    guard case let .childAgent(card)? = ChatNotificationMapper.map(
      payload: payload, selectedSessionID: identity
    ) else { return XCTFail("expected child agent") }
    XCTAssertEqual(card.providerChildID, "child-1"); XCTAssertEqual(card.sequence, 7)
  }
  func testRejectsWrongProviderSessionAndSelectedNil() {
    var wrongProvider = payload; wrongProvider["params"] = ["providerId": "other", "sessionId": "session-a", "event": ["agent": ["providerChildId": "x"]]]
    XCTAssertNil(ChatNotificationMapper.map(payload: wrongProvider, selectedSessionID: identity))
    var wrongSession = payload; wrongSession["params"] = ["providerId": "provider-a", "sessionId": "other", "event": ["agent": ["providerChildId": "x"]]]
    XCTAssertNil(ChatNotificationMapper.map(payload: wrongSession, selectedSessionID: identity))
    XCTAssertNil(ChatNotificationMapper.map(payload: payload, selectedSessionID: nil))
  }
  func testRejectsMalformedChildUpdatesAndUnknownMethod() {
    let malformed: [[String: Any]] = [
      ["method": ACPWire.Method.childAgentUpdate],
      ["method": ACPWire.Method.childAgentUpdate, "params": [String: Any]()],
      ["method": ACPWire.Method.childAgentUpdate, "params": [
        "providerId": "provider-a", "sessionId": "session-a"
      ]]
    ]
    for value in malformed {
      XCTAssertNil(ChatNotificationMapper.map(payload: value, selectedSessionID: identity))
    }
    XCTAssertNil(ChatNotificationMapper.map(payload: ["method": "unknown"], selectedSessionID: identity))
  }
  func testEventWithoutSequenceStillMaps() {
    var value = payload
    value["params"] = ["providerId": "provider-a", "sessionId": "session-a", "event": ["agent": ["providerChildId": "x"]]]
    guard case let .childAgent(card)? = ChatNotificationMapper.map(
      payload: value, selectedSessionID: identity
    ) else { return XCTFail("expected child agent") }
    XCTAssertNil(card.sequence)
  }
}
