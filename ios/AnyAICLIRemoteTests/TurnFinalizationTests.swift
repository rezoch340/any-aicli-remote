import XCTest
@testable import AnyAICLIRemote

final class TurnFinalizationTests: XCTestCase {
    func testBusyRequiresActiveTurn() {
        XCTAssertFalse(ChatStore.shouldMarkTurnBusy(activeTurnID: nil))
        XCTAssertTrue(ChatStore.shouldMarkTurnBusy(activeTurnID: UUID()))
    }

    func testFinalizesPendingAndRunningToolsForEachTerminalState() {
        let blocks = [
            ChatBlock(id: "pending", kind: .tool, toolState: .pending),
            ChatBlock(id: "running", kind: .tool, toolState: .running),
            ChatBlock(id: "done", kind: .tool, toolState: .success),
            ChatBlock(id: "text", kind: .assistant, text: "keep")
        ]
        for finalState in [ToolRunState.success, .cancelled, .failed] {
            let result = ChatStore.finalizingActiveTools(in: blocks, as: finalState)
            XCTAssertEqual(result[0].toolState, finalState)
            XCTAssertEqual(result[1].toolState, finalState)
            XCTAssertEqual(result[2].toolState, .success)
            XCTAssertEqual(result[3], blocks[3])
        }
    }
}
