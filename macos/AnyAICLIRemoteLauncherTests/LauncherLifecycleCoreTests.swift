import XCTest

enum LauncherLifecycleTestError: Error {
  case invalidPolicyFixture
}

final class LauncherLifecycleCoreTests: XCTestCase {
  func testLaunchArgumentsAreExact() {
    let plan = DaemonLaunchPlan(
      configurationURL: URL(fileURLWithPath: "/config"),
      secretFileURL: URL(fileURLWithPath: "/secret"))
    XCTAssertEqual(plan.arguments, ["--config", "/config", "--pairing-secret-file", "/secret"])
    XCTAssertFalse(plan.arguments.contains("--bind"))
    XCTAssertFalse(plan.arguments.contains("--port"))
    XCTAssertFalse(plan.arguments.contains("--agent-port"))
    XCTAssertFalse(plan.arguments.contains("--public-host"))
    XCTAssertFalse(plan.arguments.contains("--stop-agent-on-exit"))
  }
  func testPolicyResource() throws {
    let url = URL(fileURLWithPath: #filePath).deletingLastPathComponent()
      .deletingLastPathComponent().appendingPathComponent("Resources/LauncherPolicy.json")
    let policy = try LauncherPolicy.load(resourceURL: url)
    XCTAssertEqual(policy.schemaVersion, 1)
    XCTAssertEqual(policy.maximumLogEntries, 1000)
  }
  func testPolicyRejectsZeroValues() throws {
    for key in [
      "request_timeout_seconds", "resource_timeout_seconds", "health_poll_interval_seconds",
      "stop_poll_interval_seconds", "stop_poll_attempts", "interrupt_grace_seconds",
      "maximum_log_chunk_characters", "maximum_log_entries", "daemon_search_parent_depth",
    ] {
      var object = try policyObject()
      object[key] = 0
      XCTAssertThrowsError(try decode(object)) { error in
        XCTAssertEqual(error as? LauncherPolicy.PolicyError, .invalid(key))
      }
    }
  }

  func testPolicyRejectsNonFiniteDouble() throws {
    for key in [
      "request_timeout_seconds", "resource_timeout_seconds", "health_poll_interval_seconds",
      "stop_poll_interval_seconds", "interrupt_grace_seconds",
    ] {
      var object = try policyObject()
      object[key] = "not-a-number"
      XCTAssertThrowsError(try decode(object)) { error in
        XCTAssertEqual(error as? LauncherPolicy.PolicyError, .invalid(key))
      }
    }
  }

  func testPolicyRejectsSchemaAndPaths() throws {
    var object = try policyObject()
    object["schema_version"] = 2
    XCTAssertThrowsError(try decode(object)) { error in
      XCTAssertEqual(error as? LauncherPolicy.PolicyError, .invalid("schema_version"))
    }
    object["schema_version"] = 1
    object["executable_search_paths"] = [" "]
    XCTAssertThrowsError(try decode(object)) { error in
      XCTAssertEqual(error as? LauncherPolicy.PolicyError, .invalid("executable_search_paths"))
    }
  }

  func testPolicyRejectsMissingField() throws {
    var object = try policyObject()
    object.removeValue(forKey: "maximum_log_entries")
    XCTAssertThrowsError(try decode(object))
  }
  func testEndpoints() throws {
    XCTAssertEqual(
      try LocalDaemonEndpoint(bindAddress: "0.0.0.0", port: 1234).url.host,
      "127.0.0.1"
    )
    XCTAssertEqual(try LocalDaemonEndpoint(bindAddress: "", port: 1234).url.host, "127.0.0.1")
    XCTAssertEqual(
      try LocalDaemonEndpoint(bindAddress: "192.0.2.1", port: 1234).url.host,
      "192.0.2.1"
    )
    XCTAssertEqual(try LocalDaemonEndpoint(bindAddress: "::", port: 1234).url.host, "::1")
    XCTAssertEqual(try LocalDaemonEndpoint(bindAddress: "[::]", port: 1234).url.host, "::1")
    let ipv6Endpoint = try LocalDaemonEndpoint(bindAddress: "2001:db8::1", port: 1234)
    XCTAssertEqual(ipv6Endpoint.url.absoluteString, "http://[2001:db8::1]:1234")
  }

  func testEndpointRejectsInvalidPortAndWhitespaceHost() throws {
    for port in [0, 65536] {
      XCTAssertThrowsError(try LocalDaemonEndpoint(bindAddress: "127.0.0.1", port: port))
    }
    XCTAssertThrowsError(try LocalDaemonEndpoint(bindAddress: "   ", port: 1234)) { error in
      guard case .invalidHost = error as? LocalDaemonEndpoint.EndpointError else {
        return XCTFail("expected invalid host")
      }
    }
  }

  func testRuntimeFields() throws {
    let value = try JSONDecoder().decode(
      RuntimeConfiguration.self,
      from: Data(#"{"pairing_url":"u","pairing_deep_link":"d","lan_ip":"i"}"#.utf8))
    XCTAssertEqual(
      value, RuntimeConfiguration(pairingURL: "u", pairingDeepLink: "d", lanAddress: "i"))
  }
  func testHealthFields() throws {
    let value = try JSONDecoder().decode(
      HealthSnapshot.self,
      from: Data(
        #"{"ok":true,"ready":true,"hub_clients":2,"hub_up":true,"hub_err":"","agent_listening":true}"#
          .utf8))
    XCTAssertTrue(value.isHealthy)
    XCTAssertEqual(value.hubClients, 2)
  }
  func testStackStatusFields() throws {
    let value = try JSONDecoder().decode(
      DaemonStackStatus.self,
      from: Data(
        #"{"ok":true,"daemon_port":8765,"agent_port":8766,"self_pid":42,"provider_id":"grok","hub_up":false,"agent_listening":false}"#
          .utf8))
    XCTAssertEqual(
      value,
      DaemonStackStatus(
        ok: true, daemonPort: 8765, agentPort: 8766, selfPID: 42, providerID: "grok",
        hubUp: false, agentListening: false))
  }
  func testHTTPRequests() throws {
    let client = DaemonHTTPClient(
      endpoint: try LocalDaemonEndpoint(bindAddress: "", port: 99), pairingSecret: "secret",
      policy: try policy())
    let health = client.healthRequest()
    XCTAssertEqual(health.httpMethod, "GET")
    XCTAssertEqual(health.url?.path, "/health")
    XCTAssertNil(health.value(forHTTPHeaderField: ProductIdentifier.authenticationHeaderName))
    let config = client.configurationRequest()
    XCTAssertEqual(config.url?.path, "/config.json")
    XCTAssertEqual(
      config.value(forHTTPHeaderField: ProductIdentifier.authenticationHeaderName), "secret")
    let status = client.statusRequest()
    XCTAssertEqual(status.httpMethod, "GET")
    XCTAssertEqual(status.url?.path, "/api/stack/status")
    XCTAssertEqual(
      status.value(forHTTPHeaderField: ProductIdentifier.authenticationHeaderName), "secret")
    let stop = client.stopRequest()
    XCTAssertEqual(stop.httpMethod, "POST")
    XCTAssertEqual(stop.url?.path, "/api/stack/stop")
    XCTAssertEqual(
      stop.value(forHTTPHeaderField: ProductIdentifier.authenticationHeaderName), "secret")
    XCTAssertEqual(stop.value(forHTTPHeaderField: "Content-Type"), "application/json")
    XCTAssertEqual(stop.httpBody, Data("{\"keep_agent\":false}".utf8))
  }
  private func policy() throws -> LauncherPolicy { try decode(try policyObject()) }
  private func policyObject() throws -> [String: Any] {
    let url = URL(fileURLWithPath: #filePath).deletingLastPathComponent()
      .deletingLastPathComponent().appendingPathComponent("Resources/LauncherPolicy.json")
    let value = try JSONSerialization.jsonObject(with: Data(contentsOf: url))
    guard let object = value as? [String: Any] else {
      XCTFail("Launcher policy fixture root must be an object")
      throw LauncherLifecycleTestError.invalidPolicyFixture
    }
    return object
  }
  private func decode(_ object: [String: Any]) throws -> LauncherPolicy {
    try JSONDecoder().decode(
      LauncherPolicy.self, from: JSONSerialization.data(withJSONObject: object))
  }
}
