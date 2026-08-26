import XCTest

final class DaemonConfigurationTests: XCTestCase {
    func testMissingConfigurationBootstrapsInCanonicalCommandOrder() throws {
        let location = temporaryConfigurationURL()
        let runner = FakeDaemonRunner(responses: [showResult(), successResult(), successResult(), showResult()])
        let store = DaemonConfigurationStore(runner: runner, configurationURL: location, fileManager: .default)
        _ = try store.bootstrapIfMissing(migrating: nil, setAgentStopOnExit: true)
        XCTAssertEqual(runner.calls.map(\.arguments), [
            ["config", "show", "--config", location.path],
            ["config", "validate", "--config", location.path, "--input", "-"],
            ["config", "apply", "--config", location.path, "--input", "-"],
            ["config", "show", "--config", location.path],
        ])
        XCTAssertEqual(runner.calls[1].standardInput, runner.calls[2].standardInput)
        let appliedObject = try JSONSerialization.jsonObject(with: runner.calls[1].standardInput!) as? [String: Any]
        XCTAssertEqual((appliedObject?["agent"] as? [String: Any])?["stop_on_exit"] as? Bool, true)
    }

    func testExistingConfigurationBootstrapRunsShowOnly() throws {
        let location = temporaryConfigurationURL()
        try FileManager.default.createDirectory(at: location.deletingLastPathComponent(), withIntermediateDirectories: true)
        try Data(documentData()).write(to: location)
        let runner = FakeDaemonRunner(responses: [showResult()])
        let store = DaemonConfigurationStore(runner: runner, configurationURL: location)
        _ = try store.bootstrapIfMissing(migrating: nil)
        XCTAssertEqual(runner.calls.map(\.arguments), [["config", "show", "--config", location.path]])
    }

    func testSaveUsesFreshShowValidateApplyShowAndPreservesProviderAndTuning() throws {
        let location = temporaryConfigurationURL()
        let runner = FakeDaemonRunner(responses: [showResult(), successResult(), successResult(), showResult()])
        let store = DaemonConfigurationStore(runner: runner, configurationURL: location)
        _ = try store.save(editable: DaemonEditableConfiguration(bindAddress: "127.0.0.1", daemonPort: 4242, publicHost: "remote.example", agentPort: 4243, providerAlwaysApprove: false))
        XCTAssertEqual(runner.calls.map(\.arguments), [
            ["config", "show", "--config", location.path],
            ["config", "validate", "--config", location.path, "--input", "-"],
            ["config", "apply", "--config", location.path, "--input", "-"],
            ["config", "show", "--config", location.path]
        ])
        XCTAssertEqual(runner.calls[1].standardInput, runner.calls[2].standardInput)
        let object = try JSONSerialization.jsonObject(with: runner.calls[1].standardInput!) as? [String: Any]
        XCTAssertEqual((object?["provider"] as? [String: Any])?["id"] as? String, "grok")
        XCTAssertNotNil(object?["tuning"])
    }

    func testMissingPublicHostDefaultsToEmptyString() throws {
        let data = Data("{\"version\":1,\"network\":{\"bind\":\"0.0.0.0\",\"port\":2421},\"agent\":{\"port\":2419}}".utf8)
        let configuration = try CanonicalDaemonConfiguration(data: data)
        XCTAssertEqual(try configuration.editable.publicHost, "")
    }

    func testProviderAlwaysApproveFalseRoundTrip() throws {
        var configuration = try CanonicalDaemonConfiguration(
            data: providerApprovalDocument(value: "false")
        )
        var editable = try configuration.editable
        XCTAssertFalse(editable.providerAlwaysApprove)

        editable.providerAlwaysApprove = false
        try configuration.apply(editable)

        let roundTripped = try CanonicalDaemonConfiguration(data: configuration.serializedData())
        XCTAssertFalse(try roundTripped.editable.providerAlwaysApprove)
        XCTAssertEqual(try providerOptions(in: configuration)["always-approve"] as? String, "false")
    }

    func testProviderAlwaysApproveTrueRoundTrip() throws {
        var configuration = try CanonicalDaemonConfiguration(
            data: providerApprovalDocument(value: "true")
        )
        var editable = try configuration.editable
        XCTAssertTrue(editable.providerAlwaysApprove)

        editable.providerAlwaysApprove = true
        try configuration.apply(editable)

        let roundTripped = try CanonicalDaemonConfiguration(data: configuration.serializedData())
        XCTAssertTrue(try roundTripped.editable.providerAlwaysApprove)
        XCTAssertEqual(try providerOptions(in: configuration)["always-approve"] as? String, "true")
    }

