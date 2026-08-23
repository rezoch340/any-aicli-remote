import Foundation
import XCTest

final class DaemonControllerSupportTests: XCTestCase {
  func testLifecycleGenerationRejectsOldTokenAndAcceptsCurrentToken() {
    var generation = LifecycleGeneration()
    let firstToken = generation.next()
    let currentToken = generation.next()
    XCTAssertFalse(generation.accepts(firstToken))
    XCTAssertTrue(generation.accepts(currentToken))
  }

  func testDaemonLocatorBuildsSanitizedDeduplicatedPath() throws {
    let policy = try policyFixture()
    let environment = DaemonLocator.launchEnvironment(
      policy: policy,
      environment: [
        "PATH": "/usr/bin:/custom/bin:/usr/bin::middle~value",
        "ANY_AI_CLI_REMOTE_PORT": "2421",
      ],
      homeDirectory: URL(fileURLWithPath: "/Users/tester"))
    XCTAssertEqual(
      environment["PATH"],
      "/Users/tester/.local/bin:/Users/tester/bin:/opt/homebrew/bin:/usr/local/bin:/usr/bin:/custom/bin:middle~value"
    )
    XCTAssertNil(environment["ANY_AI_CLI_REMOTE_PORT"])
  }

  func testRuntimePairingAcceptsValidPayload() throws {
    let configuration = RuntimeConfiguration(
      pairingURL: "https://example.test/pairing",
      pairingDeepLink: "anyaicliremote://pair/token",
      lanAddress: "192.0.2.10")
    let pairing = try RuntimePairing(configuration: configuration)
    XCTAssertEqual(pairing.httpURL.scheme, "https")
    XCTAssertEqual(pairing.deepLinkURL.host, ProductIdentifier.deepLinkHost)
  }

  func testRuntimePairingRejectsInvalidPayloads() {
    let invalidConfigurations = [
      RuntimeConfiguration(
        pairingURL: "ftp://example.test/pairing",
        pairingDeepLink: "anyaicliremote://pair/token",
        lanAddress: nil),
      RuntimeConfiguration(
        pairingURL: "https:///pairing",
        pairingDeepLink: "anyaicliremote://pair/token",
        lanAddress: nil),
      RuntimeConfiguration(
        pairingURL: "https://example.test/pairing",
        pairingDeepLink: "wrongscheme://pair/token",
        lanAddress: nil),
      RuntimeConfiguration(
        pairingURL: "https://example.test/pairing",
        pairingDeepLink: "anyaicliremote://other/token",
        lanAddress: nil),
    ]
    for configuration in invalidConfigurations {
      XCTAssertThrowsError(try RuntimePairing(configuration: configuration))
    }
  }

  func testLogBufferLimitsAndClearDropsPartialSecret() {
    let secret = "secret-value"
    var plainBuffer = LauncherLogBuffer(
      maximumChunkCharacters: 4,
      maximumEntries: 5,
      redactedValues: { [secret] })
    plainBuffer.append("secret sauce stays")
    XCTAssertEqual(plainBuffer.entries.last?.message, "secret sauce stays")
    var buffer = LauncherLogBuffer(
      maximumChunkCharacters: 4,
      maximumEntries: 2,
      redactedValues: { [secret] })
    buffer.append("first")
    buffer.append("second")
    buffer.append("third")
    buffer.appendChunk("secret-")
    buffer.appendChunk("value\nvisible")
    let messages = buffer.entries.map(\.message).joined()
    XCTAssertFalse(messages.contains(secret))
    XCTAssertFalse(messages.contains("secr"))
    XCTAssertFalse(messages.contains("et-value"))
    XCTAssertTrue(messages.contains("••••••••"))
    XCTAssertLessThanOrEqual(buffer.entries.count, 2)
    buffer.clear()
    buffer.appendChunk("partial")
    buffer.clear()
    buffer.appendChunk("line\n")
    XCTAssertFalse(buffer.entries.map(\.message).joined().contains("partial"))
    XCTAssertEqual(buffer.entries.last?.message, "line")
  }

  private func policyFixture() throws -> LauncherPolicy {
    let resourceURL = URL(fileURLWithPath: #filePath)
      .deletingLastPathComponent()
      .deletingLastPathComponent()
      .appendingPathComponent("Resources/LauncherPolicy.json")
    let data = try Data(contentsOf: resourceURL)
    return try JSONDecoder().decode(LauncherPolicy.self, from: data)
  }
}
