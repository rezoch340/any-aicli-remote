import XCTest

@testable import AnyAICLIRemoteCore

final class JSONRPCBoundaryTests: XCTestCase {
  func testPermissionAndInteractionCategoriesAreExact() {
    XCTAssertTrue(ACPWire.isPermissionRequest(method: "permission/request"))
    XCTAssertFalse(ACPWire.isPermissionRequest(method: "ask_user"))
    XCTAssertEqual(AnyAICLIRemoteClient.incomingRequestCategory("ask_user"), .unknown)
    XCTAssertEqual(
      AnyAICLIRemoteClient.incomingRequestCategory(ACPWire.Method.interactionRequest), .interaction)
    XCTAssertEqual(
      AnyAICLIRemoteClient.incomingRequestCategory("_x.ai/ask_user_question"), .unknown)
  }

  func testPermissionReplyPayloads() {
    let selected = ACPWire.permissionReplyResult(optionID: "allow")
    XCTAssertEqual(
      selected["outcome"] as? [String: Any] as? [String: String],
      ["outcome": "selected", "optionId": "allow"]
    )
    let cancelled = ACPWire.permissionReplyResult(optionID: nil)
    XCTAssertEqual(
      cancelled["outcome"] as? [String: Any] as? [String: String],
      ["outcome": "cancelled"]
    )
  }

  func testSessionIdentityRequiresProviderAndSession() {
    let expected = SessionIdentity(providerID: "provider-a", sessionID: "session-a")
    XCTAssertTrue(
      ACPWire.matchesSessionIdentity(
        ["providerId": "provider-a", "sessionId": "session-a"],
        expected: expected
      )
    )
    XCTAssertFalse(
      ACPWire.matchesSessionIdentity(
        ["providerId": "provider-b", "sessionId": "session-a"],
        expected: expected
      )
    )
    XCTAssertFalse(
      ACPWire.matchesSessionIdentity(
        ["providerId": "provider-a", "sessionId": "session-b"],
        expected: expected
      )
    )
  }

  func testUnknownNumericRequestGetsMethodNotFoundAndKeepsID() {
    let response = AnyAICLIRemoteClient.methodNotFoundResponse(id: 17, method: "shell/exec")
    XCTAssertEqual(response["id"] as? Int, 17)
    XCTAssertEqual((response["error"] as? [String: Any])?["code"] as? Int, -32601)
  }

  func testUnknownStringRequestGetsMethodNotFoundAndKeepsID() {
    let response = AnyAICLIRemoteClient.methodNotFoundResponse(
      id: "request-17", method: "shell/exec")
    XCTAssertEqual(response["id"] as? String, "request-17")
    XCTAssertEqual((response["error"] as? [String: Any])?["code"] as? Int, -32601)
  }

  func testNotificationIsNotClassifiedAsRequest() {
    XCTAssertEqual(
      AnyAICLIRemoteClient.incomingRequestCategory(["method": "shell/exec"]),
      .notification
    )
  }

  func testPermissionRequestAndInteractionNotificationBoundaries() {
    XCTAssertEqual(
      AnyAICLIRemoteClient.incomingRequestCategory([
        "id": 3, "method": "permission/request", "params": [:]
      ]), .permission)
    XCTAssertEqual(
      AnyAICLIRemoteClient.incomingRequestCategory([
        "id": 3, "method": ACPWire.Method.interactionRequest, "params": [:]
      ]), .interaction)
    XCTAssertEqual(
      AnyAICLIRemoteClient.incomingRequestCategory([
        "method": ACPWire.Method.interactionRequest, "params": [:]
      ]), .notification)
  }
}