    func testMissingProviderApprovalConfigurationDefaultsFalse() throws {
        let configurations = [
            documentData(),
            Data("{\"version\":1,\"network\":{\"bind\":\"0.0.0.0\",\"port\":2421},\"agent\":{\"port\":2419},\"provider\":{\"id\":\"grok\",\"options\":{}}}".utf8),
            Data("{\"version\":1,\"network\":{\"bind\":\"0.0.0.0\",\"port\":2421},\"agent\":{\"port\":2419}}".utf8),
        ]

        for data in configurations {
            XCTAssertFalse(try CanonicalDaemonConfiguration(data: data).editable.providerAlwaysApprove)
        }
    }

    func testInvalidProviderAlwaysApproveStringIsRejected() throws {
        let configuration = try CanonicalDaemonConfiguration(
            data: providerApprovalDocument(value: "yes")
        )
        XCTAssertThrowsError(try configuration.editable) { error in
            XCTAssertEqual(
                error as? DaemonConfigurationError,
                .invalidField("provider.options.always-approve")
            )
        }
    }

    func testSavingProviderApprovalPreservesOtherProviderOptionsAndUnrelatedConfiguration() throws {
        var configuration = try CanonicalDaemonConfiguration(
            data: providerApprovalDocument(value: "false")
        )
        var editable = try configuration.editable
        editable.providerAlwaysApprove = true
        try configuration.apply(editable)

        let object = try JSONSerialization.jsonObject(
            with: configuration.serializedData()
        ) as? [String: Any]
        let provider = object?["provider"] as? [String: Any]
        let options = provider?["options"] as? [String: Any]
        XCTAssertEqual(provider?["id"] as? String, "grok")
        XCTAssertEqual(options?["always-approve"] as? String, "true")
        XCTAssertEqual(options?["model"] as? String, "grok-4")
        XCTAssertEqual((object?["tuning"] as? [String: Any])?["custom"] as? String, "keep")
        XCTAssertEqual(object?["unrelated"] as? String, "preserved")
    }

    func testInvalidConfigurationShapesAreRejected() throws {
        XCTAssertThrowsError(try CanonicalDaemonConfiguration(data: Data("[]".utf8))) { error in
            XCTAssertEqual(error as? DaemonConfigurationError, .rootMustBeObject)
        }
        XCTAssertThrowsError(try (try CanonicalDaemonConfiguration(data: Data("{\"network\":{}}".utf8))).editable) { error in
            XCTAssertEqual(error as? DaemonConfigurationError, .missingField("agent"))
        }
        let boolPort = Data("{\"network\":{\"bind\":\"0.0.0.0\",\"port\":true},\"agent\":{\"port\":2419}}".utf8)
        XCTAssertThrowsError(try (try CanonicalDaemonConfiguration(data: boolPort)).editable) { error in
            XCTAssertEqual(error as? DaemonConfigurationError, .invalidField("network.port"))
        }
    }

    func testLaunchEnvironmentRemovesDaemonOverridesButKeepsProviderCredential() {
        let environment = DaemonLaunchEnvironment.inheritedSanitized([
            "ANY_AI_CLI_REMOTE_PORT": "1234", "ANY_AI_CLI_REMOTE_PAIRING_SECRET": "secret",
            "ANY_AI_CLI_REMOTE_GROK_SESSIONS_DIR": "/tmp/sessions", "ANY_AI_CLI_REMOTE_GROK_ALWAYS_APPROVE": "1",
            "ANY_AI_CLI_REMOTE_GROK_LEADER": "1",
            "GROK_REMOTE_CWD": "/tmp/config", "XAI_API_KEY": "provider", "PATH": "/bin", "HOME": "/home/test",
        ])
        XCTAssertNil(environment["ANY_AI_CLI_REMOTE_PORT"])
        XCTAssertNil(environment["ANY_AI_CLI_REMOTE_PAIRING_SECRET"])
        XCTAssertNil(environment["ANY_AI_CLI_REMOTE_GROK_SESSIONS_DIR"])
        XCTAssertNil(environment["ANY_AI_CLI_REMOTE_GROK_ALWAYS_APPROVE"])
        XCTAssertNil(environment["ANY_AI_CLI_REMOTE_GROK_LEADER"])
        XCTAssertNil(environment["GROK_REMOTE_CWD"])
        XCTAssertEqual(environment["XAI_API_KEY"], "provider")
        XCTAssertEqual(environment["PATH"], "/bin")
    }

