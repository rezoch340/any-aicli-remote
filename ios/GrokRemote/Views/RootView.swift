import SwiftUI

struct RootView: View {
    @EnvironmentObject private var store: ChatStore
    @State private var path: [String] = []

    var body: some View {
        Group {
            if store.connection == .connected || store.connection == .reconnecting {
                NavigationStack(path: $path) {
                    SessionListView(path: $path)
                        .navigationDestination(for: String.self) { sessionID in
                            if let session = store.sessions.first(where: { $0.id == sessionID }) ??
                                (store.selectedSession?.id == sessionID ? store.selectedSession : nil) {
                                ChatView(session: session)
                            } else {
                                ContentUnavailableView("会话不存在", systemImage: "bubble.left.and.exclamationmark.bubble.right")
                            }
                        }
                }
            } else {
                PairingView()
            }
        }
        .task {
            if store.hasSavedProfile && store.connection == .disconnected {
                await store.connect()
            }
        }
    }
}
