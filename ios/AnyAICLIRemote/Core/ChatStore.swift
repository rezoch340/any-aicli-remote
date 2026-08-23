import Foundation

@MainActor
final class ChatStore: ObservableObject {
    @Published var connection: ConnectionStatus = .disconnected
    @Published private(set) var devices: [SavedDevice]
    @Published private(set) var activeDeviceID: UUID?
    @Published private(set) var navigationResetToken = UUID()
    @Published private(set) var deviceMessage = ""
    @Published private(set) var deviceMessageIsError = false
    @Published private(set) var deviceHealthStatuses: [UUID: DeviceHealthStatus] = [:]
    @Published var sessions: [SessionSummary] = []
    @Published var selectedSession: SessionSummary?
    @Published var blocks: [ChatBlock] = []
    @Published var isBusy = false
    @Published var statusMessage = ""
    @Published var modelState = ModelState()
    @Published var selectedFiles: [WorkspaceFile] = []
    @Published var filePickerPath = "."
    @Published var filePickerParent: String?
    @Published var filePickerDirectories: [WorkspaceFile] = []
    @Published var filePickerFiles: [WorkspaceFile] = []
    @Published var filePickerVisible = false
    @Published var filePickerLoading = false
    @Published var filePickerError: String?
    @Published private(set) var isSessionLoading = false

    private let client: AnyAICLIRemoteClient
    private let healthProbe: DeviceHealthProbe
    private let runtimeConfiguration: ClientRuntimeConfiguration
    private var activeTurnID: UUID?
    private let pendingUserEchoTracker = PendingUserEchoTracker()
    private var connectionGeneration = UUID()
    private var sessionGeneration = UUID()
    private var sessionLoadTask: Task<Void, Never>?
    private var mountedSessionIdentity: SessionIdentity?
    private var healthProbeGeneration = UUID()
    private static let devicesDefaultsKey = "savedDevices.v1"
    private static let keychainAccountPrefix = "pairing-key."

    init(healthProbe: DeviceHealthProbe? = nil, runtimeConfiguration: ClientRuntimeConfiguration = ClientRuntimeConfiguration()) {
        self.runtimeConfiguration = runtimeConfiguration
        client = AnyAICLIRemoteClient(runtimeConfiguration: runtimeConfiguration)
        self.healthProbe = healthProbe ?? DeviceHealthProbe(runtimeConfiguration: runtimeConfiguration)
        let loadResult = Self.loadDevicesAndMigrateLegacyProfile()
        devices = loadResult.devices
        if let errorMessage = loadResult.errorMessage {
            deviceMessage = errorMessage
            deviceMessageIsError = true
        }
        client.onNotification = { [weak self] object in self?.handleNotification(object) }
        client.onDisconnect = { [weak self] error in self?.handleDisconnect(error) }
    }

    var healthPollingInterval: TimeInterval { runtimeConfiguration.healthPollingInterval }

    var activeDevice: SavedDevice? {
        guard let activeDeviceID else { return nil }
        return devices.first(where: { $0.id == activeDeviceID })
    }

    func pairingKey(for deviceID: UUID) throws -> String {
        try KeychainStore.read(account: Self.keychainAccount(for: deviceID)) ?? ""
    }

    @discardableResult
    func saveDevice(
        id: UUID? = nil,
        name: String,
        address: String,
        pairingKey: String
    ) throws -> UUID {
        let deviceID = id ?? UUID()
        let account = Self.keychainAccount(for: deviceID)
        let savedKey: String
        if id == nil {
            savedKey = ""
        } else {
            savedKey = try KeychainStore.read(account: account) ?? ""
        }
        let profile = try ServerProfile.parse(
            address: address,
            fallbackKey: pairingKey.isEmpty ? savedKey : pairingKey
        )
        let trimmedName = name.trimmingCharacters(in: .whitespacesAndNewlines)
        let resolvedName = trimmedName.isEmpty ? (profile.baseURL.host ?? ProductIdentifiers.displayName) : trimmedName
        let device = SavedDevice(
            id: deviceID,
            name: resolvedName,
            baseURL: profile.baseURL
        )
        var updatedDevices = devices
        if let index = updatedDevices.firstIndex(where: { $0.id == deviceID }) {
            updatedDevices[index] = device
        } else {
            updatedDevices.append(device)
        }

        try KeychainStore.save(profile.key, account: account)
        do {
            try Self.persistDevices(updatedDevices)
        } catch {
            if savedKey.isEmpty {
                try? KeychainStore.delete(account: account)
            } else {
                try? KeychainStore.save(savedKey, account: account)
            }
            throw error
        }

        devices = updatedDevices
        deviceHealthStatuses[deviceID] = .checking
        deviceMessage = "已保存 \(resolvedName)"
        deviceMessageIsError = false
        return deviceID
    }

