import XCTest
@testable import AnyAICLIRemoteCore

final class RESTPathEncodingTests: XCTestCase {
    func testSessionIdentifierIsEncodedAsOnePathSegment() throws {
        let encodedPath = try AnyAICLIRemoteClient.percentEncodedPath(
            components: ["api", "sessions", "session/with %?#", "messages"]
        )

        XCTAssertEqual(
            encodedPath,
            "/api/sessions/session%2Fwith%20%25%3F%23/messages"
        )
    }

    func testSessionMetadataUsesProviderScopedIdentityAndNormalizedFields() throws {
        let session = try XCTUnwrap(SessionSummary(json: [
            "providerId": "provider-one",
            "sessionId": "shared-session",
            "title": "Normalized",
            "projectDir": "/workspace/project",
            "createdAt": 1_776_900_000_000,
            "lastActiveAt": 1_776_900_123_000
        ]))
        let otherProviderSession = try XCTUnwrap(SessionSummary(json: [
            "providerId": "other-provider",
            "sessionId": "shared-session"
        ]))

        XCTAssertEqual(session.id, SessionIdentity(providerID: "provider-one", sessionID: "shared-session"))
        XCTAssertNotEqual(session.id, otherProviderSession.id)
        XCTAssertEqual(session.projectDirectory, "/workspace/project")
        XCTAssertEqual(session.createdAt?.timeIntervalSince1970, 1_776_900_000)
        XCTAssertEqual(session.lastActiveAt?.timeIntervalSince1970, 1_776_900_123)
    }
}
