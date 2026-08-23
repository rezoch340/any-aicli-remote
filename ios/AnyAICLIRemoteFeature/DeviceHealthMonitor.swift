import AnyAICLIRemoteCore
import Foundation

struct DeviceHealthMonitor {
  let healthProbe: DeviceHealthProbe

  func probe(devices: [SavedDevice]) async -> [UUID: Bool] {
    await withTaskGroup(of: (UUID, Bool).self, returning: [UUID: Bool].self) { group in
      for device in devices {
        group.addTask {
          let isOnline = await healthProbe.isOnline(baseURL: device.baseURL)
          return (device.id, isOnline)
        }
      }

      var statuses: [UUID: Bool] = [:]
      for await (deviceID, isOnline) in group {
        statuses[deviceID] = isOnline
      }
      return statuses
    }
  }
}
