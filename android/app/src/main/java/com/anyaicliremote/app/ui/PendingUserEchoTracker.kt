package com.anyaicliremote.app.ui

internal class PendingUserEchoTracker {
    private var expectedText: String? = null
    private var consumedLength: Int = 0

    fun begin(text: String) {
        expectedText = text
        consumedLength = 0
    }

    fun consume(chunk: String): Boolean {
        val expected = expectedText ?: return false
        if (chunk.isEmpty()) return true
        val remaining = expected.substring(consumedLength)
        var advance = when {
            chunk == expected -> expected.length - consumedLength
            remaining.startsWith(chunk) -> chunk.length
            expected.startsWith(chunk) -> chunk.length - consumedLength
            else -> overlapAdvance(expected, chunk)
        }
        if (advance < 0) {
            clear()
            return false
        }
        consumedLength = (consumedLength + advance).coerceAtMost(expected.length)
        if (consumedLength == expected.length) clear()
        return true
    }

    fun clear() {
        expectedText = null
        consumedLength = 0
    }

    private fun overlapAdvance(expected: String, chunk: String): Int {
        val consumed = expected.substring(0, consumedLength)
        for (overlap in minOf(consumed.length, chunk.length) downTo 1) {
            if (consumed.endsWith(chunk.substring(0, overlap)) &&
                expected.startsWith(chunk.substring(overlap), consumedLength)
            ) return chunk.length - overlap
        }
        return -1
    }
}
