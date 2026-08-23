package com.anyaicliremote.core.chat

import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class PendingUserEchoTrackerTest {
    @Test
    fun fullEchoIsConsumed() {
        val tracker = PendingUserEchoTracker()
        tracker.begin("hello")

        assertTrue(tracker.consume("hello"))
        assertFalse(tracker.consume("hello"))
    }

    @Test
    fun deltaChunksAreConsumed() {
        val tracker = PendingUserEchoTracker()
        tracker.begin("hello")

        assertTrue(tracker.consume("hel"))
        assertTrue(tracker.consume("lo"))
    }

    @Test
    fun cumulativeSnapshotIsConsumed() {
        val tracker = PendingUserEchoTracker()
        tracker.begin("hello")

        assertTrue(tracker.consume("hel"))
        assertTrue(tracker.consume("hello"))
    }

    @Test
    fun overlapIsConsumed() {
        val tracker = PendingUserEchoTracker()
        tracker.begin("hello")

        assertTrue(tracker.consume("hel"))
        assertTrue(tracker.consume("ello"))
    }

    @Test
    fun unrelatedChunkClearsPending() {
        val tracker = PendingUserEchoTracker()
        tracker.begin("hello")

        assertFalse(tracker.consume("xyz"))
        assertFalse(tracker.consume("hello"))
    }

    @Test
    fun clearDropsPending() {
        val tracker = PendingUserEchoTracker()
        tracker.begin("hello")
        tracker.clear()

        assertFalse(tracker.consume("hello"))
    }

    @Test
    fun emptyChunkIsConsumedWhilePending() {
        val tracker = PendingUserEchoTracker()
        tracker.begin("hello")

        assertTrue(tracker.consume(""))
        assertTrue(tracker.consume("hello"))
    }
}