    func deleteDevice(_ deviceID: UUID) throws {
        guard let deletedDevice = devices.first(where: { $0.id == deviceID }) else { return }
        let originalDevices = devices
        let updatedDevices = devices.filter { $0.id != deviceID }
        let account = Self.keychainAccount(for: deviceID)

        try Self.persistDevices(updatedDevices)
        do {
            try KeychainStore.delete(account: account)
        } catch {
            try? Self.persistDevices(originalDevices)
            throw error
        }

        devices = updatedDevices
        deviceHealthStatuses.removeValue(forKey: deviceID)
        if activeDeviceID == deviceID { disconnect() }
        deviceMessage = "已删除 \(deletedDevice.name)"
        deviceMessageIsError = false
    }

    func reportDeviceError(_ error: Error) {
        deviceMessage = error.localizedDescription
        deviceMessageIsError = true
    }

    func deviceHealthStatus(for deviceID: UUID) -> DeviceHealthStatus {
        deviceHealthStatuses[deviceID] ?? .checking
    }

    func refreshDeviceHealth() async {
        let devicesSnapshot = devices
        let refreshGeneration = UUID()
        healthProbeGeneration = refreshGeneration
        let currentDeviceIDs = Set(devicesSnapshot.map(\.id))
        deviceHealthStatuses = deviceHealthStatuses.filter { currentDeviceIDs.contains($0.key) }
        for device in devicesSnapshot {
            deviceHealthStatuses[device.id] = .checking
        }

        await withTaskGroup(of: (UUID, Bool).self) { group in
            for device in devicesSnapshot {
                group.addTask { [healthProbe] in
                    let isOnline = await healthProbe.isOnline(baseURL: device.baseURL)
                    return (device.id, isOnline)
                }
            }
            for await (deviceID, isOnline) in group {
                guard healthProbeGeneration == refreshGeneration,
                      devices.contains(where: { $0.id == deviceID }) else { continue }
                deviceHealthStatuses[deviceID] = isOnline ? .online : .offline
            }
        }
    }

    @discardableResult
    func importPairingDeepLink(_ pairingDeepLink: URL) -> Bool {
        do {
            let parsedLink = try PairingDeepLink.parse(pairingDeepLink)
            let existingDevice = devices.first { $0.baseURL == parsedLink.profile.baseURL }
            let resolvedName = parsedLink.name ?? existingDevice?.name
                ?? parsedLink.profile.baseURL.host ?? ProductIdentifiers.displayName
            _ = try saveDevice(id: existingDevice?.id, name: resolvedName,
                               address: parsedLink.serviceAddress, pairingKey: parsedLink.profile.key)
            disconnect()
            deviceMessage = existingDevice == nil
                ? "设备已添加，请点击设备连接" : "设备已更新，请点击设备连接"
            deviceMessageIsError = false
            return true
        } catch {
            deviceMessage = error.localizedDescription
            deviceMessageIsError = true
            return false
        }
    }

    func connect(to deviceID: UUID) async -> Bool {
        guard let device = devices.first(where: { $0.id == deviceID }) else { return false }
        if activeDeviceID == deviceID, connection == .connected {
            return true
        }

        let attemptID = UUID()
        connectionGeneration = attemptID
        client.disconnect(notify: false)
        clearSessionState()
        activeDeviceID = deviceID
        connection = .connecting
        deviceMessage = "正在连接 \(device.name)"
        deviceMessageIsError = false
        do {
            let profile = ServerProfile(baseURL: device.baseURL, key: try pairingKey(for: deviceID))
            guard !profile.key.isEmpty else { throw ClientError.missingKey }
            let initialize = try await client.connect(profile: profile)
            guard ownsConnectionAttempt(attemptID, deviceID: deviceID) else { return false }

            let refreshedSessions = try await fetchSessions()
            guard ownsConnectionAttempt(attemptID, deviceID: deviceID), client.isConnected else {
                return false
            }

            applyModelState(from: initialize)
            sessions = refreshedSessions
            connection = .connected
            statusMessage = "已连接"
            deviceMessage = ""
            deviceMessageIsError = false
            return true
        } catch {
            guard ownsConnectionAttempt(attemptID, deviceID: deviceID) else { return false }
            client.disconnect(notify: false)
            activeDeviceID = nil
            connection = .failed(error.localizedDescription)
            statusMessage = error.localizedDescription
            deviceMessage = "连接失败：\(error.localizedDescription)"
            deviceMessageIsError = true
            navigationResetToken = UUID()
            return false
        }
    }

