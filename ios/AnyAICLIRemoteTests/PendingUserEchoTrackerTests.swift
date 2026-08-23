import XCTest

@testable import AnyAICLIRemoteFeature
import AnyAICLIRemoteCore

final class PendingUserEchoTrackerTests: XCTestCase {
  func testFullEchoIsConsumed() {
    let tracker = PendingUserEchoTracker()
    tracker.begin(text: "hello")
    XCTAssertTrue(tracker.consume(chunk: "hello"))
    XCTAssertFalse(tracker.consume(chunk: "hello"))
  }
  func testDeltaChunksAreConsumed() {
    let tracker = PendingUserEchoTracker()
    tracker.begin(text: "hello")
    XCTAssertTrue(tracker.consume(chunk: "hel"))
    XCTAssertTrue(tracker.consume(chunk: "lo"))
  }
  func testCumulativeSnapshotIsConsumed() {
    let tracker = PendingUserEchoTracker()
    tracker.begin(text: "hello")
    XCTAssertTrue(tracker.consume(chunk: "hel"))
    XCTAssertTrue(tracker.consume(chunk: "hello"))
  }
  func testOverlapIsConsumed() {
    let tracker = PendingUserEchoTracker()
    tracker.begin(text: "hello")
    XCTAssertTrue(tracker.consume(chunk: "hel"))
    XCTAssertTrue(tracker.consume(chunk: "ello"))
  }
  func testUnrelatedChunkClearsPending() {
    let tracker = PendingUserEchoTracker()
    tracker.begin(text: "hello")
    XCTAssertFalse(tracker.consume(chunk: "xyz"))
    XCTAssertFalse(tracker.consume(chunk: "hello"))
  }
  func testClearDropsPending() {
    let tracker = PendingUserEchoTracker()
    tracker.begin(text: "hello")
    tracker.clear()
    XCTAssertFalse(tracker.consume(chunk: "hello"))
  }
  func testEmptyChunkIsConsumedWhilePending() {
    let tracker = PendingUserEchoTracker()
    tracker.begin(text: "hello")
    XCTAssertTrue(tracker.consume(chunk: ""))
    XCTAssertTrue(tracker.consume(chunk: "hello"))
  }
}
