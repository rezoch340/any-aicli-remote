package com.anyaicliremote.core.chat

import com.anyaicliremote.core.model.ChatBlock
import com.anyaicliremote.core.model.ChatBlockKind
import com.anyaicliremote.core.model.ToolRunState
import kotlinx.serialization.json.add
import kotlinx.serialization.json.buildJsonArray
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.put
import kotlinx.serialization.json.putJsonArray
import kotlinx.serialization.json.putJsonObject
import org.junit.Assert.assertEquals
import org.junit.Test

class ChatTranscriptReducerTest {
    @Test
    fun toolOutputComesFromStandardAcpContentArray() {
        // ACP tool content is an array of {type:"content", content:{type:"text",text}}.
        val update = buildJsonObject {
            put("toolCallId", "ls-1")
            put("status", "completed")
            putJsonArray("content") {
                add(buildJsonObject {
                    put("type", "content")
                    putJsonObject("content") { put("type", "text"); put("text", "file-a\nfile-b") }
                })
            }
        }
        val blocks = ChatTranscriptReducer.upsertTool(emptyList(), update)
        assertEquals("file-a\nfile-b", blocks.single().detail)
    }

    @Test
    fun toolOutputAlsoReadsBareContentBlockDefensively() {
        val update = buildJsonObject {
            put("toolCallId", "ls-2")
            putJsonArray("content") { add(buildJsonObject { put("type", "text"); put("text", "out") }) }
        }
        assertEquals("out", ChatTranscriptReducer.upsertTool(emptyList(), update).single().detail)
    }

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
