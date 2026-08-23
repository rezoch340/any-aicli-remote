import XCTest
@testable import AnyAICLIRemote

final class WorkspaceAttachmentTests: XCTestCase {
    func testWorkspaceFileJSONAndURI() {
        let file = WorkspaceFile(json: ["path": "src/a b.swift", "name": "a b.swift", "size": 12], directory: false)
        XCTAssertEqual(file?.size, 12)
        XCTAssertTrue(file?.uri.contains("file:") == true)
    }
    func testPromptTextAndResourceLink() {
        let file = WorkspaceFile(json: ["path": "/tmp/a", "name": "a", "size": 2], directory: false)!
        let blocks = ChatStore.promptBlocks(text: "hi", attachments: [file])
        XCTAssertEqual(blocks.count, 2)
        XCTAssertEqual(blocks[1]["type"] as? String, "resource_link")
    }
    func testAttachmentOnlyAndSizeOmission() {
        let file = WorkspaceFile(json: ["path": "/tmp/a", "name": "a", "size": 0], directory: false)!
        let blocks = ChatStore.promptBlocks(text: "", attachments: [file])
        XCTAssertEqual(blocks.count, 1)
        XCTAssertNil(blocks[0]["size"])
    }
}
