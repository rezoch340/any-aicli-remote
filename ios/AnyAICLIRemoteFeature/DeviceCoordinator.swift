import AnyAICLIRemoteCore
import Foundation

/// 已保存设备的读写、健康探测与配对深链导入。
@MainActor
final class DeviceCoordinator {
  private unowned let store: ChatStore
  private let healthProbe: DeviceHealthProbe
  private let deviceRepository: DeviceProfileRepository
  private var healthProbeGeneration = UUID()

  init(store: ChatStore, healthProbe: DeviceHealthProbe, deviceRepository: DeviceProfileRepository) {
    self.store = store
    self.healthProbe = healthProbe
    self.deviceRepository = deviceRepository
  }

  func pairingKey(for deviceID: UUID) throws -> String {
    try deviceRepository.pairingKey(for: deviceID)
  }

  @discardableResult
  func saveDevice(id: UUID? = nil, name: String, address: String, pairingKey: String) throws -> UUID {
    let result = try deviceRepository.save(
      id: id, name: name, address: address, pairingKey: pairingKey, devices: store.devices)
    store.devices = result.devices
    store.deviceHealthStatuses[result.deviceID] = .checking
    store.deviceMessage = "已保存 \(result.device.name)"
    store.deviceMessageIsError = false
    return result.deviceID
  }

  func deleteDevice(_ deviceID: UUID) throws {
    guard let device = store.devices.first(where: { $0.id == deviceID }) else { return }
    store.devices = try deviceRepository.delete(deviceID: deviceID, devices: store.devices)
    store.deviceHealthStatuses.removeValue(forKey: deviceID)
    if store.activeDeviceID == deviceID { store.disconnect() }
    store.deviceMessage = "已删除 \(device.name)"
    store.deviceMessageIsError = false
  }

  func reportDeviceError(_ error: Error) {
    store.deviceMessage = error.localizedDescription
    store.deviceMessageIsError = true
  }

  func deviceHealthStatus(for deviceID: UUID) -> DeviceHealthStatus {
    store.deviceHealthStatuses[deviceID] ?? .checking
  }

  func refreshDeviceHealth() async {
    let devicesSnapshot = store.devices
    let refreshGeneration = UUID()
    healthProbeGeneration = refreshGeneration
    let currentDeviceIDs = Set(devicesSnapshot.map(\.id))
    store.deviceHealthStatuses = store.deviceHealthStatuses.filter { currentDeviceIDs.contains($0.key) }
    for device in devicesSnapshot {
      store.deviceHealthStatuses[device.id] = .checking
    }

    let monitor = DeviceHealthMonitor(healthProbe: healthProbe)
    let statuses = await monitor.probe(devices: devicesSnapshot)
    guard healthProbeGeneration == refreshGeneration else { return }
    for (deviceID, isOnline) in statuses where store.devices.contains(where: { $0.id == deviceID }) {
      store.deviceHealthStatuses[deviceID] = isOnline ? .online : .offline
    }
  }

  @discardableResult
  func importPairingDeepLink(_ pairingDeepLink: URL) -> Bool {
    do {
      let parsedLink = try PairingDeepLink.parse(pairingDeepLink)
      let existingDevice = store.devices.first { $0.baseURL == parsedLink.profile.baseURL }
      let name =
        parsedLink.name ?? existingDevice?.name ?? parsedLink.profile.baseURL.host
        ?? ProductIdentifiers.displayName
      _ = try saveDevice(
        id: existingDevice?.id, name: name, address: parsedLink.serviceAddress,
        pairingKey: parsedLink.profile.key)
      store.disconnect()
      store.deviceMessage = existingDevice == nil ? "设备已添加，请点击设备连接" : "设备已更新，请点击设备连接"
      store.deviceMessageIsError = false
      return true
    } catch {
      reportDeviceError(error)
      return false
    }
  }
}