    func disconnect() {
        connectionGeneration = UUID()
        client.disconnect(notify: false)
        activeDeviceID = nil
        clearSessionState()
        connection = .disconnected
        statusMessage = ""
        navigationResetToken = UUID()
    }

    func refreshSessions() async throws {
        guard let context = currentConnectionContext() else { throw ClientError.disconnected }
        try await refreshSessions(context: context)
    }

    private func refreshSessions(context: ConnectionContext) async throws {
        guard ownsConnection(context) else { throw CancellationError() }
        let refreshedSessions = try await fetchSessions()
        guard !Task.isCancelled, ownsConnection(context) else { throw CancellationError() }
        sessions = refreshedSessions
    }

    private func fetchSessions() async throws -> [SessionSummary] {
        let response = try await client.rest(path: "/api/sessions")
        guard let rows = response["sessions"] as? [[String: Any]] else {
            throw ClientError.malformedResponse
        }
        return rows.compactMap { SessionSummary(json: $0) }.sorted {
            ($0.lastActiveAt ?? $0.createdAt ?? .distantPast) >
                ($1.lastActiveAt ?? $1.createdAt ?? .distantPast)
        }
    }

    func openSession(_ session: SessionSummary) {
        guard let connectionContext = currentConnectionContext() else { return }
        sessionLoadTask?.cancel()
        isSessionLoading = true
        let requiresSessionLoad = mountedSessionIdentity != session.id
        if requiresSessionLoad { mountedSessionIdentity = nil }
        let loadGeneration = UUID()
        sessionGeneration = loadGeneration
        resetTurnTracking()
        selectedSession = session
        blocks = []
        isBusy = false
        closeWorkspaceFilePicker(clearSelection: true)
        statusMessage = "同步历史"
        let context = SessionContext(
            connection: connectionContext,
            sessionIdentity: session.id,
            generation: loadGeneration
        )
        sessionLoadTask = Task { [weak self] in
            await self?.loadSession(
                session,
                context: context,
                requiresSessionLoad: requiresSessionLoad
            )
        }
    }

    func closeSession(_ sessionIdentity: SessionIdentity) {
        guard selectedSession?.id == sessionIdentity else { return }
        sessionLoadTask?.cancel()
        sessionLoadTask = nil
        isSessionLoading = false
        sessionGeneration = UUID()
        resetTurnTracking()
        selectedSession = nil
        blocks = []
        isBusy = false
        closeWorkspaceFilePicker(clearSelection: true)
        statusMessage = connection == .connected ? "已连接" : ""
    }

    private func loadSession(
        _ session: SessionSummary,
        context: SessionContext,
        requiresSessionLoad: Bool
    ) async {
        defer {
            if sessionGeneration == context.generation {
                sessionLoadTask = nil
                isSessionLoading = false
            }
        }
        do {
            let history = try await client.rest(
                pathComponents: ["api", "sessions", session.sessionID, "messages"],
                query: [URLQueryItem(name: "providerId", value: session.providerID)]
            )
            guard !Task.isCancelled, ownsSession(context) else { return }
            guard let metadata = history.object("session"),
                  let authoritativeSession = SessionSummary(
                    json: metadata,
                    fallbackProviderID: history.string("providerId") ?? session.providerID
                  ),
                  authoritativeSession.id == context.sessionIdentity,
                  let messages = history["messages"] as? [[String: Any]] else {
                throw ClientError.malformedResponse
            }
            applyAuthoritativeSession(authoritativeSession)
            blocks = Self.chatBlocks(from: messages)
        } catch {
            guard !Task.isCancelled, ownsSession(context) else { return }
            statusMessage = "历史暂不可用：\(error.localizedDescription)"
        }

        guard !Task.isCancelled, ownsSession(context) else { return }
        guard requiresSessionLoad else {
            statusMessage = "在线"
            return
        }
        do {
            let loaded = try await client.rpc("session/load", params: [
                "sessionId": session.sessionID,
                "mcpServers": []
            ], timeout: runtimeConfiguration.sessionLoadTimeout)
            guard !Task.isCancelled, ownsSession(context) else { return }
            if let object = loaded as? [String: Any] { applyModelState(from: object) }
            mountedSessionIdentity = session.id
            statusMessage = "在线"
        } catch {
            guard !Task.isCancelled, ownsSession(context) else { return }
            statusMessage = "挂载失败：\(error.localizedDescription)"
        }
    }