    func testMigrationOnlyDaemonPortPreservesCanonicalFieldsAndPrefersCurrentDomain() throws {
        let location = temporaryConfigurationURL()
        let suiteName = "migration-" + UUID().uuidString
        let defaults = UserDefaults(suiteName: suiteName)!
        defaults.set(4300, forKey: LauncherPreferenceKeys.daemonPort)
        defaults.set("Stored Device", forKey: LauncherPreferenceKeys.deviceName)
        defaults.set("192.0.2.5", forKey: LauncherPreferenceKeys.lastLANAddress)
        defaults.setPersistentDomain([LauncherPreferenceKeys.daemonPort: 4999], forName: "legacy-" + suiteName)
        let runner = FakeDaemonRunner(responses: [showResult(), successResult(), successResult(), showResult()])
        let store = DaemonConfigurationStore(runner: runner, configurationURL: location)
        _ = try DaemonConfigurationMigration.migrateIfNeeded(store: store, configurationURL: location, defaults: defaults, legacyDomainName: "legacy-" + suiteName)
        let appliedObject = try JSONSerialization.jsonObject(with: runner.calls[2].standardInput!) as? [String: Any]
        let network = appliedObject?["network"] as? [String: Any]
        let agent = appliedObject?["agent"] as? [String: Any]
        XCTAssertEqual(network?["port"] as? Int, 4300)
        XCTAssertEqual(agent?["port"] as? Int, 2419)
        XCTAssertEqual((appliedObject?["provider"] as? [String: Any])?["id"] as? String, "grok")
        XCTAssertNil(defaults.object(forKey: LauncherPreferenceKeys.daemonPort))
        XCTAssertEqual(defaults.integer(forKey: LauncherPreferenceKeys.migrationVersion), DaemonConfigurationMigration.currentVersion)
        XCTAssertEqual(defaults.string(forKey: LauncherPreferenceKeys.deviceName), "Stored Device")
        XCTAssertEqual(defaults.string(forKey: LauncherPreferenceKeys.lastLANAddress), "192.0.2.5")
        defaults.removePersistentDomain(forName: suiteName)
        defaults.removePersistentDomain(forName: "legacy-" + suiteName)
    }

    func testMigrationWithExistingConfigurationShowsOnlyAndCleansServiceKeys() throws {
        let location = temporaryConfigurationURL()
        try FileManager.default.createDirectory(at: location.deletingLastPathComponent(), withIntermediateDirectories: true)
        try Data(documentData()).write(to: location)
        let suiteName = "existing-" + UUID().uuidString
        let defaults = UserDefaults(suiteName: suiteName)!
        defaults.set(4300, forKey: LauncherPreferenceKeys.daemonPort)
        let runner = FakeDaemonRunner(responses: [showResult()])
        let store = DaemonConfigurationStore(runner: runner, configurationURL: location)
        _ = try DaemonConfigurationMigration.migrateIfNeeded(store: store, configurationURL: location, defaults: defaults, legacyDomainName: "legacy-" + suiteName)
        XCTAssertEqual(runner.calls.count, 1)
        XCTAssertNil(defaults.object(forKey: LauncherPreferenceKeys.daemonPort))
        XCTAssertEqual(defaults.integer(forKey: LauncherPreferenceKeys.migrationVersion), DaemonConfigurationMigration.currentVersion)
        defaults.removePersistentDomain(forName: suiteName)
    }

    func testMigrationFailureRetainsPreferencesAndMarker() throws {
        let location = temporaryConfigurationURL()
        let suiteName = "failure-" + UUID().uuidString
        let defaults = UserDefaults(suiteName: suiteName)!
        defaults.set(4300, forKey: LauncherPreferenceKeys.daemonPort)
        let runner = FakeDaemonRunner(responses: [showResult(), failureResult()])
        let store = DaemonConfigurationStore(runner: runner, configurationURL: location)
        XCTAssertThrowsError(try DaemonConfigurationMigration.migrateIfNeeded(store: store, configurationURL: location, defaults: defaults, legacyDomainName: "legacy-" + suiteName))
        XCTAssertEqual(defaults.integer(forKey: LauncherPreferenceKeys.migrationVersion), 0)
        XCTAssertEqual(defaults.integer(forKey: LauncherPreferenceKeys.daemonPort), 4300)
    }

