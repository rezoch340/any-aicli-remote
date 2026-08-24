import XCTest
import AnyAICLIRemoteCore
@testable import AnyAICLIRemoteFeature

final class ChatTranscriptReducerTests: XCTestCase {
    func testAgentChunksAreVisibleBeforeCompletion() {
        var blocks: [ChatBlock] = []
        let tracker = PendingUserEchoTracker()
        let first = ChatTranscriptReducer.apply(
            update: ["sessionUpdate": "agent_message_chunk", "text": "第一段"],
            to: &blocks, pendingUserEchoTracker: tracker)
        if case .busy("正在回复") = first {} else { XCTFail("first chunk not busy") }
        XCTAssertEqual(blocks.last?.text, "第一段")
        let second = ChatTranscriptReducer.apply(
            update: ["sessionUpdate": "agent_message_chunk", "text": "第二段"],
            to: &blocks, pendingUserEchoTracker: tracker)
        if case .busy("正在回复") = second {} else { XCTFail("second chunk not busy") }
        XCTAssertEqual(blocks.last?.text, "第一段第二段")
        _ = ChatTranscriptReducer.apply(
            update: ["sessionUpdate": "turn_completed"], to: &blocks,
            pendingUserEchoTracker: tracker)
        XCTAssertEqual(blocks.last?.text, "第一段第二段")
    }
}
