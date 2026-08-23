import AnyAICLIRemoteCore
import Foundation

/// 设备连接的建立、主动断开与掉线处理。
@MainActor
final class ConnectionCoordinator {
  private unowned let store: ChatStore
  private let ownership: ChatOwnership

  init(store: ChatStore, ownership: ChatOwnership) {
    self.store = store
    self.ownership = ownership
  }

  func connect(to deviceID: UUID) async -> Bool {
    guard let device = store.devices.first(where: { $0.id == deviceID }) else { return false }
    if store.activeDeviceID == deviceID, store.connection == .connected { return true }
    let attemptID = ownership.advanceConnection()
    store.client.disconnect(notify: false)
    ownership.clearSessionState()
    store.activeDeviceID = deviceID
    store.connection = .connecting
    store.deviceMessage = "正在连接 \(device.name)"
    store.deviceMessageIsError = false
    do {
      let profile = ServerProfile(
        baseURL: device.baseURL, key: try store.pairingKey(for: deviceID))
      guard !profile.key.isEmpty else { throw ClientError.missingKey }
      let initialize = try await store.client.connect(profile: profile)
      guard ownership.ownsConnectionAttempt(attemptID, deviceID: deviceID) else { return false }
      let refreshedSessions = try await store.sessionCoordinator.fetchSessions()
      guard ownership.ownsConnectionAttempt(attemptID, deviceID: deviceID), store.client.isConnected
      else {
        return false
      }
      store.modelState = SessionPayloadMapper.modelState(from: initialize, current: store.modelState)
      store.sessions = refreshedSessions
      store.connection = .connected
      store.statusMessage = "已连接"
      store.deviceMessage = ""
      store.deviceMessageIsError = false
      return true
    } catch {
      guard ownership.ownsConnectionAttempt(attemptID, deviceID: deviceID) else { return false }
      store.client.disconnect(notify: false)
      store.activeDeviceID = nil
      store.connection = .failed(error.localizedDescription)
      store.statusMessage = error.localizedDescription
      store.deviceMessage = "连接失败：\(error.localizedDescription)"
      store.deviceMessageIsError = true
      store.navigationResetToken = UUID()
      return false
    }
  }

  func disconnect() {
    ownership.advanceConnection()
    store.client.disconnect(notify: false)
    store.activeDeviceID = nil
    ownership.clearSessionState()
    store.connection = .disconnected
    store.statusMessage = ""
    store.navigationResetToken = UUID()
  }

  func handleDisconnect(_ error: Error?) {
    guard store.activeDeviceID != nil else { return }
    ownership.advanceConnection()
    store.client.disconnect(notify: false)
    store.activeDeviceID = nil
    ownership.clearSessionState()
    let message = error?.localizedDescription ?? "连接中断"
    store.connection = .failed(message)
    store.statusMessage = message
    store.deviceMessage = "设备连接已断开"
    store.deviceMessageIsError = true
    store.navigationResetToken = UUID()
  }
}
