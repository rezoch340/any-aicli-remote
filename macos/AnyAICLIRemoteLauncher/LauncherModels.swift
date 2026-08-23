import Foundation

enum DaemonPhase: Equatable {
  case stopped
  case starting
  case online
  case degraded
  case stopping
  case failed(String)

  var title: String {
    switch self {
    case .stopped: "未运行"
    case .starting: "正在启动"
    case .online: "服务就绪"
    case .degraded: "Agent 连接中"
    case .stopping: "正在停止"
    case .failed: "启动失败"
    }
  }

  var detail: String {
    switch self {
    case .stopped: "配置完成后即可启动"
    case .starting: "等待 daemon 响应健康检查"
    case .online: "手机现在可以扫码连接"
    case .degraded: "daemon 在线，正在等待 Provider Agent"
    case .stopping: "正在安全关闭服务与 Agent"
    case .failed(let message): message
    }
  }
}

struct HealthSnapshot: Decodable, Equatable {
  let isHealthy: Bool
  let ready: Bool?
  let hubClients: Int?
  let hubUp: Bool?
  let hubError: String?
  let agentListening: Bool?

  enum CodingKeys: String, CodingKey {
    case isHealthy = "ok"
    case ready
    case hubClients = "hub_clients"
    case hubUp = "hub_up"
    case hubError = "hub_err"
    case agentListening = "agent_listening"
  }
}

struct RuntimeConfiguration: Decodable, Equatable {
  let pairingURL: String?
  let pairingDeepLink: String?
  let lanAddress: String?

  enum CodingKeys: String, CodingKey {
    case pairingURL = "pairing_url"
    case pairingDeepLink = "pairing_deep_link"
    case lanAddress = "lan_ip"
  }
}

struct LogEntry: Identifiable, Equatable {
  let id = UUID()
  let date: Date
  let message: String
}
