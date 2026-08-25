import Foundation

/// Decodes the neutral `session/status_update` payload. Returns nil when the
/// payload carries neither a retry nor a model switch.
public enum SessionStatusPayloadMapper {
  public static func status(from params: [String: Any]) -> SessionStatusUpdate? {
    let retry = params.object("retry").map(retryStatus)
    let modelSwitch = params.object("modelSwitch").map { object in
      ModelSwitch(
        previous: string(object, "previous"),
        current: string(object, "current"),
        reason: string(object, "reason")
      )
    }
    if retry == nil && modelSwitch == nil { return nil }
    return SessionStatusUpdate(retry: retry, modelSwitch: modelSwitch)
  }

  private static func retryStatus(_ object: [String: Any]) -> RetryStatus {
    RetryStatus(
      phase: RetryPhase(rawValue: string(object, "phase").lowercased()) ?? .retrying,
      attempt: Int(integer(object["attempt"])),
      maxRetries: Int(integer(object["maxRetries"])),
      reason: string(object, "reason"),
      rateLimit: (object["rateLimit"] as? Bool) ?? false
    )
  }

  private static func string(_ object: [String: Any], _ key: String) -> String {
    (object[key] as? String) ?? ""
  }

  private static func integer(_ value: Any?) -> Int64 {
    (value as? NSNumber)?.int64Value ?? Int64((value as? String) ?? "") ?? 0
  }
}
