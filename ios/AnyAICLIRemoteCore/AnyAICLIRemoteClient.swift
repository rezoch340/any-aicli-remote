import Foundation

@MainActor
public final class AnyAICLIRemoteClient {
  private let runtimeConfiguration: ClientRuntimeConfiguration
  public var onNotification: (([String: Any]) -> Void)?
  public var onDisconnect: ((Error?) -> Void)?

  private var profile: ServerProfile?
  private var socket: URLSessionWebSocketTask?
  private var receiveTask: Task<Void, Never>?
  private var pending: [Int: CheckedContinuation<MainActorJSONValue, Error>] = [:]
  private var nextID = 0
  private var connectionID = UUID()
  public var isConnected: Bool { socket?.state == .running }

  public init(runtimeConfiguration: ClientRuntimeConfiguration = ClientRuntimeConfiguration()) {
    self.runtimeConfiguration = runtimeConfiguration
  }

  public func connect(profile: ServerProfile) async throws -> [String: Any] {
    disconnect(notify: false)
    self.profile = profile

    let request = try Self.websocketRequest(baseURL: profile.baseURL, key: profile.key)
    let task = URLSession.shared.webSocketTask(with: request)
    let connectionID = UUID()
    self.connectionID = connectionID
    socket = task
    task.resume()
    receiveTask = Task { [weak self] in
      await self?.receiveLoop(socket: task, connectionID: connectionID)
    }

    let initializeResponse = try await rpc(
      "initialize",
      params: [
        "protocolVersion": 1,
        "clientInfo": [
          "name": ProductIdentifiers.clientName,
          "version": ProductIdentifiers.clientVersion
        ],
        "clientCapabilities": [
          "fs": ["readTextFile": true, "writeTextFile": true],
          "terminal": true
        ]
      ], timeout: runtimeConfiguration.initializeTimeout)
    return initializeResponse as? [String: Any] ?? [:]
  }

  /// Builds the authenticated WebSocket request without putting credentials in its URL.
  nonisolated static func websocketRequest(baseURL: URL, key: String) throws -> URLRequest {
    guard var components = URLComponents(url: baseURL, resolvingAgainstBaseURL: false) else {
      throw ClientError.invalidAddress
    }
    components.scheme = baseURL.scheme?.lowercased() == "https" ? "wss" : "ws"
    components.path = "/ws"
    components.query = nil
    components.fragment = nil
    guard let url = components.url else { throw ClientError.invalidAddress }

    var request = URLRequest(url: url)
    ProductIdentifiers.authorize(&request, key: key)
    return request
  }

  public func disconnect(notify: Bool = true) {
    connectionID = UUID()
    receiveTask?.cancel()
    receiveTask = nil
    socket?.cancel(with: .goingAway, reason: nil)
    socket = nil
    profile = nil
    let continuations = Array(pending.values)
    pending.removeAll()
    continuations.forEach { $0.resume(throwing: ClientError.disconnected) }
    if notify { onDisconnect?(nil) }
  }

  public func rpc(_ method: String, params: [String: Any], timeout: TimeInterval? = nil) async throws
    -> Any {
    guard let socket, socket.state == .running else { throw ClientError.disconnected }
    let requestConnectionID = connectionID
    nextID += 1
    let id = nextID
    let payload: [String: Any] = ["jsonrpc": "2.0", "id": id, "method": method, "params": params]

    let effectiveTimeout = timeout ?? runtimeConfiguration.rpcTimeout
    let transferredResult: MainActorJSONValue = try await withTaskCancellationHandler {
      try await withCheckedThrowingContinuation { continuation in
        pending[id] = continuation
        Task { [weak self] in
          do {
            try await self?.send(payload, socket: socket, connectionID: requestConnectionID)
            try await Task.sleep(nanoseconds: UInt64(effectiveTimeout * 1_000_000_000))
            guard let self, let continuation = self.pending.removeValue(forKey: id) else { return }
            continuation.resume(throwing: ClientError.server("请求超时：\(method)"))
          } catch {
            guard let self, let continuation = self.pending.removeValue(forKey: id) else { return }
            continuation.resume(throwing: error)
          }
        }
      }
    } onCancel: {
      Task { @MainActor [weak self] in
        self?.cancelPendingRequest(id: id)
      }
    }
    return transferredResult.value
  }

  public func reply(id: Int, result: [String: Any]) async throws {
    try await send(["jsonrpc": "2.0", "id": id, "result": result])
  }

  public func notify(_ method: String, params: [String: Any]) async throws {
    try await send(["jsonrpc": "2.0", "method": method, "params": params])
  }

  public func rest(
    path: String, method: String = "GET", query: [URLQueryItem] = [], body: [String: Any]? = nil
  ) async throws -> [String: Any] {
    try await rest(
      resolvedPath: path,
      pathIsPercentEncoded: false,
      method: method,
      query: query,
      body: body
    )
  }

  public func rest(
    pathComponents: [String], method: String = "GET", query: [URLQueryItem] = [],
    body: [String: Any]? = nil
  ) async throws -> [String: Any] {
    return try await rest(
      resolvedPath: try Self.percentEncodedPath(components: pathComponents),
      pathIsPercentEncoded: true,
      method: method,
      query: query,
      body: body
    )
  }

