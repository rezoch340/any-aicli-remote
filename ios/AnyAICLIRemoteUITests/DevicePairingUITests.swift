import XCTest

final class DevicePairingUITests: XCTestCase {
    func testOpenAndCancelManualDevicePairing() {
        let application = XCUIApplication()
        application.launch()

        XCTAssertTrue(application.navigationBars["设备"].waitForExistence(timeout: 5))
        XCTAssertTrue(application.staticTexts["还没有设备"].exists)
        XCTAssertTrue(application.buttons["手动添加设备"].exists)

        application.buttons["手动添加设备"].tap()

        XCTAssertTrue(application.navigationBars["添加设备"].waitForExistence(timeout: 5))
        XCTAssertTrue(application.textFields["http://mac.local:2421"].exists)
        XCTAssertTrue(application.secureTextFields["配对 Key"].exists)
        XCTAssertTrue(application.buttons["取消"].exists)
        XCTAssertTrue(application.buttons["保存"].exists)

        application.buttons["取消"].tap()
        XCTAssertTrue(application.navigationBars["设备"].waitForExistence(timeout: 5))
        XCTAssertTrue(application.buttons["手动添加设备"].exists)
    }

    func testPairAndOpenSessionListAgainstLiveDaemon() throws {
        guard ProcessInfo.processInfo.environment["ANY_AI_CLI_REMOTE_LIVE_UI_TEST"] == "1" else {
            throw XCTSkip("Live daemon UI test is opt-in")
        }

        let application = XCUIApplication()
        application.launch()
        XCTAssertTrue(application.buttons["手动添加设备"].waitForExistence(timeout: 5))
        application.buttons["手动添加设备"].tap()

        let nameField = application.textFields["名称（例如：工作室 Mac）"]
        let addressField = application.textFields["http://mac.local:2421"]
        let pairingField = application.secureTextFields["配对 Key"]
        XCTAssertTrue(nameField.waitForExistence(timeout: 5))
        nameField.tap()
        nameField.typeText("模拟器服务")
        addressField.tap()
        addressField.typeText("http://127.0.0.1:2421")
        pairingField.press(forDuration: 1.0)

        let chinesePaste = application.menuItems["粘贴"]
        let englishPaste = application.menuItems["Paste"]
        if chinesePaste.waitForExistence(timeout: 2) {
            chinesePaste.tap()
        } else {
            XCTAssertTrue(englishPaste.waitForExistence(timeout: 2))
            englishPaste.tap()
        }

        application.buttons["保存"].tap()
        let dismissPasswordPrompt = application.buttons["以后"]
        if dismissPasswordPrompt.waitForExistence(timeout: 2) {
            dismissPasswordPrompt.tap()
        }
        let deviceRow = application.buttons
            .containing(.staticText, identifier: "模拟器服务")
            .firstMatch
        XCTAssertTrue(deviceRow.waitForExistence(timeout: 5))
        deviceRow.tap()
        XCTAssertTrue(application.navigationBars["模拟器服务"].waitForExistence(timeout: 10))
        _ = application.staticTexts["在线"].waitForExistence(timeout: 5)
    }

