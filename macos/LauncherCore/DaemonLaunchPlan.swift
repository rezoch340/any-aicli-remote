import Foundation

struct DaemonLaunchPlan: Equatable {
  let arguments: [String]

  init(configurationURL: URL, secretFileURL: URL) {
    arguments = ["--config", configurationURL.path, "--pairing-secret-file", secretFileURL.path]
  }
}
