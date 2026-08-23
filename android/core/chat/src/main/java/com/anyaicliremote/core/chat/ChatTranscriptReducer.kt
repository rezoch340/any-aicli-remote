package com.anyaicliremote.core.chat

import com.anyaicliremote.core.model.ChatBlock
import com.anyaicliremote.core.model.ChatBlockKind
import com.anyaicliremote.core.model.ToolRunState
import com.anyaicliremote.core.model.obj
import com.anyaicliremote.core.model.string
import com.anyaicliremote.core.model.toolState
import kotlinx.serialization.json.JsonObject
import java.util.UUID

/** Stateless transcript operations; orchestration and turn ownership stay in ChatViewModel. */
object ChatTranscriptReducer {
    fun appendChunk(blocks: List<ChatBlock>, kind: ChatBlockKind, text: String): List<ChatBlock> {
        if (text.isEmpty()) return blocks
        val result = blocks.toMutableList()
        val last = result.lastOrNull()
        if (last?.kind == kind && kind in MERGEABLE_KINDS) {
            result[result.lastIndex] = last.copy(text = last.text + text)
        } else {
            result += ChatBlock(UUID.randomUUID().toString(), kind, text = text)
        }
        return result
    }

    fun upsertTool(blocks: List<ChatBlock>, update: JsonObject): List<ChatBlock> {
        val toolIdentifier = update.string("toolCallId", "tool_call_id", "id") ?: UUID.randomUUID().toString()
        val blockIdentifier = "tool-$toolIdentifier"
        val title = update.string("title", "toolName", "kind")
        val statusValue = update.string("status", "toolStatus")
        val detail = update.string("result") ?: update.obj("content")?.string("text") ?: ""
        val result = blocks.toMutableList()
        val index = result.indexOfFirst { it.id == blockIdentifier }
        if (index >= 0) {
            val previous = result[index]
            result[index] = previous.copy(
                title = title ?: previous.title,
                detail = detail.ifEmpty { previous.detail },
                toolState = statusValue?.let(::toolState) ?: previous.toolState,
            )
        } else {
            result += ChatBlock(
                id = blockIdentifier,
                kind = ChatBlockKind.TOOL,
                title = title ?: "工具",
                detail = detail,
                toolState = toolState(statusValue),
            )
        }
        return result
    }

    fun finalizeActiveTools(blocks: List<ChatBlock>, finalState: ToolRunState): List<ChatBlock> =
        blocks.map { block ->
            if (block.kind == ChatBlockKind.TOOL && block.toolState in ACTIVE_TOOL_STATES) {
                block.copy(toolState = finalState)
            } else {
                block
            }
        }

    private val MERGEABLE_KINDS = setOf(ChatBlockKind.USER, ChatBlockKind.ASSISTANT, ChatBlockKind.THINKING)
    private val ACTIVE_TOOL_STATES = setOf(ToolRunState.PENDING, ToolRunState.RUNNING)
}