    func createSession(cwd: String) async -> Bool {
        guard let context = currentConnectionContext() else { return false }
        let workingDirectory = cwd.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !workingDirectory.isEmpty else { return false }
        let previousSessionIdentities = Set(sessions.map(\.id))
        do {
            let raw = try await client.rpc(
                "session/new",
                params: ["cwd": workingDirectory, "mcpServers": []],
                timeout: runtimeConfiguration.sessionCreateTimeout
            )
            guard !Task.isCancelled, ownsConnection(context) else { return false }
            guard let object = raw as? [String: Any],
                  let newSessionID = object.string("sessionId", "session_id") ??
                    object.object("session")?.string("sessionId", "session_id") else {
                throw ClientError.malformedResponse
            }
            let responseMetadata = object.object("session") ?? object
            let rawResponseProviderID = responseMetadata.string("providerId", "provider_id") ??
                object.string("providerId", "provider_id")
            let responseProviderID = rawResponseProviderID.flatMap { value in
                let normalizedValue = value.trimmingCharacters(in: .whitespacesAndNewlines)
                return normalizedValue.isEmpty ? nil : normalizedValue
            }
            let rawResponseProjectDirectory = responseMetadata.string("projectDir") ??
                object.string("projectDir")
            let responseProjectDirectory = rawResponseProjectDirectory.flatMap { value in
                let normalizedValue = value.trimmingCharacters(in: .whitespacesAndNewlines)
                return normalizedValue.isEmpty ? nil : normalizedValue
            }
            do {
                try await refreshSessions(context: context)
            } catch {
                guard !Task.isCancelled, ownsConnection(context) else { return false }
                statusMessage = "会话列表刷新失败：\(error.localizedDescription)"
            }
            guard !Task.isCancelled, ownsConnection(context) else { return false }
            sessionLoadTask?.cancel()
            sessionLoadTask = nil
            isSessionLoading = false
            resetTurnTracking()
            selectedSession = nil
            sessionGeneration = UUID()
            mountedSessionIdentity = nil

            let canonicalSession: SessionSummary?
            if let responseProviderID {
                let responseIdentity = SessionIdentity(
                    providerID: responseProviderID,
                    sessionID: newSessionID
                )
                canonicalSession = sessions.first(where: { $0.id == responseIdentity })
            } else {
                let newlyIndexedIdentities = Set(sessions.map(\.id))
                    .subtracting(previousSessionIdentities)
                let newMatches = sessions.filter {
                    newlyIndexedIdentities.contains($0.id) && $0.sessionID == newSessionID
                }
                canonicalSession = newMatches.count == 1 ? newMatches.first : nil
            }

            var serverResponseSession: SessionSummary?
            if canonicalSession == nil,
               let responseProviderID,
               let responseProjectDirectory {
                var normalizedMetadata = responseMetadata
                normalizedMetadata["providerId"] = responseProviderID
                normalizedMetadata["sessionId"] = newSessionID
                normalizedMetadata["projectDir"] = responseProjectDirectory
                serverResponseSession = SessionSummary(json: normalizedMetadata)
            }

            if let resolvedSession = canonicalSession ?? serverResponseSession {
                selectedSession = resolvedSession
                mountedSessionIdentity = resolvedSession.id
                statusMessage = "已创建"
            } else {
                statusMessage = "已创建，等待历史索引"
            }
            return true
        } catch {
            guard !Task.isCancelled, ownsConnection(context) else { return false }
            statusMessage = error.localizedDescription
            return false
        }
    }

