import XCTest
@testable import AnyAICLIRemoteFeature

final class StreamingMarkdownTextTests: XCTestCase {
    func testInitiallyCompletedUsesStaticRenderer() {
        XCTAssertEqual(StreamingMarkdownRenderLifecycle(isStreaming: false), .completed)
        XCTAssertFalse(StreamingMarkdownRenderLifecycle(isStreaming: false).usesStreamingRenderer)
    }

    func testInitiallyStreamingUsesStreamedRenderer() {
        XCTAssertTrue(StreamingMarkdownRenderLifecycle(isStreaming: true).usesStreamingRenderer)
    }

    func testCompletedAfterStreamingKeepsStreamedRenderer() {
        var lifecycle = StreamingMarkdownRenderLifecycle(isStreaming: true)
        lifecycle.observe(isStreaming: false)
        XCTAssertTrue(lifecycle.usesStreamingRenderer)
    }

    func testStreamingAnimationIsEnabled() {
        XCTAssertTrue(StreamingMarkdownRenderConfiguration.animatesStreamedText)
    }
}
