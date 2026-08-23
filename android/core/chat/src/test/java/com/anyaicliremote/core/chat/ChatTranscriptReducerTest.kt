package com.anyaicliremote.core.chat

import com.anyaicliremote.core.model.ChatBlock
import com.anyaicliremote.core.model.ChatBlockKind
import com.anyaicliremote.core.model.ToolRunState
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.put
import org.junit.Assert.assertEquals
import org.junit.Test

class ChatTranscriptReducerTest {
    @Test
    fun chunksMergeOnlyAdjacentMessageKinds() {
        val first = ChatTranscriptReducer.appendChunk(emptyList(), ChatBlockKind.ASSISTANT, "a")
        val merged = ChatTranscriptReducer.appendChunk(first, ChatBlockKind.ASSISTANT, "b")
        val separated = ChatTranscriptReducer.appendChunk(merged, ChatBlockKind.SYSTEM, "notice")
        assertEquals(listOf("ab", "notice"), separated.map { it.text })
    }

    @Test
    fun toolUpdatesUpsertAndFinalizationPreservesTerminalState() {
        val update = buildJsonObject { put("toolCallId", "one"); put("title", "shell"); put("status", "running") }
        val updated = ChatTranscriptReducer.upsertTool(emptyList(), update)
        val completed = ChatTranscriptReducer.upsertTool(updated, buildJsonObject { put("toolCallId", "one"); put("result", "ok"); put("status", "success") })
        val finalized = ChatTranscriptReducer.finalizeActiveTools(completed + ChatBlock("pending", ChatBlockKind.TOOL), ToolRunState.CANCELLED)
        assertEquals("ok", finalized.first().detail)
        assertEquals(ToolRunState.SUCCESS, finalized.first().toolState)
        assertEquals(ToolRunState.CANCELLED, finalized.last().toolState)
    }


}
