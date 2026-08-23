package com.anyaicliremote.feature.ui

import com.anyaicliremote.core.remote.ACPEvent
import com.anyaicliremote.core.model.ConnectionStatus
import com.anyaicliremote.core.model.SessionSummary
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.put
import org.junit.Assert.assertTrue
import org.junit.Test

class ChatEventReducerTest {
    @Test
    fun connectedChatRejectsNullIdentityWithoutSelectedSession() {
        val state = ChatUiState(
            destination = AppDestination.CHAT,
            connection = ConnectionStatus.CONNECTED,
        )
        val reduction = ChatEventReducer().reduceNotification(
            state,
            ACPEvent.SessionUpdate(identity = null, update = agentMessageUpdate()),
        )

        assertTrue(reduction.state.blocks.isEmpty())
    }

    @Test
    fun connectedChatRejectsNullIdentityWithSelectedSession() {
        val state = ChatUiState(
            destination = AppDestination.CHAT,
            connection = ConnectionStatus.CONNECTED,
            selectedSession = SessionSummary(
                providerId = "provider",
                id = "session",
                title = "title",
                projectDirectory = "/work",
                resident = false,
                activity = "",
                createdAt = 0,
                lastActiveAt = 0,
            ),
        )
        val reduction = ChatEventReducer().reduceNotification(
            state,
            ACPEvent.SessionUpdate(identity = null, update = agentMessageUpdate()),
        )

        assertTrue(reduction.state.blocks.isEmpty())
    }

    @Test
    fun connectedChatAcceptsMatchingNonNullIdentity() {
        val selectedSession = SessionSummary(
            providerId = "provider",
            id = "session",
            title = "title",
            projectDirectory = "/work",
            resident = false,
            activity = "",
            createdAt = 0,
            lastActiveAt = 0,
        )
        val state = ChatUiState(
            destination = AppDestination.CHAT,
            connection = ConnectionStatus.CONNECTED,
            selectedSession = selectedSession,
        )
        val reduction = ChatEventReducer().reduceNotification(
            state,
            ACPEvent.SessionUpdate(selectedSession.identity, agentMessageUpdate()),
        )

        assertTrue(reduction.state.blocks.isNotEmpty())
    }

    private fun agentMessageUpdate() = buildJsonObject {
        put("sessionUpdate", "agent_message_chunk")
        put("text", "hello")
    }
}
