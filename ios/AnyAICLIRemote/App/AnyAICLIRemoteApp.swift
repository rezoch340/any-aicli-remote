import SwiftUI

@main
struct AnyAICLIRemoteApp: App {
    @StateObject private var store = ChatStore()

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
