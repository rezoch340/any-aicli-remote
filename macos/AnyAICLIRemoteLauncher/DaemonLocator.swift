import Foundation

enum DaemonLocator {
  static func locate(policy: LauncherPolicy, fileManager: FileManager = .default) -> URL? {
    var candidates = [URL]()
    if let bundled = Bundle.main.url(
      forResource: ProductIdentifier.daemonExecutableName, withExtension: nil)
    {
      candidates.append(bundled)
    }
    candidates.append(
      Bundle.main.bundleURL.appendingPathComponent(
        "Contents/MacOS/\(ProductIdentifier.daemonExecutableName)"))
    var cursor = URL(fileURLWithPath: fileManager.currentDirectoryPath, isDirectory: true)
    for _ in 0..<policy.daemonSearchParentDepth {
      candidates.append(
        cursor.appendingPathComponent("dist/\(ProductIdentifier.daemonExecutableName)"))
      let parent = cursor.deletingLastPathComponent()
      if parent == cursor { break }
      cursor = parent
    }
    var visited = Set<String>()
    return candidates.first {
      visited.insert($0.standardizedFileURL.path).inserted
        && fileManager.isExecutableFile(atPath: $0.path)
    }
  }

  static func launchEnvironment(
    policy: LauncherPolicy,
    environment: [String: String] = DaemonLaunchEnvironment.inheritedSanitized(),
    homeDirectory: URL = FileManager.default.homeDirectoryForCurrentUser
  ) -> [String: String] {
    var result = DaemonLaunchEnvironment.inheritedSanitized(environment)
    var paths = [String]()
    var seen = Set<String>()
    let configuredPaths = policy.executableSearchPaths
    let inheritedPaths = (environment["PATH"] ?? "").split(separator: ":").map(String.init)
    for path in configuredPaths + inheritedPaths {
      let expandedPath = expand(path: path, homeDirectory: homeDirectory)
      guard !expandedPath.isEmpty, seen.insert(expandedPath).inserted else { continue }
      paths.append(expandedPath)
    }
    result["PATH"] = paths.joined(separator: ":")
    return result
  }

  private static func expand(path: String, homeDirectory: URL) -> String {
    let homePath = homeDirectory.path
    if path == "~" { return homePath }
    if path.hasPrefix("~/") { return homePath + String(path.dropFirst()) }
    return path
  }
}
