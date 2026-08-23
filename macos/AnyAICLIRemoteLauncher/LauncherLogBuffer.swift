import Foundation

struct LauncherLogBuffer {
  private(set) var entries = [LogEntry]()
  private var remainder = ""
  let maximumChunkCharacters: Int
  let maximumEntries: Int
  let redactedValues: () -> [String]

  init(
    maximumChunkCharacters: Int,
    maximumEntries: Int,
    redactedValues: @escaping () -> [String]
  ) {
    self.maximumChunkCharacters = maximumChunkCharacters
    self.maximumEntries = maximumEntries
    self.redactedValues = redactedValues
  }

  mutating func appendChunk(_ chunk: String) {
    remainder += chunk
    remainder = redactFullValues(remainder)
    var lines = remainder.components(separatedBy: "\n")
    remainder = lines.removeLast()
    for line in lines where !line.isEmpty {
      append(line)
    }
    emitOverflow()
  }

  mutating func flush() {
    if !remainder.isEmpty {
      append(remainder)
      remainder = ""
    }
  }

  mutating func clear() {
    entries.removeAll()
    remainder = ""
  }

  mutating func append(_ message: String) {
    let safeMessage = redactFullValues(message)
    entries.append(LogEntry(date: Date(), message: safeMessage))
    if entries.count > maximumEntries {
      entries.removeFirst(entries.count - maximumEntries)
    }
  }

  private mutating func emitOverflow() {
    let maximumSecretLength = sensitiveValues().map(\.count).max() ?? 0
    let retainedLimit = maximumChunkCharacters + max(0, maximumSecretLength - 1)
    while remainder.count > retainedLimit {
      let protectedSuffixLength = longestProtectedSuffixLength()
      let safeAreaLength = remainder.count - protectedSuffixLength
      let emitCount = min(maximumChunkCharacters, safeAreaLength)
      guard emitCount > 0 else { return }
      append(String(remainder.prefix(emitCount)))
      remainder.removeFirst(emitCount)
    }
  }

  private func redactFullValues(_ message: String) -> String {
    var result = message
    for value in sensitiveValues() {
      result = result.replacingOccurrences(of: value, with: "••••••••")
    }
    return result
  }

  private func sensitiveValues() -> [String] {
    redactedValues().filter { !$0.isEmpty }
  }

  private func longestProtectedSuffixLength() -> Int {
    var longest = 0
    for value in sensitiveValues() where value.count > 1 {
      for length in 1..<value.count {
        let prefix = String(value.prefix(length))
        if remainder.hasSuffix(prefix) {
          longest = max(longest, length)
        }
      }
    }
    return longest
  }
}