    @MainActor
    func testLauncherSettingsOnlyPersistsDeviceMetadata() {
        let suiteName = "settings-" + UUID().uuidString
        let defaults = UserDefaults(suiteName: suiteName)!
        let settings = LauncherSettings(defaults: defaults)
        settings.daemonPort = 4300
        settings.deviceName = "Test Device"
        settings.lastLANAddress = "192.0.2.1"
        XCTAssertNil(defaults.object(forKey: LauncherPreferenceKeys.daemonPort))
        XCTAssertEqual(defaults.string(forKey: LauncherPreferenceKeys.deviceName), "Test Device")
        XCTAssertEqual(defaults.string(forKey: LauncherPreferenceKeys.lastLANAddress), "192.0.2.1")
        defaults.removePersistentDomain(forName: suiteName)
    }

    private func temporaryConfigurationURL() -> URL {
        URL(fileURLWithPath: NSTemporaryDirectory()).appendingPathComponent(UUID().uuidString).appendingPathComponent("config.json")
    }

    private func documentData() -> Data {
        Data("{\"version\":1,\"network\":{\"bind\":\"0.0.0.0\",\"port\":2421,\"public_host\":\"\"},\"agent\":{\"port\":2419,\"stop_on_exit\":false},\"provider\":{\"id\":\"grok\"},\"tuning\":{\"history\":{\"max_bytes\":100}}}".utf8)
    }

    private func providerApprovalDocument(value: String) -> Data {
        Data("{\"version\":1,\"network\":{\"bind\":\"0.0.0.0\",\"port\":2421,\"public_host\":\"\"},\"agent\":{\"port\":2419},\"provider\":{\"id\":\"grok\",\"options\":{\"always-approve\":\"\(value)\",\"model\":\"grok-4\"}},\"tuning\":{\"custom\":\"keep\"},\"unrelated\":\"preserved\"}".utf8)
    }

    private func providerOptions(in configuration: CanonicalDaemonConfiguration) throws -> [String: Any] {
        let object = try JSONSerialization.jsonObject(with: configuration.serializedData()) as? [String: Any]
        let provider = object?["provider"] as? [String: Any]
        return provider?["options"] as? [String: Any] ?? [:]
    }

    private func showResult() -> DaemonCommandResult { DaemonCommandResult(standardOutput: documentData(), standardError: Data(), terminationStatus: 0) }
    private func successResult() -> DaemonCommandResult { DaemonCommandResult(standardOutput: Data(), standardError: Data(), terminationStatus: 0) }
    private func failureResult() -> DaemonCommandResult { DaemonCommandResult(standardOutput: Data(), standardError: Data("failed".utf8), terminationStatus: 1) }
}

private final class FakeDaemonRunner: DaemonCommandRunning {
    struct Call { let arguments: [String]; let standardInput: Data? }
    var calls: [Call] = []
    private var responses: [DaemonCommandResult]

    init(responses: [DaemonCommandResult]) { self.responses = responses }

    func run(arguments: [String], standardInput: Data?) throws -> DaemonCommandResult {
        calls.append(Call(arguments: arguments, standardInput: standardInput))
        return responses.removeFirst()
    }
}


extension DaemonConfigurationTests {
    func testProductionRunnerCapturesLargeOutputAndCleansPrivateDirectory() throws {
        let fixtureRoot = URL(fileURLWithPath: NSTemporaryDirectory()).appendingPathComponent(UUID().uuidString)
        try FileManager.default.createDirectory(at: fixtureRoot, withIntermediateDirectories: true, attributes: [.posixPermissions: 0o700])
        defer { try? FileManager.default.removeItem(at: fixtureRoot) }
        let fixtureURL = fixtureRoot.appendingPathComponent("output-fixture.sh")
        let fixtureScript = "#!/bin/sh\ni=0\nwhile [ $i -lt 262144 ]; do printf x; i=$((i + 1)); done\nprintf 'fixture-error' >&2\n"
        try Data(fixtureScript.utf8).write(to: fixtureURL)
        try FileManager.default.setAttributes([.posixPermissions: 0o700], ofItemAtPath: fixtureURL.path)

        let temporaryRootURL = fixtureRoot.appendingPathComponent("runner-temporary", isDirectory: true)
        try FileManager.default.createDirectory(at: temporaryRootURL, withIntermediateDirectories: true, attributes: [.posixPermissions: 0o700])
        let runner = ProcessDaemonCommandRunner(executableURL: fixtureURL, temporaryRootURL: temporaryRootURL)
        let result = try runner.run(arguments: [], standardInput: nil)

        XCTAssertEqual(result.terminationStatus, 0)
        XCTAssertEqual(result.standardOutput.count, 262144)
        XCTAssertEqual(String(decoding: result.standardError, as: UTF8.self), "fixture-error")
        XCTAssertEqual(try FileManager.default.contentsOfDirectory(atPath: temporaryRootURL.path), [])
    }
}
