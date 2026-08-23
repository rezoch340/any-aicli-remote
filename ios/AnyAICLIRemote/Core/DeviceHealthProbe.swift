import Foundation

struct DeviceHealthProbe: Sendable {
    private let session: URLSession
    private let runtimeConfiguration: ClientRuntimeConfiguration

    init(session: URLSession = .shared, runtimeConfiguration: ClientRuntimeConfiguration = ClientRuntimeConfiguration()) {
        self.session = session
        self.runtimeConfiguration = runtimeConfiguration
    }

    func isOnline(baseURL: URL) async -> Bool {
        guard let healthURL = Self.healthURL(baseURL: baseURL) else { return false }
        var request = URLRequest(
            url: healthURL,
            cachePolicy: .reloadIgnoringLocalCacheData,
            timeoutInterval: runtimeConfiguration.healthRequestTimeout
        )
        request.httpMethod = "GET"
        request.setValue("application/json", forHTTPHeaderField: "Accept")
        do {
            let (_, response) = try await session.data(for: request)
            guard let httpResponse = response as? HTTPURLResponse else { return false }
            return (200..<300).contains(httpResponse.statusCode)
        } catch {
            return false
        }
    }

    static func healthURL(baseURL: URL) -> URL? {
        guard var components = URLComponents(url: baseURL, resolvingAgainstBaseURL: false) else {
            return nil
        }
        components.path = "/health"
        components.query = nil
        components.fragment = nil
        return components.url
    }
}
