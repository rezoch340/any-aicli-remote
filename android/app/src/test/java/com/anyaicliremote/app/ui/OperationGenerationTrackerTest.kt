package com.anyaicliremote.app.ui

import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class OperationGenerationTrackerTest {
    @Test
    fun advancingConnectionInvalidatesAllOlderOperations() {
        val tracker = OperationGenerationTracker()
        val firstConnection = tracker.advanceConnection()
        val firstSession = tracker.advanceSession()

        val secondConnection = tracker.advanceConnection()

        assertFalse(tracker.isConnectionCurrent(firstConnection))
        assertFalse(tracker.isSessionCurrent(firstSession))
        assertTrue(tracker.isConnectionCurrent(secondConnection))
        assertTrue(tracker.isSessionCurrent(secondConnection))
    }

    @Test
    fun advancingSessionKeepsConnectionButInvalidatesOlderSession() {
        val tracker = OperationGenerationTracker()
        val connection = tracker.advanceConnection()
        val firstSession = tracker.advanceSession()

        val secondSession = tracker.advanceSession()

        assertTrue(tracker.isConnectionCurrent(connection))
        assertFalse(tracker.isSessionCurrent(firstSession))
        assertTrue(tracker.isSessionCurrent(secondSession))
    }
}
