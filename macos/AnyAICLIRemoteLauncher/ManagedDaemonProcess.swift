import Foundation

final class ManagedDaemonProcess {
  let generation: UInt64
  private let process: Process
  private let pipe: Pipe

  init(
    generation: UInt64, executableURL: URL, plan: DaemonLaunchPlan, environment: [String: String],
    onLog: @escaping @Sendable (UInt64, String) -> Void,
    onTermination: @escaping @Sendable (UInt64, Int32) -> Void
  ) throws {
    self.generation = generation
    process = Process()
    pipe = Pipe()
    process.executableURL = executableURL
    process.arguments = plan.arguments
    process.currentDirectoryURL = FileManager.default.homeDirectoryForCurrentUser
    process.environment = environment
    process.standardInput = FileHandle.nullDevice
    process.standardOutput = pipe
    process.standardError = pipe
    pipe.fileHandleForReading.readabilityHandler = { handle in
      let data = handle.availableData
      if data.isEmpty {
        handle.readabilityHandler = nil
        return
      }
      onLog(generation, String(decoding: data, as: UTF8.self))
    }
    process.terminationHandler = { terminated in
      onTermination(generation, terminated.terminationStatus)
    }
    try process.run()
  }

  var isRunning: Bool { process.isRunning }
  func interrupt() { process.interrupt() }
  func terminate() { process.terminate() }
  func close() { pipe.fileHandleForReading.readabilityHandler = nil }
}
