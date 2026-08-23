import XCTest
@testable import AnyAICLIRemote

final class PairingDeepLinkTests: XCTestCase {
    private let serviceKey = "service-secret"

    func testCurrentAndLegacyLinksParse() throws {
        for scheme in [ProductIdentifiers.pairingScheme, LegacyCompatibility.pairingScheme] {
            let link = try XCTUnwrap(URL(string: "\(scheme)://pair?url=https%3A%2F%2Fexample.test%3A8443&key=\(serviceKey)"))
            let parsed = try PairingDeepLink.parse(link)
            XCTAssertEqual(parsed.profile.baseURL.port, 8443)
            XCTAssertEqual(parsed.profile.key, serviceKey)
        }
    }

    func testEmbeddedKeyIsAcceptedAndRemovedFromBaseURL() throws {
        let link = try XCTUnwrap(URL(string: "\(ProductIdentifiers.pairingScheme)://pair?url=https%3A%2F%2Fexample.test%3A8443%2Fpath%3Fkey%3D\(serviceKey)%26x%3Dy"))
        let parsed = try PairingDeepLink.parse(link)
        XCTAssertEqual(parsed.profile.key, serviceKey)
        XCTAssertEqual(parsed.profile.baseURL.absoluteString, "https://example.test:8443")
        XCTAssertFalse(parsed.profile.baseURL.absoluteString.contains(serviceKey))
    }

    func testInvalidLinksAreRejected() throws {
        let current = ProductIdentifiers.pairingScheme
        let cases = [
            "wrong://pair?url=https%3A%2F%2Fexample.test&key=x",
            "\(current)://other?url=https%3A%2F%2Fexample.test&key=x",
            "\(current)://pair?key=x",
            "\(current)://pair?url=https%3A%2F%2Fexample.test",
            "\(current)://pair?url=ftp%3A%2F%2Fexample.test&key=x",
        ]
        for value in cases {
            XCTAssertThrowsError(try PairingDeepLink.parse(try XCTUnwrap(URL(string: value))), "Expected rejection for \(value)")
        }
    }
}
