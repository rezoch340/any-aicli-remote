import XCTest
@testable import AnyAICLIRemote

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
        let blocks = ChatStore.chatBlocks(from: messages)
        XCTAssertEqual(blocks.map(\.id), ["history-0", "history-1", "history-2", "history-3"])
        XCTAssertEqual(blocks.map(\.kind), [.system, .user, .assistant, .tool])
        XCTAssertEqual(blocks[3].toolState, .success)
        XCTAssertEqual(blocks.map(\.text), ["rules", "hello", "hi", ""])
    }
}
