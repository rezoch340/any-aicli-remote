import Foundation

enum DaemonLaunchEnvironment {
    private static let scrubbedNames: Set<String> = [
        "ANY_AI_CLI_REMOTE_CONFIG",
        "ANY_AI_CLI_REMOTE_BIND",
        "ANY_AI_CLI_REMOTE_PORT",
        "ANY_AI_CLI_REMOTE_AGENT_HOST",
        "ANY_AI_CLI_REMOTE_AGENT_PORT",
        "ANY_AI_CLI_REMOTE_PAIRING_SECRET",
        "ANY_AI_CLI_REMOTE_PAIRING_SECRET_FILE",
        "ANY_AI_CLI_REMOTE_AGENT_SECRET",
        "ANY_AI_CLI_REMOTE_AGENT_SECRET_FILE",
        "ANY_AI_CLI_REMOTE_RUNTIME_DIR",
        "ANY_AI_CLI_REMOTE_PUBLIC_HOST",
        "ANY_AI_CLI_REMOTE_PROVIDER",
        "ANY_AI_CLI_REMOTE_PROVIDER_PATH",
        "ANY_AI_CLI_REMOTE_DATA_DIR",
        "ANY_AI_CLI_REMOTE_ENSURE_AGENT",
        "ANY_AI_CLI_REMOTE_STOP_AGENT_ON_EXIT",
        "ANY_AI_CLI_REMOTE_PROVIDER_SESSIONS_DIR",
        "ANY_AI_CLI_REMOTE_PROVIDER_ALWAYS_APPROVE",
        "ANY_AI_CLI_REMOTE_PROVIDER_LEADER",
        "ANY_AI_CLI_REMOTE_GROK_SESSIONS_DIR",
        "ANY_AI_CLI_REMOTE_GROK_ALWAYS_APPROVE",
        "ANY_AI_CLI_REMOTE_GROK_LEADER",
        "GROK_REMOTE_BIND",
        "GROK_REMOTE_PORT",
        "GROK_REMOTE_AGENT_HOST",
        "GROK_REMOTE_AGENT_PORT",
        "GROK_REMOTE_SECRET_FILE",
        "GROK_REMOTE_RUNTIME_DIR",
        "GROK_REMOTE_PUBLIC_HOST",
        "GROK_REMOTE_PROVIDER",
        "GROK_REMOTE_GROK_PATH",
        "GROK_REMOTE_SESSIONS_DIR",
        "GROK_REMOTE_ENSURE_AGENT",
        "GROK_REMOTE_STOP_AGENT_ON_EXIT",
        "GROK_REMOTE_ALWAYS_APPROVE",
        "GROK_REMOTE_LEADER",
        "GROK_REMOTE_CWD",
        "GROK_PLUGIN_DATA"
    ]

    static func inheritedSanitized(
        _ environment: [String: String] = ProcessInfo.processInfo.environment
    ) -> [String: String] {
        environment.filter { !scrubbedNames.contains($0.key) }
    }
}
