import XCTest
@testable import AnyAICLIRemoteCore

final class WebSocketAuthorizationTests: XCTestCase {
    func testPairingKeyIsOnlyInAuthorizationHeader() throws {
        let pairingKey = "test-pairing-key-7f3c"
        let request = try AnyAICLIRemoteClient.websocketRequest(
            baseURL: XCTUnwrap(URL(string: "http://127.0.0.1:2421/base?old=query#fragment")),
            key: pairingKey
        )

        XCTAssertEqual(request.url?.scheme, "ws")
        XCTAssertEqual(request.url?.path, "/ws")
        XCTAssertNil(request.url?.query)
        XCTAssertNil(request.url?.fragment)
        XCTAssertFalse(request.url?.absoluteString.contains(pairingKey) == true)
        XCTAssertFalse(String(describing: request).contains(pairingKey))
        XCTAssertEqual(request.value(forHTTPHeaderField: ProductIdentifiers.authorizationHeader), pairingKey)
    }

    func testHTTPSCustomPortIsPreserved() throws {
        let request = try AnyAICLIRemoteClient.websocketRequest(
            baseURL: XCTUnwrap(URL(string: "https://example.test:8443/anything?key=old")),
            key: "another-test-key"
        )

        XCTAssertEqual(request.url?.scheme, "wss")
        XCTAssertEqual(request.url?.host, "example.test")
        XCTAssertEqual(request.url?.port, 8443)
        XCTAssertEqual(request.url?.path, "/ws")
        XCTAssertNil(request.url?.query)
    }
}
