import Darwin
import Foundation
import XCTest

private enum LauncherRealLifecycleFixture {
  static let enabledEnvironmentName = "ANY_AI_CLI_REMOTE_LAUNCHER_E2E"
  static let daemonEnvironmentName = "ANY_AI_CLI_REMOTE_LAUNCHER_E2E_DAEMON"
  static let pairingSecret = "launcher-e2e-pairing-secret-2026"
  static let operationTimeout = Duration.seconds(10)
  static let pollInterval = Duration.milliseconds(25)
}

private struct LauncherRealLifecycleFailure: Error, CustomStringConvertible {
  let description: String
}

final class LauncherRealLifecycleTests: XCTestCase {
  func testRealStartStopRestartStatusAndPairingPayload() async throws {
    let environment = ProcessInfo.processInfo.environment
    guard environment[LauncherRealLifecycleFixture.enabledEnvironmentName] == "1" else {
      throw XCTSkip("Launcher real lifecycle E2E is disabled")
    }
    guard
      let executablePath = environment[LauncherRealLifecycleFixture.daemonEnvironmentName],
      !executablePath.isEmpty,
      FileManager.default.isExecutableFile(atPath: executablePath)
    else {
      XCTFail("Launcher E2E daemon environment variable must name an executable")
      return
    }

    let fileManager = FileManager.default
    let rootURL = fileManager.temporaryDirectory.appendingPathComponent(
      "launcher-real-lifecycle-" + UUID().uuidString, isDirectory: true)
    try fileManager.createDirectory(
      at: rootURL, withIntermediateDirectories: false, attributes: [.posixPermissions: 0o700])
    defer { try? fileManager.removeItem(at: rootURL) }

    let executableURL = URL(fileURLWithPath: executablePath)
    let configurationURL = rootURL.appendingPathComponent("config.json")
    let dataURL = rootURL.appendingPathComponent("data", isDirectory: true)
    let runtimeURL = rootURL.appendingPathComponent("runtime", isDirectory: true)
    let homeURL = rootURL.appendingPathComponent("home", isDirectory: true)
    let commandRootURL = rootURL.appendingPathComponent("commands", isDirectory: true)
    for directoryURL in [dataURL, runtimeURL, homeURL, commandRootURL] {
      try fileManager.createDirectory(
        at: directoryURL, withIntermediateDirectories: false,
        attributes: [.posixPermissions: 0o700])
    }

    let reservedPorts = try reserveDistinctLoopbackPorts()
    let daemonPort = reservedPorts.daemon
    let agentPort = reservedPorts.agent
    let runner = ProcessDaemonCommandRunner(
      executableURL: executableURL, fileManager: fileManager, temporaryRootURL: commandRootURL)
    try prepareConfiguration(
      runner: runner, configurationURL: configurationURL, daemonPort: daemonPort,
      agentPort: agentPort, dataURL: dataURL, runtimeURL: runtimeURL)

    let policyURL = URL(fileURLWithPath: #filePath).deletingLastPathComponent()
      .deletingLastPathComponent().appendingPathComponent("Resources/LauncherPolicy.json")
    let policy = try LauncherPolicy.load(resourceURL: policyURL)
    let endpoint = try LocalDaemonEndpoint(bindAddress: "127.0.0.1", port: daemonPort)
    let client = DaemonHTTPClient(
      endpoint: endpoint, pairingSecret: LauncherRealLifecycleFixture.pairingSecret,
      policy: policy)
    var launchEnvironment = environment
    launchEnvironment["HOME"] = homeURL.path
    launchEnvironment.removeValue(forKey: LauncherRealLifecycleFixture.enabledEnvironmentName)
    launchEnvironment.removeValue(forKey: LauncherRealLifecycleFixture.daemonEnvironmentName)
    launchEnvironment = DaemonLocator.launchEnvironment(
      policy: policy, environment: launchEnvironment, homeDirectory: homeURL)

    var firstProcessID: Int?
    for generation in UInt64(1)...2 {
      var secretFileURL: URL?
      var managedProcess: ManagedDaemonProcess?
      do {
        let materializedURL = try EphemeralDaemonSecretFile.materialize(
          secret: LauncherRealLifecycleFixture.pairingSecret)
        secretFileURL = materializedURL
        managedProcess = try ManagedDaemonProcess(
          generation: generation,
          executableURL: executableURL,
          plan: DaemonLaunchPlan(
            configurationURL: configurationURL, secretFileURL: materializedURL),
          environment: launchEnvironment,
          onLog: { _, _ in },
          onTermination: { _, _ in })

        try await waitUntilHealthy(client: client)
        try EphemeralDaemonSecretFile.remove(at: materializedURL)
        secretFileURL = nil
        try require(
          !fileManager.fileExists(atPath: materializedURL.path),
          "temporary secret file remained after startup")

        let status = try await client.status()
        try validate(
          status: status, daemonPort: daemonPort, agentPort: agentPort,
          priorProcessID: firstProcessID)
        if firstProcessID == nil { firstProcessID = status.selfPID }

        let configuration = try await client.configuration()
        try validatePairing(
          RuntimePairing(configuration: configuration), daemonPort: daemonPort,
          secret: LauncherRealLifecycleFixture.pairingSecret)

        try await client.stop()
        try await waitUntilStopped(client: client, process: managedProcess!)
        managedProcess?.close()
        managedProcess = nil
        try assertLoopbackPortsAreBindable(daemonPort, agentPort)
      } catch {
        if let secretFileURL { try? EphemeralDaemonSecretFile.remove(at: secretFileURL) }
        await cleanUp(process: managedProcess)
        XCTFail("Launcher real lifecycle E2E failed")
        return
      }
    }
  }

  private func waitUntilHealthy(client: DaemonHTTPClient) async throws {
    let deadline = ContinuousClock.now + LauncherRealLifecycleFixture.operationTimeout
    while ContinuousClock.now < deadline {
      if let health = try? await client.health(), health.isHealthy { return }
      try await Task.sleep(for: LauncherRealLifecycleFixture.pollInterval)
    }
    throw LauncherRealLifecycleFailure(description: "daemon health did not become available")
  }

  private func waitUntilStopped(client: DaemonHTTPClient, process: ManagedDaemonProcess) async throws {
    let deadline = ContinuousClock.now + LauncherRealLifecycleFixture.operationTimeout
    while ContinuousClock.now < deadline {
      let healthUnavailable = (try? await client.health()) == nil
      if healthUnavailable && !process.isRunning { return }
      try await Task.sleep(for: LauncherRealLifecycleFixture.pollInterval)
    }
    throw LauncherRealLifecycleFailure(description: "daemon did not stop")
  }

  private func cleanUp(process: ManagedDaemonProcess?) async {
    guard let process else { return }
    if process.isRunning { process.terminate() }
    let deadline = ContinuousClock.now + LauncherRealLifecycleFixture.operationTimeout
    while process.isRunning && ContinuousClock.now < deadline {
      try? await Task.sleep(for: LauncherRealLifecycleFixture.pollInterval)
    }
    process.close()
  }

  private func validate(
    status: DaemonStackStatus, daemonPort: Int, agentPort: Int, priorProcessID: Int?
  ) throws {
    try require(status.ok, "stack status was not ok")
    try require(status.daemonPort == daemonPort, "stack status daemon port differed")
    try require(status.agentPort == agentPort, "stack status agent port differed")
    try require(status.selfPID > 0, "stack status process ID was invalid")
    try require(status.providerID == "grok", "stack status provider differed")
    try require(!status.hubUp, "provider hub unexpectedly connected")
    try require(!status.agentListening, "provider agent unexpectedly listened")
    if let priorProcessID {
      try require(status.selfPID != priorProcessID, "restart reused the first process ID")
    }
  }

  private func validatePairing(_ pairing: RuntimePairing, daemonPort: Int, secret: String) throws {
    try require(
      !containsWorkspaceText(pairing.httpURL.absoluteString),
      "pairing URL contained workspace state")
    try require(
      !containsWorkspaceText(pairing.deepLinkURL.absoluteString),
      "pairing deep link contained workspace state")
    guard var httpComponents = URLComponents(url: pairing.httpURL, resolvingAgainstBaseURL: false)
    else { throw LauncherRealLifecycleFailure(description: "pairing URL was invalid") }
    try require(httpComponents.port == daemonPort, "pairing URL port differed")
    let httpItems = try uniqueQueryItems(httpComponents.queryItems)
    try require(httpItems.count == 2, "pairing URL query shape differed")
    try require(httpItems["auto"] == "1", "pairing URL auto flag differed")
    try require(httpItems["key"] == secret, "pairing URL key differed")
    try require(!containsWorkspaceField(httpItems), "pairing URL contained workspace state")

    guard let deepComponents = URLComponents(url: pairing.deepLinkURL, resolvingAgainstBaseURL: false)
    else { throw LauncherRealLifecycleFailure(description: "pairing deep link was invalid") }
    try require(deepComponents.scheme == ProductIdentifier.deepLinkScheme, "deep link scheme differed")
    try require(deepComponents.host == ProductIdentifier.deepLinkHost, "deep link host differed")
    let deepItems = try uniqueQueryItems(deepComponents.queryItems)
    try require(deepItems.count == 2, "deep link query shape differed")
    try require(deepItems["key"] == secret, "deep link key differed")
    try require(!containsWorkspaceField(deepItems), "deep link contained workspace state")
    httpComponents.query = nil
    httpComponents.fragment = nil
    try require(
      deepItems["url"] == httpComponents.url?.absoluteString,
      "deep link base URL differed or retained a key")
  }

  private func uniqueQueryItems(_ items: [URLQueryItem]?) throws -> [String: String] {
    var result = [String: String]()
    for item in items ?? [] {
      guard result[item.name] == nil, let value = item.value else {
        throw LauncherRealLifecycleFailure(description: "pairing query contained duplicates")
      }
      result[item.name] = value
    }
    return result
  }

  private func containsWorkspaceField(_ items: [String: String]) -> Bool {
    items.keys.contains(where: containsWorkspaceText)
  }

  private func containsWorkspaceText(_ text: String) -> Bool {
    let normalized = text.lowercased()
    return normalized.contains("cwd") || normalized.contains("workspace")
  }

  private func require(_ condition: @autoclosure () -> Bool, _ message: String) throws {
    guard condition() else { throw LauncherRealLifecycleFailure(description: message) }
  }
}

private func prepareConfiguration(
  runner: ProcessDaemonCommandRunner, configurationURL: URL, daemonPort: Int, agentPort: Int,
  dataURL: URL, runtimeURL: URL
) throws {
  let showResult = try runner.run(
    arguments: ["config", "show", "--config", configurationURL.path], standardInput: nil)
  try requireSuccessfulCommand(showResult, operation: "config show")
  guard var document = try JSONSerialization.jsonObject(with: showResult.standardOutput) as? [String: Any],
    var network = document["network"] as? [String: Any],
    var agent = document["agent"] as? [String: Any],
    var storage = document["storage"] as? [String: Any],
    var provider = document["provider"] as? [String: Any]
  else { throw LauncherRealLifecycleFailure(description: "config show payload shape differed") }
  network["bind"] = "127.0.0.1"
  network["port"] = daemonPort
  network["public_host"] = ""
  agent["host"] = "127.0.0.1"
  agent["port"] = agentPort
  agent["ensure"] = false
  agent["stop_on_exit"] = true
  storage["data_directory"] = dataURL.path
  storage["runtime_directory"] = runtimeURL.path
  provider["id"] = "grok"
  document["network"] = network
  document["agent"] = agent
  document["storage"] = storage
  document["provider"] = provider
  let candidate = try JSONSerialization.data(withJSONObject: document, options: [.sortedKeys])

  let validateResult = try runner.run(
    arguments: ["config", "validate", "--config", configurationURL.path, "--input", "-"],
    standardInput: candidate)
  try requireSuccessfulCommand(validateResult, operation: "config validate")
  let applyResult = try runner.run(
    arguments: ["config", "apply", "--config", configurationURL.path, "--input", "-"],
    standardInput: candidate)
  try requireSuccessfulCommand(applyResult, operation: "config apply")
}

private func requireSuccessfulCommand(_ result: DaemonCommandResult, operation: String) throws {
  guard result.terminationStatus == 0 else {
    throw LauncherRealLifecycleFailure(description: operation + " failed")
  }
}

private func reserveDistinctLoopbackPorts() throws -> (daemon: Int, agent: Int) {
  let daemonSocket = try bindLoopbackSocket(port: 0)
  defer { Darwin.close(daemonSocket.descriptor) }
  let agentSocket = try bindLoopbackSocket(port: 0)
  defer { Darwin.close(agentSocket.descriptor) }
  guard daemonSocket.port != agentSocket.port else {
    throw LauncherRealLifecycleFailure(description: "dynamic ports were not distinct")
  }
  return (daemonSocket.port, agentSocket.port)
}

private func assertLoopbackPortsAreBindable(_ firstPort: Int, _ secondPort: Int) throws {
  let firstSocket = try bindLoopbackSocket(port: firstPort)
  defer { Darwin.close(firstSocket.descriptor) }
  let secondSocket = try bindLoopbackSocket(port: secondPort)
  Darwin.close(secondSocket.descriptor)
}

private func bindLoopbackSocket(port: Int) throws -> (descriptor: Int32, port: Int) {
  let descriptor = Darwin.socket(AF_INET, SOCK_STREAM, 0)
  guard descriptor >= 0 else {
    throw LauncherRealLifecycleFailure(description: "could not create loopback socket")
  }
  var reuseAddress: Int32 = 1
  guard
    Darwin.setsockopt(
      descriptor, SOL_SOCKET, SO_REUSEADDR, &reuseAddress,
      socklen_t(MemoryLayout<Int32>.size)) == 0
  else {
    Darwin.close(descriptor)
    throw LauncherRealLifecycleFailure(description: "could not configure loopback socket")
  }
  var address = sockaddr_in()
  address.sin_len = UInt8(MemoryLayout<sockaddr_in>.size)
  address.sin_family = sa_family_t(AF_INET)
  address.sin_port = in_port_t(port).bigEndian
  address.sin_addr = in_addr(s_addr: inet_addr("127.0.0.1"))
  let bindResult = withUnsafePointer(to: &address) { pointer in
    pointer.withMemoryRebound(to: sockaddr.self, capacity: 1) {
      Darwin.bind(descriptor, $0, socklen_t(MemoryLayout<sockaddr_in>.size))
    }
  }
  guard bindResult == 0 else {
    Darwin.close(descriptor)
    throw LauncherRealLifecycleFailure(description: "could not bind loopback socket")
  }
  var boundAddress = sockaddr_in()
  var boundLength = socklen_t(MemoryLayout<sockaddr_in>.size)
  let nameResult = withUnsafeMutablePointer(to: &boundAddress) { pointer in
    pointer.withMemoryRebound(to: sockaddr.self, capacity: 1) {
      Darwin.getsockname(descriptor, $0, &boundLength)
    }
  }
  guard nameResult == 0 else {
    Darwin.close(descriptor)
    throw LauncherRealLifecycleFailure(description: "could not inspect loopback socket")
  }
  return (descriptor, Int(in_port_t(bigEndian: boundAddress.sin_port)))
}
