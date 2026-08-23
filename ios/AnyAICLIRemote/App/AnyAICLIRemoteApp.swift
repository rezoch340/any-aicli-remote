import SwiftUI

@main
struct AnyAICLIRemoteApp: App {
    #if DEBUG
    private static let uiTestingResetArgument = "--ui-testing-reset-storage"
    #endif
    @StateObject private var store: ChatStore

    init() {
        #if DEBUG
        if ProcessInfo.processInfo.arguments.contains(Self.uiTestingResetArgument) {
            ChatStore.resetStorageForUITesting()
        }
        #endif
        _store = StateObject(wrappedValue: ChatStore())
    }

    var body: some Scene {
        WindowGroup {
            RootView()
                .environmentObject(store)
                .preferredColorScheme(.dark)
                .onOpenURL { pairingDeepLink in
                    store.importPairingDeepLink(pairingDeepLink)
                }
        }
    }
}
