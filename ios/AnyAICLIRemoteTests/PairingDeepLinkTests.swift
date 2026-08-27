import XCTest
@testable import AnyAICLIRemoteCore

final class PairingDeepLinkTests: XCTestCase {
  func testCurrentSchemeParsesPairingLink() throws {
    let link = try XCTUnwrap(URL(string: "\(ProductIdentifiers.pairingScheme)://pair?url=https%3A%2F%2Fexample.com&key=secret"))

    let pairing = try PairingDeepLink.parse(link)

    XCTAssertEqual(pairing.serviceAddress, "https://example.com")
    XCTAssertEqual(pairing.profile.key, "secret")
  }

  func testLauncherHTTPPairingURLParses() throws {
    let link = try XCTUnwrap(
      URL(string: "http://192.168.1.20:2421/?auto=1&key=secret&name=iPad%20%E7%9C%9F%E6%9C%BA"))

    let pairing = try PairingDeepLink.parse(link)

    XCTAssertEqual(pairing.serviceAddress, "http://192.168.1.20:2421")
    XCTAssertEqual(pairing.profile.baseURL.absoluteString, "http://192.168.1.20:2421")
    XCTAssertEqual(pairing.profile.key, "secret")
    XCTAssertEqual(pairing.name, "iPad 真机")
  }

  func testHTTPURLWithoutAutoPairMarkerIsRejected() {
    let link = URL(string: "http://192.168.1.20:2421/?key=secret")!

    XCTAssertThrowsError(try PairingDeepLink.parse(link))
  }

  func testHTTPURLWithoutPairingKeyIsRejected() {
    let link = URL(string: "http://192.168.1.20:2421/?auto=1")!

    XCTAssertThrowsError(try PairingDeepLink.parse(link))
  }

  func testPreviousSchemeIsRejected() {
    let previousScheme = ["grok", "remote"].joined()
    let link = URL(string: "\(previousScheme)://pair?url=https%3A%2F%2Fexample.com&key=secret")!

    XCTAssertThrowsError(try PairingDeepLink.parse(link))
  }
}
