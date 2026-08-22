import SwiftUI

@MainActor
final class LauncherApplicationDelegate: NSObject, NSApplicationDelegate {
    weak var controller: DaemonController?

    func applicationShouldTerminateAfterLastWindowClosed(_ sender: NSApplication) -> Bool {
        true
    }

    func applicationShouldTerminate(_ sender: NSApplication) -> NSApplication.TerminateReply {
        guard let controller, controller.showsStopAction else { return .terminateNow }
        Task {
            await controller.stop()
            sender.reply(toApplicationShouldTerminate: true)
        }
        return .terminateLater
    }
}

@main
@MainActor
struct GrokRemoteLauncherApp: App {
    @NSApplicationDelegateAdaptor(LauncherApplicationDelegate.self) private var applicationDelegate
    @StateObject private var settings: LauncherSettings
    @StateObject private var controller: DaemonController

    init() {
        let settings = LauncherSettings()
        _settings = StateObject(wrappedValue: settings)
        _controller = StateObject(wrappedValue: DaemonController(settings: settings))
    }

    var body: some Scene {
        WindowGroup {
            ContentView(controller: controller, settings: settings)
                .frame(minWidth: 920, minHeight: 700)
                .onAppear { applicationDelegate.controller = controller }
        }
        .defaultSize(width: 1080, height: 790)
        .commands {
            CommandGroup(after: .toolbar) {
                Button("刷新服务状态") { Task { await controller.pollHealth() } }
                    .keyboardShortcut("r", modifiers: .command)
            }
        }
    }
}
