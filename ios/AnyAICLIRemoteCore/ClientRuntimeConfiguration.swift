import Foundation

public struct ClientRuntimeConfiguration: Equatable, Sendable {
    public let initializeTimeout: TimeInterval
    public let rpcTimeout: TimeInterval
    public let sessionLoadTimeout: TimeInterval
    public let sessionCreateTimeout: TimeInterval
    public let healthRequestTimeout: TimeInterval
    public let healthPollingInterval: TimeInterval
    public let webSocketMaximumMessageBytes: Int

    public init(
        initializeTimeout: TimeInterval = 20,
        rpcTimeout: TimeInterval = 120,
        sessionLoadTimeout: TimeInterval = 90,
        sessionCreateTimeout: TimeInterval = 60,
        healthRequestTimeout: TimeInterval = 3,
        healthPollingInterval: TimeInterval = 5,
        webSocketMaximumMessageBytes: Int = 64 << 20
    ) {
        self.initializeTimeout = initializeTimeout
        self.rpcTimeout = rpcTimeout
        self.sessionLoadTimeout = sessionLoadTimeout
        self.sessionCreateTimeout = sessionCreateTimeout
        self.healthRequestTimeout = healthRequestTimeout
        self.healthPollingInterval = healthPollingInterval
        self.webSocketMaximumMessageBytes = webSocketMaximumMessageBytes
    }
}