    func send(_ text: String) {
        guard let session = selectedSession,
              let context = currentSessionContext(sessionIdentity: session.id) else { return }
        let trimmed = text.trimmingCharacters(in: .whitespacesAndNewlines)
        let attachments = selectedFiles
        guard !trimmed.isEmpty || !attachments.isEmpty else { return }
        let turnID = UUID()
        activeTurnID = turnID
        pendingUserEchoTracker.clear()
        if !trimmed.isEmpty { pendingUserEchoTracker.begin(text: trimmed) }
        blocks.append(ChatBlock(id: UUID().uuidString, kind: .user, text: trimmed, attachments: attachments))
        closeWorkspaceFilePicker(clearSelection: true)
        isBusy = true
        statusMessage = "等待助手"
        Task {
            do {
                guard ownsSession(context) else { return }
                let response = try await client.rpc("session/prompt", params: [
                    "sessionId": session.sessionID,
                    "prompt": Self.promptBlocks(text: trimmed, attachments: attachments)
                ])
                guard !Task.isCancelled, ownsSession(context), activeTurnID == turnID else { return }
                let stopReason = (response as? [String: Any])?.string("stopReason")?.lowercased()
                if stopReason?.contains("cancel") == true {
                    finishTurn(toolState: .cancelled, status: "已停止")
                } else {
                    finishTurn(toolState: .success, status: "完成")
                }
            } catch {
                guard !Task.isCancelled, ownsSession(context), activeTurnID == turnID else { return }
                guard !error.localizedDescription.lowercased().contains("cancel") else {
                    finishTurn(toolState: .cancelled, status: "已停止")
                    return
                }
                blocks.append(ChatBlock(id: UUID().uuidString, kind: .system, text: error.localizedDescription))
                finishTurn(toolState: .failed, status: "发送失败")
            }
        }
    }

    nonisolated static func promptBlocks(text: String, attachments: [WorkspaceFile]) -> [[String: Any]] {
        var result: [[String: Any]] = []
        if !text.isEmpty { result.append(["type": "text", "text": text]) }
        for file in attachments {
            var link: [String: Any] = ["type": "resource_link", "name": file.name, "uri": file.uri, "description": file.relativePath]
            if file.size > 0 { link["size"] = file.size }
            result.append(link)
        }
        return result
    }

    nonisolated static func shouldMarkTurnBusy(activeTurnID: UUID?) -> Bool {
        activeTurnID != nil
    }

    func closeWorkspaceFilePicker(clearSelection: Bool = false) {
        filePickerVisible = false
        filePickerLoading = false
        filePickerError = nil
        filePickerPath = "."
        filePickerParent = nil
        filePickerDirectories = []
        filePickerFiles = []
        if clearSelection { selectedFiles = [] }
    }

    func browseWorkspace(path: String = ".") {
        guard let session = selectedSession, let context = currentSessionContext(sessionIdentity: session.id) else { return }
        filePickerVisible = true
        filePickerLoading = true
        filePickerError = nil
        Task { [weak self] in
            guard let self else { return }
            do {
                let response = try await client.rest(path: "/api/fs/list", query: [URLQueryItem(name: "providerId", value: session.providerID), URLQueryItem(name: "sessionId", value: session.sessionID), URLQueryItem(name: "path", value: path)])
                guard ownsSession(context) else { return }
                filePickerPath = response.string("path") ?? path
                filePickerParent = response.string("parent")
                filePickerDirectories = response.array("dirs").compactMap { WorkspaceFile(json: $0, directory: true) }
                filePickerFiles = response.array("files").compactMap { WorkspaceFile(json: $0, directory: false) }
                filePickerLoading = false
            } catch {
                if ownsSession(context) {
                    filePickerLoading = false
                    filePickerError = error.localizedDescription
                }
            }
        }
    }
    func toggleFile(_ file: WorkspaceFile) {
        guard !file.directory else { return }
        selectedFiles = selectedFiles.contains(file) ? selectedFiles.filter { $0 != file } : selectedFiles + [file]
    }
    func removeFile(_ file: WorkspaceFile) {
        selectedFiles.removeAll { $0 == file }
    }

    func cancel() {
        guard let session = selectedSession,
              let context = currentSessionContext(sessionIdentity: session.id) else { return }
        finishTurn(toolState: .cancelled, status: "已停止")
        Task {
            guard ownsSession(context) else { return }
            try? await client.notify("session/cancel", params: ["sessionId": session.sessionID])
            guard !Task.isCancelled, ownsSession(context) else { return }
            finishTurn(toolState: .cancelled, status: "已停止")
        }
    }

    func setEffort(_ effort: String) {
        guard let session = selectedSession,
              let context = currentSessionContext(sessionIdentity: session.id) else { return }
        let modelID = modelState.currentModelID
        Task {
            do {
                guard ownsSession(context) else { return }
                _ = try await client.rest(path: "/api/effort", method: "POST", body: [
                    "providerId": session.providerID,
                    "sessionId": session.sessionID,
                    "modelId": modelID,
                    "effort": effort
                ])
                guard !Task.isCancelled, ownsSession(context) else { return }
                modelState.effort = effort
            } catch {
                guard !Task.isCancelled, ownsSession(context) else { return }
                statusMessage = "切换失败：\(error.localizedDescription)"
            }
        }
    }

