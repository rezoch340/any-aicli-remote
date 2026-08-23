import Foundation

struct ClientRuntimeConfiguration: Equatable, Sendable {
    var initializeTimeout: TimeInterval = 20
    var rpcTimeout: TimeInterval = 120
    var sessionLoadTimeout: TimeInterval = 90
    var sessionCreateTimeout: TimeInterval = 60
    var healthRequestTimeout: TimeInterval = 3
    var healthPollingInterval: TimeInterval = 5
}