    func testStreamingResponseAutoScrollAgainstLiveDaemon() throws {
        guard ProcessInfo.processInfo.environment["ANY_AI_CLI_REMOTE_LIVE_UI_TEST"] == "1" else {
            throw XCTSkip("Live daemon UI test is opt-in")
        }

        let application = XCUIApplication()
        application.launch()
        let deviceRow = application.buttons
            .containing(.staticText, identifier: "模拟器服务")
            .firstMatch
        XCTAssertTrue(deviceRow.waitForExistence(timeout: 10))
        deviceRow.tap()
        XCTAssertTrue(application.navigationBars["模拟器服务"].waitForExistence(timeout: 10))

        application.buttons["新建会话"].tap()
        XCTAssertTrue(application.navigationBars["新建会话"].waitForExistence(timeout: 5))
        application.buttons["创建"].tap()

        let composer = application.descendants(matching: .any)["chat-composer"]
        XCTAssertTrue(composer.waitForExistence(timeout: 10))
        composer.tap()
        composer.typeText("请不要调用工具。请连续输出四十行编号短句，每行内容不同，用于测试移动端流式吐字与自动滚动。最后单独输出由 AUTOSCROLL、END、MARKER 三段拼接成的全大写字符串，不要分隔符。")
        application.buttons["发送"].tap()
        XCTAssertTrue(application.staticTexts["等待助手"].waitForExistence(timeout: 10))
        XCTAssertTrue(application.buttons["停止生成"].waitForExistence(timeout: 5))
        attachScreenshot(named: "streaming-started")
        let chatStatus = application.descendants(matching: .any)["chat-status"]
        let completedPredicate = NSPredicate(format: "label == %@ OR value == %@", "完成", "完成")
        let completedExpectation = XCTNSPredicateExpectation(predicate: completedPredicate, object: chatStatus)
        wait(for: [completedExpectation], timeout: 120)
        let markerPredicate = NSPredicate(format: "label CONTAINS %@", "AUTOSCROLLENDMARKER")
        let markerText = application.textViews.matching(markerPredicate).firstMatch
        XCTAssertTrue(markerText.waitForExistence(timeout: 10))
        assertAssistantAtComposer(application: application)
        attachScreenshot(named: "streaming-completed")
        XCTAssertTrue(application.descendants(matching: .any)["chat-composer"].waitForExistence(timeout: 5))

        application.terminate()
        application.launch()
        let reopenedDevice = application.buttons
            .containing(.staticText, identifier: "模拟器服务")
            .firstMatch
        XCTAssertTrue(reopenedDevice.waitForExistence(timeout: 10))
        reopenedDevice.tap()
        XCTAssertTrue(application.navigationBars["模拟器服务"].waitForExistence(timeout: 10))
        XCTAssertTrue(application.cells.firstMatch.waitForExistence(timeout: 10))
        application.cells.firstMatch.tap()
        XCTAssertTrue(application.descendants(matching: .any)["chat-composer"].waitForExistence(timeout: 10))
        let reopenedMarker = application.textViews.matching(markerPredicate).firstMatch
        XCTAssertTrue(reopenedMarker.waitForExistence(timeout: 10))
        assertAssistantAtComposer(application: application)
        attachScreenshot(named: "session-reopened-at-bottom")
    }

    private func assertAssistantAtComposer(application: XCUIApplication) {
        let assistantMessages = application.descendants(matching: .any).matching(identifier: "assistant-message")
        let composer = application.descendants(matching: .any)["chat-composer"]
        XCTAssertTrue(assistantMessages.firstMatch.waitForExistence(timeout: 10))
        XCTAssertTrue(composer.waitForExistence(timeout: 10))
        guard assistantMessages.count > 0 else {
            XCTFail("Missing assistant message")
            return
        }
        let assistantMessage = assistantMessages.element(boundBy: assistantMessages.count - 1)
        let maximumComposerGap: CGFloat = 80
        let minimumVisibleOverlap: CGFloat = -20
        let assistantFrame = assistantMessage.frame
        let composerFrame = composer.frame
        XCTAssertTrue(assistantFrame.intersects(application.windows.firstMatch.frame))
        let composerGap = composerFrame.minY - assistantFrame.maxY
        XCTAssertGreaterThanOrEqual(composerGap, minimumVisibleOverlap)
        XCTAssertLessThanOrEqual(composerGap, maximumComposerGap)
    }

    private func attachScreenshot(named name: String) {
        let attachment = XCTAttachment(screenshot: XCUIScreen.main.screenshot())
        attachment.name = name
        attachment.lifetime = .keepAlways
        add(attachment)
    }
}
