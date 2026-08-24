import XCTest

@testable import AnyAICLIRemoteFeature
import AnyAICLIRemoteCore

final class SessionHistoryReconstructionTests: XCTestCase {
  func testReconstructsSupportedHistoryInStableOrderAndIDs() {
    let messages: [[String: Any]] = [
      ["role": "system", "content": "rules"],
      ["role": "user", "content": "hello"],
      ["role": "assistant", "content": "hi"],
      ["role": "tool", "content": "ok"],
      ["role": "unknown", "content": "skip"],
      ["role": "user"]
    ]
    let blocks = SessionPayloadMapper.chatBlocks(from: messages)
    XCTAssertEqual(blocks.map(\.id), ["history-0", "history-1", "history-2", "history-3"])
    XCTAssertEqual(blocks.map(\.kind), [.system, .user, .assistant, .tool])
    XCTAssertEqual(blocks[3].toolState, .success)
    XCTAssertEqual(blocks.map(\.text), ["rules", "hello", "hi", ""])
  }

  func testReconstructsTopLevelChildAgentsAndIgnoresBadRecords() throws {
    let fallback = try XCTUnwrap(SessionSummary(json: ["providerId": "p", "sessionId": "s"]))
    let result = try SessionPayloadMapper.history(from: [
      "session": ["providerId": "p", "sessionId": "s"], "messages": [["role": "user", "content": "hello"]],
      "childAgents": [["providerChildId": "a", "status": "running"], ["providerChildId": "  "]]
    ], fallback: fallback)
    XCTAssertEqual(result.childAgents.map(\.providerChildID), ["a"])
    XCTAssertEqual(result.blocks.count, 1)
    XCTAssertFalse(result.blocks.contains { $0.text == "a" })
  }
}
