import XCTest
@testable import AnyAICLIRemoteCore
@testable import AnyAICLIRemoteFeature

final class SessionStatusMapperTests: XCTestCase {
  private let identity = SessionIdentity(providerID: "provider-a", sessionID: "session-a")

  private func statusPayload(_ body: [String: Any]) -> [String: Any] {
    var params: [String: Any] = ["providerId": "provider-a", "sessionId": "session-a"]
    params.merge(body) { _, new in new }
    return ["method": ACPWire.Method.statusUpdate, "params": params]
  }

  func testMapsRetryStatus() {
    let payload = statusPayload(["retry": [
      "phase": "retrying", "attempt": 2, "maxRetries": 5, "reason": "transient"
    ]])
    guard case let .status(status)? = ChatNotificationMapper.map(payload: payload, selectedSessionID: identity) else {
      return XCTFail("expected status")
    }
    XCTAssertEqual(status.retry?.phase, .retrying)
    XCTAssertEqual(status.retry?.attempt, 2)
    XCTAssertEqual(status.retry?.maxRetries, 5)
    XCTAssertNil(status.modelSwitch)
  }

  func testMapsModelSwitchAndFormatsNotice() {
    let payload = statusPayload(["modelSwitch": ["current": "grok-3", "reason": "unavailable"]])
    guard case let .status(status)? = ChatNotificationMapper.map(payload: payload, selectedSessionID: identity) else {
      return XCTFail("expected status")
    }
    XCTAssertEqual(status.modelSwitch?.current, "grok-3")
    XCTAssertTrue(SessionStatusFormatter.notice(status).contains("grok-3"))
  }

  func testEmptyStatusAndWrongSessionRejected() {
    XCTAssertNil(ChatNotificationMapper.map(payload: statusPayload([:]), selectedSessionID: identity))
    var wrongSession = statusPayload(["retry": ["phase": "failed"]])
    wrongSession["params"] = ["providerId": "provider-a", "sessionId": "other", "retry": ["phase": "failed"]]
    XCTAssertNil(ChatNotificationMapper.map(payload: wrongSession, selectedSessionID: identity))
  }

  func testCurrentModeUpdateRoutesToModeNotAsTranscript() {
    let payload: [String: Any] = ["method": ACPWire.Method.sessionUpdate, "params": [
      "providerId": "provider-a", "sessionId": "session-a",
      "update": ["sessionUpdate": "current_mode_update", "currentModeId": "plan"]
    ]]
    guard case let .mode(mode)? = ChatNotificationMapper.map(payload: payload, selectedSessionID: identity) else {
      return XCTFail("expected mode")
    }
    XCTAssertEqual(mode, "plan")
  }
}
