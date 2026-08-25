import AnyAICLIRemoteCore
import Foundation

/// Renders a neutral session status update into a short human notice.
enum SessionStatusFormatter {
  static func notice(_ status: SessionStatusUpdate) -> String {
    if let retry = status.retry { return retryNotice(retry) }
    if let modelSwitch = status.modelSwitch { return modelSwitchNotice(modelSwitch) }
    return ""
  }

  private static func retryNotice(_ retry: RetryStatus) -> String {
    switch retry.phase {
    case .retrying:
      let progress = retry.maxRetries > 0 ? "\(retry.attempt)/\(retry.maxRetries)" : "\(retry.attempt)"
      return "正在重试 \(progress)\(reasonSuffix(retry.reason))"
    case .exhausted:
      return retry.rateLimit ? "已达速率上限，重试耗尽" : "重试已耗尽\(reasonSuffix(retry.reason))"
    case .failed:
      return "请求失败\(reasonSuffix(retry.reason))"
    }
  }

  private static func modelSwitchNotice(_ modelSwitch: ModelSwitch) -> String {
    let target = modelSwitch.current.isEmpty ? "其他模型" : modelSwitch.current
    return "已自动切换到 \(target)\(reasonSuffix(modelSwitch.reason))"
  }

  private static func reasonSuffix(_ reason: String) -> String {
    let trimmed = reason.trimmingCharacters(in: .whitespacesAndNewlines)
    return trimmed.isEmpty ? "" : "：\(trimmed)"
  }
}
