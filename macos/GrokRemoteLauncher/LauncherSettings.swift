import Combine
import Foundation

@MainActor
final class LauncherSettings: ObservableObject {
    private enum Key {
        static let workspace = "workspace"
        static let daemonPort = "daemonPort"
        static let agentPort = "agentPort"
        static let bindAddress = "bindAddress"
        static let publicHost = "publicHost"
        static let lastLANAddress = "lastLANAddress"
    }

    private let defaults: UserDefaults

    @Published var workspace: String { didSet { defaults.set(workspace, forKey: Key.workspace) } }
    @Published var daemonPort: Int { didSet { defaults.set(daemonPort, forKey: Key.daemonPort) } }
    @Published var agentPort: Int { didSet { defaults.set(agentPort, forKey: Key.agentPort) } }
    @Published var bindAddress: String { didSet { defaults.set(bindAddress, forKey: Key.bindAddress) } }
    @Published var publicHost: String { didSet { defaults.set(publicHost, forKey: Key.publicHost) } }
    var lastLANAddress: String { didSet { defaults.set(lastLANAddress, forKey: Key.lastLANAddress) } }

    init(defaults: UserDefaults = .standard) {
        self.defaults = defaults
        workspace = defaults.string(forKey: Key.workspace) ?? FileManager.default.homeDirectoryForCurrentUser.path
        daemonPort = defaults.object(forKey: Key.daemonPort) as? Int ?? 2421
        agentPort = defaults.object(forKey: Key.agentPort) as? Int ?? 2419
        bindAddress = defaults.string(forKey: Key.bindAddress) ?? "0.0.0.0"
        publicHost = defaults.string(forKey: Key.publicHost) ?? ""
        lastLANAddress = defaults.string(forKey: Key.lastLANAddress) ?? ""
    }
}
