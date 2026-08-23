import XCTest
@testable import AnyAICLIRemote
final class TurnFinalizationTests: XCTestCase {
    func testBusyRequiresActiveTurn() {
        XCTAssertFalse(ChatStore.shouldMarkTurnBusy(activeTurnID: nil))
        XCTAssertTrue(ChatStore.shouldMarkTurnBusy(activeTurnID: UUID()))
    }
}
