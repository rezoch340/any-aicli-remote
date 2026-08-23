import Foundation
import UIKit
import XCTest

final class DevicePairingUITests: XCTestCase {
    private enum PairingKeyInput {
        case typed(String)
        case file(path: String)
    }

    private let resetArgument = "--ui-testing-reset-storage"

    private func launchApplication() -> XCUIApplication {
        let application = XCUIApplication()
        application.launchArguments.append(resetArgument)
        application.launch()
        return application
    }

    func testOpenAndCancelManualDevicePairing() {
        let application = launchApplication()

        XCTAssertTrue(application.navigationBars["设备"].waitForExistence(timeout: 5))
        XCTAssertTrue(application.staticTexts["还没有设备"].exists)
        let brandText = application.staticTexts
            .containing(NSPredicate(format: "label CONTAINS %@", "Any AI CLI Remote"))
            .firstMatch
        XCTAssertTrue(brandText.exists)
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

    func testOfflineDevicesPersistAndConnectionFailureReturnsToDeviceList() throws {
        let application = launchApplication()
        XCTAssertTrue(application.buttons["手动添加设备"].waitForExistence(timeout: 5))
        let offlineKeyInput = PairingKeyInput.typed("offline-test-pairing-key")
        try addDevice(
            application: application,
            name: "离线设备一",
            address: "http://127.0.0.1:1",
            pairingKeyInput: offlineKeyInput
        )
        try addDevice(
            application: application,
            name: "离线设备二",
            address: "http://127.0.0.1:2",
            pairingKeyInput: offlineKeyInput
        )
        let firstOfflineRow = deviceButton(application: application, name: "离线设备一")
        let secondOfflineRow = deviceButton(application: application, name: "离线设备二")
        let ready = NSPredicate(format: "exists == true AND isHittable == true")
        try require(expect(firstOfflineRow, matching: ready, timeout: 5), "第一台设备不可操作")
        try require(expect(secondOfflineRow, matching: ready, timeout: 5), "第二台设备不可操作")
        application.terminate()
        application.launchArguments.removeAll { $0 == resetArgument }
        application.launch()
        let reopenedFirstRow = deviceButton(application: application, name: "离线设备一")
        let reopenedSecondRow = deviceButton(application: application, name: "离线设备二")
        try require(reopenedFirstRow.waitForExistence(timeout: 5), "重启后第一台设备未持久化")
        try require(reopenedSecondRow.waitForExistence(timeout: 5), "重启后第二台设备未持久化")
        reopenedFirstRow.tap()
        try require(application.navigationBars["设备"].waitForExistence(timeout: 10), "离线连接后未回到设备页")
        try require(deviceButton(application: application, name: "离线设备一").exists, "第一台设备丢失")
        try require(deviceButton(application: application, name: "离线设备二").exists, "第二台设备丢失")
    }

    func testPairAndOpenSessionListAgainstLiveDaemon() throws {
        guard ProcessInfo.processInfo.environment["ANY_AI_CLI_REMOTE_LIVE_UI_TEST"] == "1" else {
            throw XCTSkip("Live daemon UI test is opt-in")
        }

        let application = launchApplication()
        try addDevice(
            application: application,
            name: "模拟器服务",
            address: "http://127.0.0.1:2421",
            pairingKeyInput: .file(path: try livePairingKeyFilePath())
        )
        application.terminate()
        application.launchArguments.removeAll { $0 == resetArgument }
        application.launch()
        let reopenedDeviceRow = deviceButton(application: application, name: "模拟器服务")
        try require(reopenedDeviceRow.waitForExistence(timeout: 10), "重启后未找到已配对设备")
        reopenedDeviceRow.tap()
        try require(application.navigationBars["模拟器服务"].waitForExistence(timeout: 10), "未进入已配对设备")
        try require(application.cells.firstMatch.waitForExistence(timeout: 10), "使用已保存 Key 鉴权后未进入会话列表")
    }

    func testStreamingResponseAutoScrollAgainstLiveDaemon() throws {
        guard ProcessInfo.processInfo.environment["ANY_AI_CLI_REMOTE_LIVE_UI_TEST"] == "1" else {
            throw XCTSkip("Live daemon UI test is opt-in")
        }

        let application = launchApplication()
        try addDevice(
            application: application,
            name: "模拟器服务",
            address: "http://127.0.0.1:2421",
            pairingKeyInput: .file(path: try livePairingKeyFilePath())
        )
        let deviceRow = deviceButton(application: application, name: "模拟器服务")
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
        application.launchArguments.removeAll { $0 == resetArgument }
        application.launch()
        let reopenedDevice = deviceButton(application: application, name: "模拟器服务")
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

    private func livePairingKeyFilePath() throws -> String {
        let path = ProcessInfo.processInfo.environment["ANY_AI_CLI_REMOTE_LIVE_PAIRING_KEY_FILE"] ?? ""
        try require(!path.isEmpty, "未提供 live 配对 Key 文件")
        return path
    }

    private func addDevice(
        application: XCUIApplication,
        name: String,
        address: String,
        pairingKeyInput: PairingKeyInput
    ) throws {
        try require(application.navigationBars["设备"].waitForExistence(timeout: 5), "设备页未出现")
        let addButton = application.buttons["添加设备"]
        if addButton.waitForExistence(timeout: 2) {
            addButton.tap()
        } else {
            let manualButton = application.buttons["手动添加设备"]
            try require(manualButton.waitForExistence(timeout: 5), "手动添加设备按钮未出现")
            manualButton.tap()
        }
        try require(application.navigationBars["添加设备"].waitForExistence(timeout: 5), "添加设备页未出现")
        let nameField = application.textFields["名称（例如：工作室 Mac）"]
        try typeAndAwaitValue(nameField, expected: name, fieldName: "名称")
        let addressField = application.textFields["http://mac.local:2421"]
        try typeAndAwaitValue(addressField, expected: address, fieldName: "服务地址")
        let pairingField = application.secureTextFields["配对 Key"]
        switch pairingKeyInput {
        case let .typed(pairingKey):
            try typeAndAwaitSecureValue(pairingField, expected: pairingKey, fieldName: "配对 Key")
        case let .file(path):
            let expectedKey = try String(contentsOfFile: path, encoding: .utf8)
                .trimmingCharacters(in: .whitespacesAndNewlines)
            try require(!expectedKey.isEmpty, "配对 Key 文件为空")
            UIPasteboard.general.string = expectedKey
            try pasteAndAwaitSecureValue(application: application, field: pairingField, expected: expectedKey)
        }
        let saveButton = application.buttons["保存"]
        try require(saveButton.waitForExistence(timeout: 5), "保存按钮未出现")
        try require(saveButton.isEnabled && saveButton.isHittable, "保存按钮不可用或不可点击")
        saveButton.tap()
        try dismissPasswordAutofillPromptIfPresent(application: application)
        let deviceList = application.navigationBars["设备"]
        let deviceRow = deviceButton(application: application, name: name)
        let ready = NSPredicate(format: "exists == true AND isHittable == true")
        try require(expect(deviceList, matching: ready, timeout: 5), "保存后未返回可操作设备页")
        try require(expect(deviceRow, matching: ready, timeout: 5), "保存后未显示可操作目标设备")
    }

    private func dismissPasswordAutofillPromptIfPresent(application: XCUIApplication) throws {
        let localizedButton = application.buttons["以后"]
        let englishButton = application.buttons["Not Now"]
        if localizedButton.waitForExistence(timeout: 2), localizedButton.isHittable {
            localizedButton.tap()
        } else if englishButton.waitForExistence(timeout: 2), englishButton.isHittable {
            englishButton.tap()
        }
    }

    private func typeAndAwaitValue(
        _ field: XCUIElement,
        expected: String,
        fieldName: String
    ) throws {
        try require(field.waitForExistence(timeout: 5), "\(fieldName)输入框未出现")
        field.tap()
        field.typeText(expected)
        let predicate = NSPredicate(format: "value == %@", expected)
        try require(expect(field, matching: predicate, timeout: 5), "\(fieldName)输入值不完整")
    }

    private func pasteAndAwaitSecureValue(
        application: XCUIApplication,
        field: XCUIElement,
        expected: String
    ) throws {
        try require(field.waitForExistence(timeout: 5), "配对 Key 输入框未出现")
        let placeholder = field.value as? String
        field.tap()
        field.press(forDuration: 1.0)
        let localizedPaste = application.menuItems["粘贴"]
        let englishPaste = application.menuItems["Paste"]
        if localizedPaste.waitForExistence(timeout: 2) {
            localizedPaste.tap()
        } else {
            try require(englishPaste.waitForExistence(timeout: 2), "粘贴菜单未出现")
            englishPaste.tap()
        }
        dismissPastePermissionIfPresent(application: application)
        try awaitSecureValue(field, expected: expected, placeholder: placeholder)
    }

    private func dismissPastePermissionIfPresent(application: XCUIApplication) {
        let localizedAllow = application.buttons["允许粘贴"]
        let englishAllow = application.buttons["Allow Paste"]
        if localizedAllow.waitForExistence(timeout: 2), localizedAllow.isHittable {
            localizedAllow.tap()
        } else if englishAllow.waitForExistence(timeout: 2), englishAllow.isHittable {
            englishAllow.tap()
        }
    }

    private func awaitSecureValue(_ field: XCUIElement, expected: String, placeholder: String?) throws {
        let predicate = NSPredicate { object, _ in
            guard let value = (object as? XCUIElement)?.value as? String else { return false }
            return value == expected || (value != placeholder && value.contains("•"))
        }
        try require(expect(field, matching: predicate, timeout: 5), "配对 Key 输入值无效")
    }

    private func typeAndAwaitSecureValue(
        _ field: XCUIElement,
        expected: String,
        fieldName: String
    ) throws {
        try require(field.waitForExistence(timeout: 5), "\(fieldName)输入框未出现")
        let placeholder = field.value as? String
        field.tap()
        field.typeText(expected)
        try awaitSecureValue(field, expected: expected, placeholder: placeholder)
    }

    private func expect(_ element: XCUIElement, matching predicate: NSPredicate, timeout: TimeInterval) -> Bool {
        let expectation = XCTNSPredicateExpectation(predicate: predicate, object: element)
        return XCTWaiter.wait(for: [expectation], timeout: timeout) == .completed
    }

    private func require(_ condition: Bool, _ message: String) throws {
        guard condition else {
            XCTFail(message)
            throw UIFlowError.stepFailed
        }
    }

    private enum UIFlowError: Error {
        case stepFailed
    }

    private func deviceButton(application: XCUIApplication, name: String) -> XCUIElement {
        application.descendants(matching: .any)["device-row-\(name)"]
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
