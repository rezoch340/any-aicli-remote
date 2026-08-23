import Foundation

final class PendingUserEchoTracker {
    private var expectedText: String?
    private var consumedLength = 0

    func begin(text: String) { expectedText = text; consumedLength = 0 }

    func consume(chunk: String) -> Bool {
        guard let expected = expectedText else { return false }
        if chunk.isEmpty { return true }
        let expectedCharacters = Array(expected)
        let chunkCharacters = Array(chunk)
        let consumed = min(consumedLength, expectedCharacters.count)
        let remaining = String(expectedCharacters.dropFirst(consumed))
        let advance: Int
        if chunk == expected { advance = expectedCharacters.count - consumed }
        else if remaining.hasPrefix(chunk) { advance = chunkCharacters.count }
        else if expected.hasPrefix(chunk) { advance = chunkCharacters.count - consumed }
        else { advance = overlapAdvance(expectedCharacters, chunkCharacters, consumed: consumed) }
        guard advance >= 0 else { clear(); return false }
        consumedLength = min(consumed + advance, expectedCharacters.count)
        if consumedLength == expectedCharacters.count { clear() }
        return true
    }

    func clear() { expectedText = nil; consumedLength = 0 }

    private func overlapAdvance(_ expected: [Character], _ chunk: [Character], consumed: Int) -> Int {
        let consumedCharacters = Array(expected.prefix(consumed))
        guard !chunk.isEmpty else { return 0 }
        for overlap in stride(from: min(consumedCharacters.count, chunk.count), through: 1, by: -1) {
            if Array(consumedCharacters.suffix(overlap)) == Array(chunk.prefix(overlap)) &&
                Array(expected.dropFirst(consumed).prefix(chunk.count - overlap)) == Array(chunk.dropFirst(overlap)) {
                return chunk.count - overlap
            }
        }
        return -1
    }
}