    func answerPermission(blockID: String, optionID: String?) {
        guard let index = blocks.firstIndex(where: { $0.id == blockID }),
              let rpcID = blocks[index].rpcID,
              let sessionIdentity = selectedSession?.id,
              let context = currentSessionContext(sessionIdentity: sessionIdentity) else { return }
        Task {
            guard ownsSession(context) else { return }
            let outcome: [String: Any]
            if let optionID {
                outcome = ["outcome": ["outcome": "selected", "optionId": optionID]]
            } else {
                outcome = ["outcome": ["outcome": "cancelled"]]
            }
            try? await client.reply(id: rpcID, result: outcome)
            guard !Task.isCancelled, ownsSession(context) else { return }
            blocks.removeAll { $0.id == blockID }
        }
    }

    private func handleDisconnect(_ error: Error?) {
        guard activeDeviceID != nil else { return }
        connectionGeneration = UUID()
        client.disconnect(notify: false)
        activeDeviceID = nil
        clearSessionState()
        let message = error?.localizedDescription ?? "连接中断"
        connection = .failed(message)
        statusMessage = message
        deviceMessage = "设备连接已断开"
        deviceMessageIsError = true
        navigationResetToken = UUID()
    }

    private func handleNotification(_ object: [String: Any]) {
        guard connection == .connected else { return }
        let method = object.string("method") ?? ""
        if method == "session/update" {
            let params = object.object("params") ?? [:]
            guard matchesSelectedSession(params) else { return }
            let update = params.object("update") ?? params
            applyUpdate(update)
            return
        }
        if method == "sessions/changed" {
            guard let context = currentConnectionContext() else { return }
            Task { try? await refreshSessions(context: context) }
            return
        }
        if method.contains("permission") || method.contains("ask_user") {
            guard let rpcID = (object["id"] as? NSNumber)?.intValue else { return }
            let params = object.object("params") ?? [:]
            guard matchesSelectedSession(params) else { return }
            let question = params.string("question", "message") ?? "CLI 需要你的确认"
            let options = (params["options"] as? [[String: Any]] ?? []).map {
                PermissionOption(
                    id: $0.string("optionId", "id") ?? "allow",
                    label: $0.string("name", "label") ?? "允许"
                )
            }
            blocks.append(ChatBlock(
                id: "permission-\(rpcID)", kind: .permission, text: question,
                rpcID: rpcID,
                options: options.isEmpty ? [PermissionOption(id: "allow", label: "允许")] : options
            ))
        }
    }

    private func applyUpdate(_ update: [String: Any]) {
        let type = update.string("sessionUpdate") ?? ""
        let text = update.object("content")?.string("text") ?? update.string("text") ?? ""
        switch type {
        case "user_message_chunk":
            if !pendingUserEchoTracker.consume(chunk: text) { appendChunk(kind: .user, text: text) }
        case "agent_message_chunk":
            appendChunk(kind: .assistant, text: text)
            if Self.shouldMarkTurnBusy(activeTurnID: activeTurnID) {
                isBusy = true
                statusMessage = "正在回复"
            }
        case "agent_thought_chunk":
            appendChunk(kind: .thinking, text: text)
            if Self.shouldMarkTurnBusy(activeTurnID: activeTurnID) {
                isBusy = true
                statusMessage = "正在思考"
            }
        case "tool_call", "tool_call_update":
            upsertTool(update)
            if Self.shouldMarkTurnBusy(activeTurnID: activeTurnID) {
                isBusy = true
                statusMessage = update.string("title", "toolName", "kind") ?? "正在使用工具"
            }
        case "plan":
            appendChunk(kind: .plan, text: text.isEmpty ? String(describing: update["entries"] ?? "Plan") : text)
        case "session_recap": appendChunk(kind: .system, text: text)
        case "turn_completed", "task_completed":
            finishTurn(toolState: .success, status: "完成")
        case "cancelled", "turn_cancelled", "task_cancelled":
            finishTurn(toolState: .cancelled, status: "已停止")
        case "turn_failed", "task_failed", "failed", "error":
            finishTurn(toolState: .failed, status: "执行失败")
        default: break
        }
    }

    private func appendChunk(kind: ChatBlockKind, text: String) {
        guard !text.isEmpty else { return }
        if let last = blocks.indices.last, blocks[last].kind == kind,
           [.user, .assistant, .thinking].contains(kind) {
            blocks[last].text += text
        } else {
            blocks.append(ChatBlock(id: UUID().uuidString, kind: kind, text: text))
        }
    }

