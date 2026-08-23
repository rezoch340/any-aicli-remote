import XCTest

@testable import AnyAICLIRemoteCore

final class JSONRPCBoundaryTests: XCTestCase {
  func testPermissionAndAskUserAreACPUIRequests() {
    XCTAssertTrue(ACPWire.isPermissionRequest(method: "permission/request"))
    XCTAssertTrue(ACPWire.isPermissionRequest(method: "ask_user"))
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
}
