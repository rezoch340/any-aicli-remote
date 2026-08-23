import AnyAICLIRemoteCore
import SwiftUI

private enum AppNavigationRoute: Hashable {
    case sessions(UUID)
    case chat(deviceID: UUID, sessionIdentity: SessionIdentity)
}

public struct RootView: View {
    @EnvironmentObject private var store: ChatStore
    @State private var path: [AppNavigationRoute] = []

    public init() {}

    public var body: some View {
        NavigationStack(path: $path) {
            DeviceListView { device in
                Task {
                    if await store.connect(to: device.id) {
                        path = [.sessions(device.id)]
                    }
                }
            }
            .navigationDestination(for: AppNavigationRoute.self) { route in
                switch route {
                case .sessions(let deviceID):
                    if store.activeDeviceID == deviceID, store.connection == .connected {
                        SessionListView { session in
                            path.append(.chat(deviceID: deviceID, sessionIdentity: session.id))
                        }
                    } else {
                        ContentUnavailableView("设备已离线", systemImage: "wifi.slash")
                    }
                case .chat(let deviceID, let sessionIdentity):
                    if store.activeDeviceID == deviceID,
                       let session = store.sessions.first(where: { $0.id == sessionIdentity }) ??
                        (store.selectedSession?.id == sessionIdentity ? store.selectedSession : nil) {
                        ChatView(session: session)
                    } else {
                        ContentUnavailableView("会话不可用", systemImage: "bubble.left.and.exclamationmark.bubble.right")
                    }
                }
            }
        }
        .onChange(of: store.navigationResetToken) { _, _ in
            path.removeAll()
        }
        .onChange(of: path) { oldPath, newPath in
            if !oldPath.isEmpty, newPath.isEmpty, store.activeDeviceID != nil {
                store.disconnect()
            }
        }
    }
}
