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
        composer.typeText("不要调用工具。输出一段不超过十行的富文本，包含一个标题、一句粗体和三项列表；最后单独输出 RICHSHORTENDMARKER。")
        application.buttons["发送"].tap()
        XCTAssertTrue(application.staticTexts["等待助手"].waitForExistence(timeout: 10))
        XCTAssertTrue(application.buttons["停止生成"].waitForExistence(timeout: 5))
        attachScreenshot(named: "streaming-started")
        let chatStatus = application.descendants(matching: .any)["chat-status"]
        try awaitStreamingSnapshots(application: application, status: chatStatus)
        let completedPredicate = NSPredicate(format: "label == %@ OR value == %@", "完成", "完成")
        let completedExpectation = XCTNSPredicateExpectation(predicate: completedPredicate, object: chatStatus)
        wait(for: [completedExpectation], timeout: 120)
        let markerPredicate = NSPredicate(
            format: "identifier == %@ AND value CONTAINS %@", "assistant-message", "RICHSHORTENDMARKER"
        )
        let markerText = application.descendants(matching: .any).matching(markerPredicate).firstMatch
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
        let reopenedMarker = application.descendants(matching: .any).matching(markerPredicate).firstMatch
        XCTAssertTrue(reopenedMarker.waitForExistence(timeout: 10))
        assertAssistantAtComposer(application: application)
        attachScreenshot(named: "session-reopened-at-bottom")
    }

    func testChildAgentCardsAgainstLiveDaemon() throws {
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
        try require(deviceRow.waitForExistence(timeout: 10), "未找到模拟器服务")
        deviceRow.tap()
        try require(application.navigationBars["模拟器服务"].waitForExistence(timeout: 10), "未进入设备")
        application.buttons["新建会话"].tap()
        try require(application.navigationBars["新建会话"].waitForExistence(timeout: 5), "新建会话页未出现")
        application.buttons["创建"].tap()

        let composer = application.descendants(matching: .any)["chat-composer"]
        try require(composer.waitForExistence(timeout: 10), "聊天输入框未出现")
        composer.tap()
        composer.typeText("请同时启动两个 explore 子 Agent，分别完成两个独立的小型代码库探索任务；必须等待两个子 Agent 都结束后，再回复唯一标记 CHILDAGENTE2EOK。")
        application.buttons["发送"].tap()
        let strip = application.descendants(matching: .any)["child-agent-strip"]
        try require(strip.waitForExistence(timeout: 30), "子 Agent 区域未出现")
        try awaitChildAgentCardCount(application: application, minimum: 2, timeout: 60)
        try awaitTerminalChildAgentCard(application: application, timeout: 180)
        attachScreenshot(named: "child-agents-live")

        let chatStatus = application.descendants(matching: .any)["chat-status"]
        let completed = NSPredicate(format: "label == %@ OR value == %@", "完成", "完成")
        try require(expect(chatStatus, matching: completed, timeout: 180), "聊天未完成")
        let markerPredicate = NSPredicate(
            format: "label CONTAINS %@ OR value CONTAINS %@", "CHILDAGENTE2EOK", "CHILDAGENTE2EOK"
        )
        let marker = application.descendants(matching: .any).matching(markerPredicate).firstMatch
        try require(marker.waitForExistence(timeout: 30), "未找到 CHILDAGENTE2EOK 标记")

        application.terminate()
        application.launchArguments.removeAll { $0 == resetArgument }
        application.launch()
        let reopenedDevice = deviceButton(application: application, name: "模拟器服务")
        try require(reopenedDevice.waitForExistence(timeout: 10), "重启后未找到设备")
        reopenedDevice.tap()
        try require(application.navigationBars["模拟器服务"].waitForExistence(timeout: 10), "重启后未进入设备")
        try require(application.cells.firstMatch.waitForExistence(timeout: 10), "未找到历史会话")
        application.cells.firstMatch.tap()
        try require(composer.waitForExistence(timeout: 10), "历史会话输入框未出现")
        try awaitChildAgentCardCount(application: application, minimum: 2, timeout: 60)
        attachScreenshot(named: "child-agents-history")
    }

    func testStructuredAskInteractionAgainstLiveDaemon() throws {
        guard ProcessInfo.processInfo.environment["ANY_AI_CLI_REMOTE_LIVE_UI_TEST"] == "1" else {
            throw XCTSkip("Live daemon UI test is opt-in")
        }

        let application = try openNewLiveChat()
        let composer = application.descendants(matching: .any)["chat-composer"]
        composer.tap()
        let askPrompt = "必须调用 ask_user_question，只问颜色，选项必须按“蓝色、绿色”的顺序提供。"
            + "收到回答后，只有在既看到选择“蓝色”又看到 user notes 精确包含“移动端自定义答案”时，"
            + "才只输出 ASKCUSTOMANSWEROK，否则输出不同失败标记。"
        composer.typeText(askPrompt)
        application.buttons["发送"].tap()

        let askSheet = application.descendants(matching: .any)["interaction-ask-sheet"]
        try require(askSheet.waitForExistence(timeout: 60), "未出现结构化提问面板")
        try require(
            application.descendants(matching: .any)["interaction-cancel"].exists,
            "default ask 模式未找到取消按钮"
        )
        try require(
            !application.descendants(matching: .any)["interaction-chat-about"].exists,
            "default ask 模式不应显示先聊一下"
        )
        try require(
            !application.descendants(matching: .any)["interaction-skip"].exists,
            "default ask 模式不应显示跳过"
        )
        let question = application.descendants(matching: .any)["interaction-question-0"]
        let blueOption = application.descendants(matching: .any)["interaction-option-0-0"]
        let greenOption = application.descendants(matching: .any)["interaction-option-0-1"]
        try require(question.exists, "未找到第一个问题")
        try require(blueOption.exists, "未找到蓝色选项")
        try require(greenOption.exists, "未找到绿色选项")
        let customAnswer = application.descendants(matching: .any)["interaction-custom-answer-0"]
        try typeAndAwaitValue(customAnswer, expected: "移动端自定义答案", fieldName: "自定义答案")
        blueOption.tap()
        try require(
            waitForAccessibilityValue(blueOption, expected: "selected", timeout: 5),
            "点击蓝色后选项未进入 selected 状态"
        )
        attachScreenshot(named: "ask-interaction-filled")
        application.descendants(matching: .any)["interaction-submit"].tap()
        try require(askSheet.waitForNonExistence(timeout: 10), "提交后结构化提问面板未关闭")
        try awaitAssistantMarker(application: application, marker: "ASKCUSTOMANSWEROK", timeout: 120)
    }

    private func waitForAccessibilityValue(
        _ element: XCUIElement,
        expected: String,
        timeout: TimeInterval
    ) -> Bool {
        let predicate = NSPredicate(format: "value == %@", expected)
        return expect(element, matching: predicate, timeout: timeout)
    }

    func testPlanApprovalStartsAtTopAgainstLiveDaemon() throws {
        guard ProcessInfo.processInfo.environment["ANY_AI_CLI_REMOTE_LIVE_UI_TEST"] == "1" else {
            throw XCTSkip("Live daemon UI test is opt-in")
        }

        let application = try openNewLiveChat()
        let composer = application.descendants(matching: .any)["chat-composer"]
        composer.tap()
        composer.typeText("必须先调用 enter_plan_mode，生成至少30行编号的只读计划，然后调用 exit_plan_mode。收到修改意见后，第二版必须包含唯一文本 PLANREVISIONTWO 并再次 exit_plan_mode；不要实施计划。")
        application.buttons["发送"].tap()

        let planSheet = application.descendants(matching: .any)["interaction-plan-sheet"]
        try require(planSheet.waitForExistence(timeout: 90), "未出现计划批准面板")
        let planTitle = application.staticTexts["计划待批准"]
        let planContent = application.descendants(matching: .any)["interaction-plan-content"]
        try require(planTitle.exists, "未找到计划标题")
        try require(planContent.exists, "未找到计划内容")
        assertPlanStartsAtTop(application: application, title: planTitle, content: planContent)
        attachScreenshot(named: "plan-interaction-first-version")

        let feedback = application.descendants(matching: .any)["interaction-plan-feedback"]
        try typeAndAwaitValue(feedback, expected: "第二版必须包含 PLANREVISIONTWO", fieldName: "计划修改意见")
        application.descendants(matching: .any)["interaction-plan-revise"].tap()
        try awaitInteractionMarker(
            application: application,
            identifier: "interaction-plan-content",
            marker: "PLANREVISIONTWO",
            timeout: 120
        )
        assertPlanStartsAtTop(application: application, title: planTitle, content: planContent)
        attachScreenshot(named: "plan-interaction-second-version")
        application.descendants(matching: .any)["interaction-plan-abandon"].tap()
        try require(planSheet.waitForNonExistence(timeout: 10), "放弃后计划批准面板未关闭")
    }

    private func openNewLiveChat() throws -> XCUIApplication {
        let application = launchApplication()
        try addDevice(
            application: application,
            name: "模拟器服务",
            address: "http://127.0.0.1:2421",
            pairingKeyInput: .file(path: try livePairingKeyFilePath())
        )
        let deviceRow = deviceButton(application: application, name: "模拟器服务")
        try require(deviceRow.waitForExistence(timeout: 10), "未找到模拟器服务")
        deviceRow.tap()
        try require(application.navigationBars["模拟器服务"].waitForExistence(timeout: 10), "未进入设备")
        application.buttons["新建会话"].tap()
        try require(application.navigationBars["新建会话"].waitForExistence(timeout: 5), "新建会话页未出现")
        application.buttons["创建"].tap()
        try require(application.descendants(matching: .any)["chat-composer"].waitForExistence(timeout: 10), "聊天输入框未出现")
        return application
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

    private func awaitChildAgentCardCount(application: XCUIApplication, minimum: Int, timeout: TimeInterval) throws {
        let deadline = Date().addingTimeInterval(timeout)
        let cards = application.descendants(matching: .any).matching(NSPredicate(format: "identifier BEGINSWITH %@", "child-agent-card-"))
        while Date() < deadline {
            if cards.count >= minimum { return }
            RunLoop.current.run(until: Date().addingTimeInterval(0.25))
        }
        throw UIFlowError.message("子 Agent 卡片数量不足：期望至少 \(minimum)，实际 \(cards.count)")
    }

    private func awaitTerminalChildAgentCard(application: XCUIApplication, timeout: TimeInterval) throws {
        let deadline = Date().addingTimeInterval(timeout)
        let terminalValues = Set(["已完成", "失败", "已取消"])
        let cards = application.descendants(matching: .any).matching(NSPredicate(format: "identifier BEGINSWITH %@", "child-agent-card-"))
        while Date() < deadline {
            for index in 0..<cards.count {
                let value = cards.element(boundBy: index).value as? String
                if let value, terminalValues.contains(value) { return }
            }
            RunLoop.current.run(until: Date().addingTimeInterval(0.25))
        }
        throw UIFlowError.message("未等到子 Agent 卡片进入终态（已完成/失败/已取消）")
    }

    private func dismissPasswordAutofillPromptIfPresent(application: XCUIApplication) throws {
        let localizedButton = application.buttons["以后"]
        let englishButton = application.buttons["Not Now"]
        let deadline = Date().addingTimeInterval(5)
        var tappedPrompt = false
        while Date() < deadline {
            let candidate: XCUIElement?
            if localizedButton.exists && localizedButton.isHittable {
                candidate = localizedButton
            } else if englishButton.exists && englishButton.isHittable {
                candidate = englishButton
            } else {
                candidate = nil
            }
            if let candidate {
                tappedPrompt = true
                candidate.tap()
                RunLoop.current.run(until: Date().addingTimeInterval(0.25))
                if !localizedButton.exists && !englishButton.exists { return }
            } else {
                RunLoop.current.run(until: Date().addingTimeInterval(0.25))
            }
        }
        if tappedPrompt && (localizedButton.exists || englishButton.exists) {
            throw UIFlowError.message("保存密码提示未消失")
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

    private func awaitAssistantMarker(
        application: XCUIApplication,
        marker: String,
        timeout: TimeInterval
    ) throws {
        try awaitInteractionMarker(
            application: application,
            identifier: "assistant-message",
            marker: marker,
            timeout: timeout
        )
    }

    private func awaitInteractionMarker(
        application: XCUIApplication,
        identifier: String,
        marker: String,
        timeout: TimeInterval
    ) throws {
        let predicate = NSPredicate(
            format: "identifier == %@ AND (label CONTAINS %@ OR value CONTAINS %@)",
            identifier,
            marker,
            marker
        )
        let element = application.descendants(matching: .any).matching(predicate).firstMatch
        try require(element.waitForExistence(timeout: timeout), "未找到标记 \(marker)")
    }

    private func assertPlanStartsAtTop(
        application: XCUIApplication,
        title: XCUIElement,
        content: XCUIElement
    ) {
        let window = application.windows.firstMatch
        XCTAssertTrue(window.waitForExistence(timeout: 5))
        let topBoundary = window.frame.minY + window.frame.height * 0.45
        XCTAssertLessThan(title.frame.minY, topBoundary)
        XCTAssertLessThan(content.frame.minY, topBoundary)
    }

    private func require(_ condition: Bool, _ message: String) throws {
        guard condition else {
            XCTFail(message)
            throw UIFlowError.stepFailed
        }
    }

    private enum UIFlowError: Error {
        case stepFailed
        case message(String)
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

    private func awaitStreamingSnapshots(application: XCUIApplication, status: XCUIElement) throws {
        let messages = application.descendants(matching: .any).matching(identifier: "assistant-message")
        let completed = NSPredicate(format: "label == %@ OR value == %@", "完成", "完成")
        var lengths: [Int] = []
        let deadline = Date().addingTimeInterval(90)
        while Date() < deadline {
            if completed.evaluate(with: status) {
                XCTFail("流式尚未观察到三个增长快照就已完成")
                throw UIFlowError.stepFailed
            }
            if messages.count > 0 {
                let value = messages.element(boundBy: messages.count - 1).value as? String ?? ""
                if !value.isEmpty, value.count > (lengths.last ?? 0) { lengths.append(value.count) }
                if lengths.count >= 3 && lengths[0] < lengths[1] && lengths[1] < lengths[2] {
                    attachScreenshot(named: "streaming-intermediate")
                    return
                }
            }
            RunLoop.current.run(until: Date().addingTimeInterval(0.1))
        }
        XCTFail("90秒内未观察到三个严格增长的流式文本快照：\(lengths)")
        throw UIFlowError.stepFailed
    }

    private func attachScreenshot(named name: String) {
        let attachment = XCTAttachment(screenshot: XCUIScreen.main.screenshot())
        attachment.name = name
        attachment.lifetime = .keepAlways
        add(attachment)
    }
}
