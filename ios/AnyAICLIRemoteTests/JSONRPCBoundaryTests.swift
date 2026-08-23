import XCTest
@testable import AnyAICLIRemote

final class JSONRPCBoundaryTests: XCTestCase {
    func testPermissionAndAskUserAreUIRequests() {
        XCTAssertEqual(AnyAICLIRemoteClient.incomingRequestCategory("permission/request"), .permission)
        XCTAssertEqual(AnyAICLIRemoteClient.incomingRequestCategory("ask_user"), .permission)
    }

    func testUnknownNumericRequestGetsMethodNotFoundAndKeepsID() {
        let response = AnyAICLIRemoteClient.methodNotFoundResponse(id: 17, method: "shell/exec")
        XCTAssertEqual(response["id"] as? Int, 17)
        XCTAssertEqual((response["error"] as? [String: Any])?["code"] as? Int, -32601)
    }

    func testUnknownStringRequestGetsMethodNotFoundAndKeepsID() {
        let response = AnyAICLIRemoteClient.methodNotFoundResponse(id: "request-17", method: "shell/exec")
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