    private func upsertTool(_ update: [String: Any]) {
        let id = update.string("toolCallId", "tool_call_id", "id") ?? UUID().uuidString
        let title = update.string("title", "toolName", "kind")
        let rawStatus = update.string("status", "toolStatus")
        let detail = update.string("content", "result") ?? update.object("content")?.string("text") ?? ""
        if let index = blocks.firstIndex(where: { $0.id == "tool-\(id)" }) {
            if let title { blocks[index].title = title }
            if rawStatus != nil { blocks[index].toolState = ToolRunState(raw: rawStatus) }
            if !detail.isEmpty { blocks[index].detail = detail }
        } else {
            blocks.append(ChatBlock(
                id: "tool-\(id)",
                kind: .tool,
                title: title ?? "工具",
                detail: detail,
                toolState: ToolRunState(raw: rawStatus)
            ))
        }
    }

    private func resetTurnTracking() {
        activeTurnID = nil
        pendingUserEchoTracker.clear()
    }

    private func finishTurn(toolState: ToolRunState, status: String) {
        resetTurnTracking()
        blocks = Self.finalizingActiveTools(in: blocks, as: toolState)
        isBusy = false
        statusMessage = status
    }

    private func matchesSelectedSession(_ params: [String: Any]) -> Bool {
        guard let selectedSession else { return false }
        return Self.matchesSessionIdentity(params, expected: selectedSession.id)
    }

    nonisolated static func matchesSessionIdentity(
        _ params: [String: Any],
        expected: SessionIdentity
    ) -> Bool {
        params.string("providerId") == expected.providerID &&
            params.string("sessionId") == expected.sessionID
    }

    nonisolated static func finalizingActiveTools(
        in blocks: [ChatBlock],
        as finalState: ToolRunState
    ) -> [ChatBlock] {
        blocks.map { block in
            guard block.kind == .tool, [.pending, .running].contains(block.toolState) else {
                return block
            }
            var finalizedBlock = block
            finalizedBlock.toolState = finalState
            return finalizedBlock
        }
    }

    private func clearSessionState() {
        sessionLoadTask?.cancel()
        sessionLoadTask = nil
        isSessionLoading = false
        sessionGeneration = UUID()
        mountedSessionIdentity = nil
        resetTurnTracking()
        sessions = []
        selectedSession = nil
        blocks = []
        isBusy = false
        modelState = ModelState()
        closeWorkspaceFilePicker(clearSelection: true)
    }

    private func ownsConnectionAttempt(_ attemptID: UUID, deviceID: UUID) -> Bool {
        connectionGeneration == attemptID && activeDeviceID == deviceID
    }

    private func currentConnectionContext() -> ConnectionContext? {
        guard connection == .connected, client.isConnected, let activeDeviceID else { return nil }
        return ConnectionContext(deviceID: activeDeviceID, generation: connectionGeneration)
    }

    private func ownsConnection(_ context: ConnectionContext) -> Bool {
        connection == .connected &&
            client.isConnected &&
            activeDeviceID == context.deviceID &&
            connectionGeneration == context.generation
    }

    private func currentSessionContext(sessionIdentity: SessionIdentity) -> SessionContext? {
        guard let connection = currentConnectionContext(), selectedSession?.id == sessionIdentity else {
            return nil
        }
        return SessionContext(
            connection: connection,
            sessionIdentity: sessionIdentity,
            generation: sessionGeneration
        )
    }

    private func ownsSession(_ context: SessionContext) -> Bool {
        ownsConnection(context.connection) &&
            selectedSession?.id == context.sessionIdentity &&
            sessionGeneration == context.generation
    }

    private static func persistDevices(
        _ devices: [SavedDevice],
        defaults: UserDefaults = .standard
    ) throws {
        let data = try JSONEncoder().encode(devices)
        defaults.set(data, forKey: devicesDefaultsKey)
        guard defaults.data(forKey: devicesDefaultsKey) == data else {
            throw DeviceStorageError.persistenceFailed
        }
    }

    private static func keychainAccount(for deviceID: UUID) -> String {
        keychainAccountPrefix + deviceID.uuidString.lowercased()
    }

