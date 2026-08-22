import Foundation

@MainActor
final class ChatStore: ObservableObject {
    @Published var connection: ConnectionStatus = .disconnected
    @Published var address = UserDefaults.standard.string(forKey: "serverAddress") ?? ""
    @Published var pairingKey = KeychainStore.read(account: "pairing-key") ?? ""
    @Published var defaultCwd = UserDefaults.standard.string(forKey: "defaultCwd") ?? "~"
    @Published var sessions: [SessionSummary] = []
    @Published var selectedSession: SessionSummary?
    @Published var blocks: [ChatBlock] = []
    @Published var isBusy = false
    @Published var statusMessage = ""
    @Published var modelState = ModelState()

    private let client = GrokRemoteClient()
    private var reconnectTask: Task<Void, Never>?
    private var manuallyDisconnected = false
    private var activeProfile: ServerProfile?

    init() {
        client.onNotification = { [weak self] object in self?.handleNotification(object) }
        client.onDisconnect = { [weak self] error in self?.handleDisconnect(error) }
    }

    var hasSavedProfile: Bool { !address.isEmpty && !pairingKey.isEmpty }

    func connect(isReconnect: Bool = false) async {
        manuallyDisconnected = false
        if !isReconnect {
            reconnectTask?.cancel()
            reconnectTask = nil
        }
        do {
            let profile = try ServerProfile.parse(address: address, fallbackKey: pairingKey)
            activeProfile = profile
            address = profile.baseURL.absoluteString.trimmingCharacters(in: CharacterSet(charactersIn: "/"))
            pairingKey = profile.key
            UserDefaults.standard.set(address, forKey: "serverAddress")
            UserDefaults.standard.set(defaultCwd, forKey: "defaultCwd")
            KeychainStore.save(profile.key, account: "pairing-key")
            connection = .connecting
            let initialize = try await client.connect(profile: profile)
            applyModelState(from: initialize)
            connection = .connected
            statusMessage = "已连接"
            try await refreshSessions()
        } catch {
            connection = .failed(error.localizedDescription)
            statusMessage = error.localizedDescription
        }
    }

    func disconnect() {
        manuallyDisconnected = true
        reconnectTask?.cancel()
        client.disconnect(notify: false)
        connection = .disconnected
    }

    func refreshSessions() async throws {
        let raw = try await client.rpc("_x.ai/sessions/list", params: [:], timeout: 30)
        let rows = unwrapSessions(raw)
        sessions = rows.compactMap(SessionSummary.init(json:)).sorted {
            ($0.isResident ? 1 : 0, $0.updatedAt ?? .distantPast) >
            ($1.isResident ? 1 : 0, $1.updatedAt ?? .distantPast)
        }
    }

    func openSession(_ session: SessionSummary) async {
        selectedSession = session
        blocks = []
        isBusy = false
        statusMessage = "同步历史"
        do {
            let history = try await client.rest(
                path: "/api/session/history",
                query: [
                    URLQueryItem(name: "sessionId", value: session.id),
                    URLQueryItem(name: "cwd", value: session.cwd),
                    URLQueryItem(name: "limit", value: "400"),
                    URLQueryItem(name: "chat_only", value: "1")
                ]
            )
            for event in history["events"] as? [[String: Any]] ?? [] { ingestHistory(event) }
        } catch {
            statusMessage = "历史暂不可用：\(error.localizedDescription)"
        }

        do {
            let loaded = try await client.rpc("session/load", params: [
                "sessionId": session.id,
                "cwd": session.cwd,
                "mcpServers": []
            ], timeout: 90)
            if let object = loaded as? [String: Any] { applyModelState(from: object) }
            statusMessage = "在线"
        } catch {
            statusMessage = "挂载失败：\(error.localizedDescription)"
        }
    }

    func createSession(cwd: String) async -> SessionSummary? {
        do {
            let raw = try await client.rpc("session/new", params: ["cwd": cwd, "mcpServers": []], timeout: 60)
            guard let object = raw as? [String: Any],
                  let id = object.string("sessionId", "session_id") ?? object.object("session")?.string("sessionId") else {
                throw ClientError.malformedResponse
            }
            let session = SessionSummary(json: ["sessionId": id, "title": "新会话", "cwd": cwd, "resident": true])
            try? await refreshSessions()
            if let session { selectedSession = session }
            return session
        } catch {
            statusMessage = error.localizedDescription
            return nil
        }
    }

    func send(_ text: String) {
        guard let session = selectedSession else { return }
        let trimmed = text.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !trimmed.isEmpty else { return }
        blocks.append(ChatBlock(id: UUID().uuidString, kind: .user, text: trimmed))
        isBusy = true
        statusMessage = "等待 Grok"
        Task {
            do {
                _ = try await client.rpc("session/prompt", params: [
                    "sessionId": session.id,
                    "prompt": [["type": "text", "text": trimmed]]
                ])
            } catch {
                guard !error.localizedDescription.lowercased().contains("cancel") else { return }
                blocks.append(ChatBlock(id: UUID().uuidString, kind: .system, text: error.localizedDescription))
                isBusy = false
                statusMessage = "发送失败"
            }
        }
    }

