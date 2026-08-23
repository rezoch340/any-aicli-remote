import Foundation

enum DaemonHTTPError: LocalizedError {
    case invalidResponse
    case status(Int)

    var errorDescription: String? {
        switch self {
        case .invalidResponse:
            return "Daemon HTTP 响应无效"
        case .status(let code):
            return "Daemon HTTP 状态无效：\(code)"
        }
    }
}

struct DaemonHTTPClient {
    private enum HTTPMethod {
        static let get = "GET"
        static let post = "POST"
    }

    private enum HTTPHeader {
        static let contentType = "Content-Type"
        static let json = "application/json"
    }

    private enum StopRequest {
        static let body = "{\"keep_agent\":false}"
    }

    let endpoint: LocalDaemonEndpoint
    let pairingSecret: String
    let session: URLSession

    init(
        endpoint: LocalDaemonEndpoint,
        pairingSecret: String,
        policy: LauncherPolicy,
        session: URLSession? = nil
    ) {
        self.endpoint = endpoint
        self.pairingSecret = pairingSecret
        if let session {
            self.session = session
        } else {
            let configuration = URLSessionConfiguration.ephemeral
            configuration.timeoutIntervalForRequest = policy.requestTimeoutSeconds
            configuration.timeoutIntervalForResource = policy.resourceTimeoutSeconds
            self.session = URLSession(configuration: configuration)
        }
    }

    func healthRequest() -> URLRequest {
        makeRequest(path: LocalDaemonEndpoint.healthPath, method: HTTPMethod.get, authenticated: false)
    }

    func configurationRequest() -> URLRequest {
        makeRequest(path: LocalDaemonEndpoint.configurationPath, method: HTTPMethod.get, authenticated: true)
    }

    func stopRequest() -> URLRequest {
        var request = makeRequest(path: LocalDaemonEndpoint.stopPath, method: HTTPMethod.post, authenticated: true)
        request.setValue(HTTPHeader.json, forHTTPHeaderField: HTTPHeader.contentType)
        request.httpBody = Data(StopRequest.body.utf8)
        return request
    }

    func health() async throws -> HealthSnapshot {
        try await decode(healthRequest())
    }

    func configuration() async throws -> RuntimeConfiguration {
        try await decode(configurationRequest())
    }

    func stop() async throws {
        _ = try await perform(stopRequest())
    }

    private func makeRequest(path: String, method: String, authenticated: Bool) -> URLRequest {
        var request = URLRequest(url: endpoint.url(path: path))
        request.httpMethod = method
        if authenticated {
            request.setValue(pairingSecret, forHTTPHeaderField: ProductIdentifier.authenticationHeaderName)
        }
        return request
    }

    private func decode<Value: Decodable>(_ request: URLRequest) async throws -> Value {
        try JSONDecoder().decode(Value.self, from: try await perform(request))
    }

    private func perform(_ request: URLRequest) async throws -> Data {
        let (data, response) = try await session.data(for: request)
        guard let httpResponse = response as? HTTPURLResponse else {
            throw DaemonHTTPError.invalidResponse
        }
        guard (200..<300).contains(httpResponse.statusCode) else {
            throw DaemonHTTPError.status(httpResponse.statusCode)
        }
        return data
    }
}