    private static func loadDevicesAndMigrateLegacyProfile() -> DeviceLoadResult {
        let defaults = UserDefaults.standard
        let legacyDomain = defaults.persistentDomain(forName: LegacyCompatibility.bundleIdentifier) ?? [:]
        defaults.removeObject(forKey: "defaultCwd")
        let savedDeviceData = defaults.data(forKey: devicesDefaultsKey) ??
            legacyDomain[devicesDefaultsKey] as? Data
        if let data = savedDeviceData {
            do {
                let savedDevices = try JSONDecoder().decode([SavedDevice].self, from: data)
                try persistDevices(savedDevices, defaults: defaults)
                return DeviceLoadResult(
                    devices: savedDevices,
                    errorMessage: nil
                )
            } catch {
                return DeviceLoadResult(
                    devices: [],
                    errorMessage: "设备列表迁移失败，原数据已保留：\(error.localizedDescription)"
                )
            }
        }

        let legacyAddress = defaults.string(forKey: "serverAddress") ??
            legacyDomain["serverAddress"] as? String ?? ""
        do {
            let legacyKey = try KeychainStore.read(account: "pairing-key") ?? ""
            if legacyAddress.isEmpty, legacyKey.isEmpty {
                try persistDevices([], defaults: defaults)
                return DeviceLoadResult(devices: [], errorMessage: nil)
            }

            let profile = try ServerProfile.parse(address: legacyAddress, fallbackKey: legacyKey)
            let deviceID = UUID()
            let savedDevices = [SavedDevice(
                id: deviceID,
                name: profile.baseURL.host ?? ProductIdentifiers.displayName,
                baseURL: profile.baseURL
            )]
            let migratedAccount = keychainAccount(for: deviceID)

            try KeychainStore.save(profile.key, account: migratedAccount)
            do {
                try persistDevices(savedDevices, defaults: defaults)
            } catch {
                try? KeychainStore.delete(account: migratedAccount)
                throw error
            }

            do {
                try KeychainStore.delete(account: "pairing-key")
            } catch {
                defaults.removeObject(forKey: devicesDefaultsKey)
                try? KeychainStore.delete(account: migratedAccount)
                throw error
            }

            defaults.removeObject(forKey: "serverAddress")
            return DeviceLoadResult(devices: savedDevices, errorMessage: nil)
        } catch {
            return DeviceLoadResult(
                devices: [],
                errorMessage: "旧设备配置迁移失败，原配置已保留：\(error.localizedDescription)"
            )
        }
    }

    private func applyAuthoritativeSession(_ session: SessionSummary) {
        if let index = sessions.firstIndex(where: { $0.id == session.id }) {
            sessions[index] = session
        } else {
            sessions.append(session)
        }
        selectedSession = session
    }

    private static func chatBlocks(from messages: [[String: Any]]) -> [ChatBlock] {
        messages.enumerated().compactMap { messageIndex, message in
            guard let role = message.string("role")?.lowercased(),
                  let content = message.string("content") else { return nil }
            let blockID = "history-\(messageIndex)"
            switch role {
            case "system":
                return ChatBlock(id: blockID, kind: .system, text: content)
            case "user":
                return ChatBlock(id: blockID, kind: .user, text: content)
            case "assistant":
                return ChatBlock(id: blockID, kind: .assistant, text: content)
            case "tool":
                return ChatBlock(
                    id: blockID,
                    kind: .tool,
                    title: "工具",
                    detail: content,
                    toolState: .success
                )
            default:
                return nil
            }
        }
    }

    private func applyModelState(from object: [String: Any]) {
        let source = object.object("models") ?? object.object("_meta")?.object("modelState") ?? object.object("modelState")
        guard let source else { return }
        if let model = source.string("currentModelId") { modelState.currentModelID = model }
        let available = source["availableModels"] as? [[String: Any]] ?? []
        if let current = available.first(where: { $0.string("modelId") == modelState.currentModelID }),
           let meta = current.object("_meta") {
            if let effort = meta.string("reasoningEffort") { modelState.effort = effort }
            let levels = (meta["reasoningEfforts"] as? [[String: Any]] ?? []).compactMap { $0.string("value", "id") }
            if !levels.isEmpty { modelState.effortLevels = levels }
        }
    }
}

private struct DeviceLoadResult {
    let devices: [SavedDevice]
    let errorMessage: String?
}

private struct ConnectionContext {
    let deviceID: UUID
    let generation: UUID
}

private struct SessionContext {
    let connection: ConnectionContext
    let sessionIdentity: SessionIdentity
    let generation: UUID
}

private enum DeviceStorageError: LocalizedError {
    case persistenceFailed

    var errorDescription: String? {
        "设备列表保存失败"
    }
}
