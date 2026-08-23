import Foundation

struct LifecycleGeneration: Equatable {
  private(set) var value: UInt64 = 0

  mutating func next() -> UInt64 {
    value &+= 1
    return value
  }

  func accepts(_ candidate: UInt64) -> Bool {
    candidate == value
  }
}
