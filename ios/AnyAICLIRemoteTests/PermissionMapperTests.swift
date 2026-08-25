import XCTest
@testable import AnyAICLIRemoteCore
@testable import AnyAICLIRemoteFeature

final class PermissionMapperTests: XCTestCase {
  private let identity = SessionIdentity(providerID: "provider-a", sessionID: "session-a")

  func testPermissionQuestionComesFromToolCallTitle() {
    let payload: [String: Any] = [
      "id": 7, "method": "session/request_permission",
      "params": [
        "providerId": "provider-a", "sessionId": "session-a",
        "toolCall": ["title": "bash: ls -la"],
        "options": [["optionId": "allow-once", "name": "允许一次"]]
      ]
    ]
    guard case let .permission(request)? = ChatNotificationMapper.map(
      payload: payload, selectedSessionID: identity)
    else { return XCTFail("expected permission") }
    XCTAssertEqual(request.question, "bash: ls -la")
    XCTAssertEqual(request.options.first?.id, "allow-once")
  }

  func testPermissionFallsBackWhenNoToolCallTitle() {
    let payload: [String: Any] = [
      "id": 8, "method": "session/request_permission",
      "params": ["providerId": "provider-a", "sessionId": "session-a", "options": []]
    ]
    guard case let .permission(request)? = ChatNotificationMapper.map(
      payload: payload, selectedSessionID: identity)
    else { return XCTFail("expected permission") }
    XCTAssertFalse(request.question.isEmpty)
  }
}
