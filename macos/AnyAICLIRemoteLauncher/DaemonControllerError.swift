import Foundation

enum ControllerError: LocalizedError {
  case daemonMissing
  case policyMissing
  case stopTimeout
  case secretFileCleanup
  case exited(Int32)

  var errorDescription: String? {
    switch self {
    case .daemonMissing:
      return "找不到 daemon"
    case .policyMissing:
      return "Launcher policy 未加载"
    case .stopTimeout:
      return "daemon 未能在限定时间内停止"
    case .secretFileCleanup:
      return "无法删除旧的 daemon 临时密钥文件"
    case .exited(let status):
      return "daemon 异常退出（状态 \(status)）"
    }
  }
}
