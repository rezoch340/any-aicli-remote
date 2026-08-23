import Foundation

struct LocalDaemonEndpoint: Equatable {
    static let healthPath = "/health"
    static let configurationPath = "/config.json"
    static let statusPath = "/api/stack/status"
    static let stopPath = "/api/stack/stop"

    let url: URL

    init(bindAddress: String, port: Int) throws {
        guard (1...65535).contains(port) else {
            throw EndpointError.invalidPort(port)
        }
        let trimmedAddress = bindAddress.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !(!bindAddress.isEmpty && trimmedAddress.isEmpty) else {
            throw EndpointError.invalidHost(bindAddress)
        }
        let normalizedHost: String
        if trimmedAddress.isEmpty || trimmedAddress == "0.0.0.0" {
            normalizedHost = "127.0.0.1"
        } else if trimmedAddress == "::" || trimmedAddress == "[::]" {
            normalizedHost = "::1"
        } else {
            normalizedHost = trimmedAddress.trimmingCharacters(in: CharacterSet(charactersIn: "[]"))
        }
        guard !normalizedHost.isEmpty else {
            throw EndpointError.invalidHost(bindAddress)
        }

        var components = URLComponents()
        components.scheme = "http"
        components.host = normalizedHost
        components.port = port
        if let endpointURL = components.url {
            url = endpointURL
        } else if normalizedHost.contains(":") {
            let endpointText = "http://[" + normalizedHost + "]:" + String(port)
            guard let endpointURL = URL(string: endpointText) else {
                throw EndpointError.invalidHost(bindAddress)
            }
            url = endpointURL
        } else {
            throw EndpointError.invalidHost(bindAddress)
        }
    }

    func url(path: String) -> URL {
        url.appendingPathComponent(String(path.drop(while: { $0 == "/" })))
    }

    enum EndpointError: LocalizedError {
        case invalidPort(Int)
        case invalidHost(String)

        var errorDescription: String? {
            switch self {
            case .invalidPort(let value):
                return "端口无效：\(value)"
            case .invalidHost(let value):
                return "地址无效：\(value)"
            }
        }
    }
}