  nonisolated static func percentEncodedPath(components: [String]) throws -> String {
    let unreservedCharacters = CharacterSet(
      charactersIn: "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-._~"
    )
    let encodedComponents = try components.map { component in
      guard
        let encodedComponent = component.addingPercentEncoding(
          withAllowedCharacters: unreservedCharacters
        )
      else {
        throw ClientError.invalidAddress
      }
      return encodedComponent
    }
    return "/" + encodedComponents.joined(separator: "/")
  }

  private func rest(
    resolvedPath: String,
    pathIsPercentEncoded: Bool,
    method: String,
    query: [URLQueryItem],
    body: [String: Any]?
  ) async throws -> [String: Any] {
    guard let profile else { throw ClientError.disconnected }
    let requestConnectionID = connectionID
    guard var components = URLComponents(url: profile.baseURL, resolvingAgainstBaseURL: false)
    else {
      throw ClientError.invalidAddress
    }
    if pathIsPercentEncoded {
      components.percentEncodedPath = resolvedPath
    } else {
      components.path = resolvedPath
    }
    components.queryItems = query.isEmpty ? nil : query
    guard let url = components.url else { throw ClientError.invalidAddress }

    var request = URLRequest(url: url)
    request.httpMethod = method
    ProductIdentifiers.authorize(&request, key: profile.key)
    request.setValue("application/json", forHTTPHeaderField: "Accept")
    if let body {
      request.setValue("application/json", forHTTPHeaderField: "Content-Type")
      request.httpBody = try JSONSerialization.data(withJSONObject: body)
    }
    let (data, response) = try await URLSession.shared.data(for: request)
    guard connectionID == requestConnectionID else { throw ClientError.disconnected }
    guard let http = response as? HTTPURLResponse else { throw ClientError.malformedResponse }
    let object = (try? JSONSerialization.jsonObject(with: data)) as? [String: Any] ?? [:]
    guard (200..<300).contains(http.statusCode) else {
      throw ClientError.server(object.string("error", "message") ?? "HTTP \(http.statusCode)")
    }
    return object
  }

  private func send(_ payload: [String: Any]) async throws {
    guard let socket else { throw ClientError.disconnected }
    try await send(payload, socket: socket, connectionID: connectionID)
  }

  private func cancelPendingRequest(id: Int) {
    pending.removeValue(forKey: id)?.resume(throwing: CancellationError())
  }

  private func send(
    _ payload: [String: Any],
    socket: URLSessionWebSocketTask,
    connectionID: UUID
  ) async throws {
    guard self.connectionID == connectionID, self.socket === socket else {
      throw ClientError.disconnected
    }
    let data = try JSONSerialization.data(withJSONObject: payload)
    guard let text = String(data: data, encoding: .utf8) else {
      throw ClientError.malformedResponse
    }
    try await socket.send(.string(text))
  }

  private func receiveLoop(socket: URLSessionWebSocketTask, connectionID: UUID) async {
    do {
      while !Task.isCancelled {
        let message = try await socket.receive()
        guard self.connectionID == connectionID, self.socket === socket else { return }
        let data: Data
        switch message {
        case .string(let text): data = Data(text.utf8)
        case .data(let binary): data = binary
        @unknown default: continue
        }
        guard let object = try JSONSerialization.jsonObject(with: data) as? [String: Any] else {
          continue
        }
        if let method = object["method"] as? String,
          Self.incomingRequestCategory(object) == .unknown {
          let error = Self.methodNotFoundResponse(id: object["id"]!, method: method)
          try await send(error, socket: socket, connectionID: connectionID)
          continue
        }
        handle(object)
      }
    } catch {
      guard !Task.isCancelled,
        self.connectionID == connectionID,
        self.socket === socket
      else { return }
      self.socket = nil
      let continuations = Array(pending.values)
      pending.removeAll()
      continuations.forEach { $0.resume(throwing: error) }
      onDisconnect?(error)
    }
  }

  enum IncomingRequestCategory: Equatable { case permission, unknown, notification }

  nonisolated static func incomingRequestCategory(_ method: String) -> IncomingRequestCategory {
    if method.contains("permission") || method.contains("ask_user") { return .permission }
    return .unknown
  }

  nonisolated static func incomingRequestCategory(_ object: [String: Any])
    -> IncomingRequestCategory {
    guard let method = object["method"] as? String else { return .notification }
    guard object["id"] != nil else { return .notification }
    return incomingRequestCategory(method)
  }

  nonisolated static func methodNotFoundResponse(id: Any, method: String) -> [String: Any] {
    [
      "jsonrpc": "2.0", "id": id,
      "error": ["code": -32601, "message": "Method not found: \(method)"]
    ]
  }

  private func handle(_ object: [String: Any]) {
    if let id = (object["id"] as? NSNumber)?.intValue,
      object["method"] == nil,
      let continuation = pending.removeValue(forKey: id) {
      if let error = object["error"] as? [String: Any] {
        continuation.resume(throwing: ClientError.server(error.string("message") ?? "RPC error"))
      } else {
        continuation.resume(returning: MainActorJSONValue(value: object["result"] ?? [:]))
      }
      return
    }
    onNotification?(object)
  }
}

/// JSONSerialization values remain confined to AnyAICLIRemoteClient's main actor.
private struct MainActorJSONValue: @unchecked Sendable {
  public let value: Any
}
