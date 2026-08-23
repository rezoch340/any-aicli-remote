package com.anyaicliremote.app.ui

import com.anyaicliremote.app.model.ChatBlock
import com.anyaicliremote.app.model.ChatBlockKind
import com.anyaicliremote.app.model.SessionIdentity
import com.anyaicliremote.app.model.ToolRunState
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.put
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class TurnFinalizationTest {
    @Test
    fun finalizationEndsOnlyActiveToolBlocks() {
        val blocks = listOf(
            ChatBlock("pending", ChatBlockKind.TOOL, toolState = ToolRunState.PENDING),
            ChatBlock("running", ChatBlockKind.TOOL, toolState = ToolRunState.RUNNING),
            ChatBlock("complete", ChatBlockKind.TOOL, toolState = ToolRunState.SUCCESS),
            ChatBlock("message", ChatBlockKind.ASSISTANT, text = "done"),
        )

        val finalized = finalizeActiveTools(blocks, ToolRunState.CANCELLED)

        assertEquals(ToolRunState.CANCELLED, finalized[0].toolState)
        assertEquals(ToolRunState.CANCELLED, finalized[1].toolState)
        assertEquals(ToolRunState.SUCCESS, finalized[2].toolState)
        assertEquals(blocks[3], finalized[3])
    }

    @Test
    fun sessionNotificationRequiresExactCompositeIdentity() {
        val expected = SessionIdentity("provider-one", "shared-session")
        val exact = buildJsonObject {
            put("providerId", "provider-one")
            put("sessionId", "shared-session")
        }
        val otherProvider = buildJsonObject {
            put("providerId", "provider-two")
            put("sessionId", "shared-session")
        }
        val missingProvider = buildJsonObject { put("sessionId", "shared-session") }

        assertTrue(matchesSessionIdentity(exact, expected))
        assertFalse(matchesSessionIdentity(otherProvider, expected))
        assertFalse(matchesSessionIdentity(missingProvider, expected))
    }
}
