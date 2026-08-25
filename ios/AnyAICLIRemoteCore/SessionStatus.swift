import Foundation

/// Neutral phase of a request retry reported by the daemon.
public enum RetryPhase: String, Equatable {
  case retrying
  case exhausted
  case failed
}

/// Provider-neutral view of a request retry in progress.
public struct RetryStatus: Equatable {
  public let phase: RetryPhase
  public let attempt: Int
  public let maxRetries: Int
  public let reason: String
  public let rateLimit: Bool

  public init(phase: RetryPhase, attempt: Int = 0, maxRetries: Int = 0, reason: String = "", rateLimit: Bool = false) {
    self.phase = phase
    self.attempt = attempt
    self.maxRetries = maxRetries
    self.reason = reason
    self.rateLimit = rateLimit
  }
}

/// Provider-neutral view of an automatic model switch.
public struct ModelSwitch: Equatable {
  public let previous: String
  public let current: String
  public let reason: String

  public init(previous: String = "", current: String = "", reason: String = "") {
    self.previous = previous
    self.current = current
    self.reason = reason
  }
}

/// A provider-neutral, transient session status update. Exactly one of `retry`
/// or `modelSwitch` is populated. Display-only; never parses a provider wire.
public struct SessionStatusUpdate: Equatable {
  public let retry: RetryStatus?
  public let modelSwitch: ModelSwitch?

  public init(retry: RetryStatus? = nil, modelSwitch: ModelSwitch? = nil) {
    self.retry = retry
    self.modelSwitch = modelSwitch
  }
}
