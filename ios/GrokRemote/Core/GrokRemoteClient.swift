import Foundation

@MainActor
final class GrokRemoteClient {
    var onNotification: (([String: Any]) -> Void)?
    var onDisconnect: ((Error?) -> Void)?

    private var profile: ServerProfile?
    private var socket: URLSessionWebSocketTask?
    private var receiveTask: Task<Void, Never>?
    private var pending: [Int: CheckedContinuation<Any, Error>] = [:]
    private var nextID = 0

    var isConnected: Bool { socket?.state == .running }

    func connect(profile: ServerProfile) async throws -> [String: Any] {
        disconnect(notify: false)
        self.profile = profile

        var components = URLComponents(url: profile.baseURL, resolvingAgainstBaseURL: false)
        components?.scheme = profile.baseURL.scheme == "https" ? "wss" : "ws"
        components?.path = "/ws"
        components?.queryItems = [URLQueryItem(name: "key", value: profile.key)]
        guard let url = components?.url else { throw ClientError.invalidAddress }

        var request = URLRequest(url: url)
        request.setValue(profile.key, forHTTPHeaderField: "X-Grok-Remote-Key")
        let task = URLSession.shared.webSocketTask(with: request)
        socket = task
        task.resume()
        receiveTask = Task { [weak self] in await self?.receiveLoop() }

        let raw = try await rpc("initialize", params: [
            "protocolVersion": 1,
            "clientInfo": ["name": "grok-remote-app-ios", "version": "0.1.0"],
            "clientCapabilities": [:]
        ], timeout: 20)
        return raw as? [String: Any] ?? [:]
    }

    func disconnect(notify: Bool = true) {
        receiveTask?.cancel()
        receiveTask = nil
        socket?.cancel(with: .goingAway, reason: nil)
        socket = nil
        let continuations = Array(pending.values)
        pending.removeAll()
        continuations.forEach { $0.resume(throwing: ClientError.disconnected) }
        if notify { onDisconnect?(nil) }
    }

    func rpc(_ method: String, params: [String: Any], timeout: TimeInterval = 120) async throws -> Any {
        guard let socket, socket.state == .running else { throw ClientError.disconnected }
        nextID += 1
        let id = nextID
        let payload: [String: Any] = ["jsonrpc": "2.0", "id": id, "method": method, "params": params]

        return try await withCheckedThrowingContinuation { continuation in
            pending[id] = continuation
            Task { [weak self] in
                do {
                    try await self?.send(payload)
                    try await Task.sleep(nanoseconds: UInt64(timeout * 1_000_000_000))
                    guard let self, let continuation = self.pending.removeValue(forKey: id) else { return }
                    continuation.resume(throwing: ClientError.server("请求超时：\(method)"))
                } catch {
                    guard let self, let continuation = self.pending.removeValue(forKey: id) else { return }
                    continuation.resume(throwing: error)
                }
            }
        }
    }

    func reply(id: Int, result: [String: Any]) async throws {
        try await send(["jsonrpc": "2.0", "id": id, "result": result])
    }

    func notify(_ method: String, params: [String: Any]) async throws {
        try await send(["jsonrpc": "2.0", "method": method, "params": params])
    }

    func rest(path: String, method: String = "GET", query: [URLQueryItem] = [], body: [String: Any]? = nil) async throws -> [String: Any] {
        guard let profile else { throw ClientError.disconnected }
        guard var components = URLComponents(url: profile.baseURL, resolvingAgainstBaseURL: false) else {
            throw ClientError.invalidAddress
        }
        components.path = path
        components.queryItems = query.isEmpty ? nil : query
        guard let url = components.url else { throw ClientError.invalidAddress }

        var request = URLRequest(url: url)
        request.httpMethod = method
        request.setValue(profile.key, forHTTPHeaderField: "X-Grok-Remote-Key")
        request.setValue("application/json", forHTTPHeaderField: "Accept")
        if let body {
            request.setValue("application/json", forHTTPHeaderField: "Content-Type")
            request.httpBody = try JSONSerialization.data(withJSONObject: body)
        }
        let (data, response) = try await URLSession.shared.data(for: request)
        guard let http = response as? HTTPURLResponse else { throw ClientError.malformedResponse }
        let object = (try? JSONSerialization.jsonObject(with: data)) as? [String: Any] ?? [:]
        guard (200..<300).contains(http.statusCode) else {
            throw ClientError.server(object.string("error", "message") ?? "HTTP \(http.statusCode)")
        }
        return object
    }

    private func send(_ payload: [String: Any]) async throws {
        guard let socket else { throw ClientError.disconnected }
        let data = try JSONSerialization.data(withJSONObject: payload)
        guard let text = String(data: data, encoding: .utf8) else { throw ClientError.malformedResponse }
        try await socket.send(.string(text))
    }

    private func receiveLoop() async {
        do {
            while !Task.isCancelled, let socket {
                let message = try await socket.receive()
                let data: Data
                switch message {
                case .string(let text): data = Data(text.utf8)
                case .data(let binary): data = binary
                @unknown default: continue
                }
                guard let object = try JSONSerialization.jsonObject(with: data) as? [String: Any] else { continue }
                handle(object)
            }
        } catch {
            guard !Task.isCancelled else { return }
            socket = nil
            let continuations = Array(pending.values)
            pending.removeAll()
            continuations.forEach { $0.resume(throwing: error) }
            onDisconnect?(error)
        }
    }

    private func handle(_ object: [String: Any]) {
        if let id = (object["id"] as? NSNumber)?.intValue,
           object["method"] == nil,
           let continuation = pending.removeValue(forKey: id) {
            if let error = object["error"] as? [String: Any] {
                continuation.resume(throwing: ClientError.server(error.string("message") ?? "RPC error"))
            } else {
                continuation.resume(returning: object["result"] ?? [:])
            }
            return
        }
        onNotification?(object)
    }
}
