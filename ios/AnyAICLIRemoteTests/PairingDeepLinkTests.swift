import XCTest
@testable import AnyAICLIRemoteCore

final class PairingDeepLinkTests: XCTestCase {
  func testCurrentSchemeParsesPairingLink() throws {
    let link = try XCTUnwrap(URL(string: "\(ProductIdentifiers.pairingScheme)://pair?url=https%3A%2F%2Fexample.com&key=secret"))

    let pairing = try PairingDeepLink.parse(link)

    XCTAssertEqual(pairing.serviceAddress, "https://example.com")
    XCTAssertEqual(pairing.profile.key, "secret")
  }

  func testPreviousSchemeIsRejected() {
    let previousScheme = ["grok", "remote"].joined()
    let link = URL(string: "\(previousScheme)://pair?url=https%3A%2F%2Fexample.com&key=secret")!

    XCTAssertThrowsError(try PairingDeepLink.parse(link))
  }
}