    func cancel() {
        guard let session = selectedSession else { return }
        Task {
            try? await client.notify("session/cancel", params: ["sessionId": session.id])
            isBusy = false
            statusMessage = "已停止"
        }
    }

    func setEffort(_ effort: String) {
        guard let session = selectedSession else { return }
        Task {
            do {
                _ = try await client.rest(path: "/api/effort", method: "POST", body: [
                    "sessionId": session.id,
                    "modelId": modelState.currentModelID,
                    "effort": effort
                ])
                modelState.effort = effort
            } catch {
                statusMessage = "切换失败：\(error.localizedDescription)"
            }
        }
    }

    func answerPermission(blockID: String, optionID: String?) {
        guard let index = blocks.firstIndex(where: { $0.id == blockID }),
              let rpcID = blocks[index].rpcID else { return }
        Task {
            let outcome: [String: Any]
            if let optionID {
                outcome = ["outcome": ["outcome": "selected", "optionId": optionID]]
            } else {
                outcome = ["outcome": ["outcome": "cancelled"]]
            }
            try? await client.reply(id: rpcID, result: outcome)
            blocks.removeAll { $0.id == blockID }
        }
    }

    private func handleDisconnect(_ error: Error?) {
        guard !manuallyDisconnected else { return }
        connection = .reconnecting
        statusMessage = error?.localizedDescription ?? "连接中断"
        scheduleReconnect()
    }

    private func scheduleReconnect() {
        guard reconnectTask == nil || reconnectTask?.isCancelled == true else { return }
        reconnectTask = Task {
            var delay: UInt64 = 1
            while !Task.isCancelled && !manuallyDisconnected {
                try? await Task.sleep(nanoseconds: delay * 1_000_000_000)
                await connect(isReconnect: true)
                if connection == .connected {
                    reconnectTask = nil
                    return
                }
                connection = .reconnecting
                delay = min(delay * 2, 15)
            }
            reconnectTask = nil
        }
    }

    private func handleNotification(_ object: [String: Any]) {
        let method = object.string("method") ?? ""
        if method == "_x.ai/remote/pong" { return }
        if method == "session/update" || method == "_x.ai/session/update" || method == "x.ai/session/update" {
            let params = object.object("params") ?? [:]
            let sid = params.string("sessionId")
            guard sid == nil || sid == selectedSession?.id else { return }
            let update = params.object("update") ?? params
            applyUpdate(update)
            return
        }
        if method == "_x.ai/sessions/changed" {
            Task { try? await refreshSessions() }
            return
        }
        if method.contains("permission") || method.contains("ask_user") {
            guard let rpcID = (object["id"] as? NSNumber)?.intValue else { return }
            let params = object.object("params") ?? [:]
            let question = params.string("question", "message") ?? "Grok 需要你的确认"
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
        case "user_message_chunk": appendChunk(kind: .user, text: text)
        case "agent_message_chunk":
            appendChunk(kind: .assistant, text: text)
            isBusy = true; statusMessage = "正在回复"
        case "agent_thought_chunk":
            appendChunk(kind: .thinking, text: text)
            isBusy = true; statusMessage = "正在思考"
        case "tool_call", "tool_call_update":
            upsertTool(update)
            isBusy = true; statusMessage = update.string("title", "toolName", "kind") ?? "正在使用工具"
        case "plan":
            appendChunk(kind: .plan, text: text.isEmpty ? String(describing: update["entries"] ?? "Plan") : text)
        case "session_recap": appendChunk(kind: .system, text: text)
        case "turn_completed", "task_completed":
            isBusy = false; statusMessage = "完成"
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
        let title = update.string("title", "toolName", "kind") ?? "工具"
        let status = ToolRunState(raw: update.string("status", "toolStatus"))
        let detail = update.string("content", "result") ?? update.object("content")?.string("text") ?? ""
        if let index = blocks.firstIndex(where: { $0.id == "tool-\(id)" }) {
            blocks[index].title = title
            blocks[index].toolState = status
            if !detail.isEmpty { blocks[index].detail = detail }
        } else {
            blocks.append(ChatBlock(id: "tool-\(id)", kind: .tool, title: title, detail: detail, toolState: status))
        }
    }

    private func ingestHistory(_ event: [String: Any]) {
        if let params = event.object("params") {
            applyUpdate(params.object("update") ?? params)
        } else if let update = event.object("update") {
            applyUpdate(update)
        } else {
            applyUpdate(event)
        }
        isBusy = false
    }

    private func unwrapSessions(_ raw: Any) -> [[String: Any]] {
        if let rows = raw as? [[String: Any]] { return rows }
        if let object = raw as? [String: Any] {
            if let rows = object["sessions"] as? [[String: Any]] { return rows }
            if let result = object["result"] { return unwrapSessions(result) }
        }
        return []
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
