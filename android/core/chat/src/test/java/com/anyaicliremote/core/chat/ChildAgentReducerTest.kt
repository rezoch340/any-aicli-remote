package com.anyaicliremote.core.chat

import com.anyaicliremote.core.model.ChildAgentCard
import com.anyaicliremote.core.model.ChildAgentStatus
import org.junit.Assert.assertEquals
import org.junit.Test

class ChildAgentReducerTest {
    @Test
    fun insertsNewCardKeepingFirstSeenOrder() {
        val cards = ChildAgentReducer.apply(emptyList(), card("a", sequence = 1))
            .let { ChildAgentReducer.apply(it, card("b", sequence = 2)) }
        assertEquals(listOf("a", "b"), cards.map { it.providerChildId })
    }

    @Test
    fun mergesInPlaceOnSameChildId() {
        val started = ChildAgentReducer.apply(emptyList(), card("a", status = ChildAgentStatus.RUNNING, sequence = 1))
        val finished = ChildAgentReducer.apply(started, card("a", status = ChildAgentStatus.COMPLETED, sequence = 2))
        assertEquals(1, finished.size)
        assertEquals(ChildAgentStatus.COMPLETED, finished.single().status)
    }

    @Test
    fun dropsStaleOutOfOrderUpdate() {
        val finished = ChildAgentReducer.apply(emptyList(), card("a", status = ChildAgentStatus.COMPLETED, sequence = 5))
        // A late update with a smaller sequence must not regress the card.
        val afterStale = ChildAgentReducer.apply(finished, card("a", status = ChildAgentStatus.RUNNING, sequence = 3))
        assertEquals(ChildAgentStatus.COMPLETED, afterStale.single().status)
    }

    @Test
    fun mergeDoesNotEraseEarlierFieldsWithEmptyUpdate() {
        val started = ChildAgentReducer.apply(emptyList(), card("a", description = "explore", sequence = 1))
        val progress = ChildAgentReducer.apply(started, card("a", description = "", turnCount = 3, sequence = 2))
        assertEquals("explore", progress.single().description)
        assertEquals(3, progress.single().turnCount)
    }

    private fun card(
        id: String,
        status: ChildAgentStatus = ChildAgentStatus.RUNNING,
        description: String = "",
        turnCount: Int = 0,
        sequence: Long? = null,
    ) = ChildAgentCard(
        providerChildId = id,
        status = status,
        description = description,
        turnCount = turnCount,
        sequence = sequence,
    )
}
