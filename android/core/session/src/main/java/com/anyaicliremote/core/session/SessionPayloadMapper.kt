package com.anyaicliremote.core.session

import com.anyaicliremote.core.model.ChatBlock
import com.anyaicliremote.core.model.ChatBlockKind
import com.anyaicliremote.core.model.ModelState
import com.anyaicliremote.core.model.SessionMessage
import com.anyaicliremote.core.model.SessionSummary
import com.anyaicliremote.core.model.ToolRunState
import com.anyaicliremote.core.model.WorkspaceFile
import com.anyaicliremote.core.model.ChildAgentCard
import com.anyaicliremote.core.model.childAgentStatus
import com.anyaicliremote.core.model.long
import com.anyaicliremote.core.model.obj
import com.anyaicliremote.core.model.objects
import com.anyaicliremote.core.model.string
import kotlinx.serialization.json.JsonObject

/** Pure translation of REST/RPC payloads into the UI model. */
object SessionPayloadMapper {
    fun sessions(response: JsonObject): List<SessionSummary> =
        response.objects("sessions").mapNotNull(SessionSummary::from)
            .sortedWith(compareByDescending<SessionSummary> { it.resident }.thenByDescending { it.updatedAt })

    fun historyBlocks(session: SessionSummary, messages: List<SessionMessage>): List<ChatBlock> =
        messages.mapIndexed { index, message -> historyBlock(session, index, message) }

    fun historyBlock(session: SessionSummary, index: Int, message: SessionMessage): ChatBlock {
        val blockId = "history:${session.providerId}:${session.id}:$index"
        return when (message.role) {
            "user" -> ChatBlock(blockId, ChatBlockKind.USER, text = message.content)
            "assistant" -> ChatBlock(blockId, ChatBlockKind.ASSISTANT, text = message.content)
            "tool" -> ChatBlock(
                id = blockId,
                kind = ChatBlockKind.TOOL,
                title = "工具",
                detail = message.content,
                toolState = ToolRunState.SUCCESS,
            )
            else -> ChatBlock(blockId, ChatBlockKind.SYSTEM, text = message.content)
        }
    }

    fun workspaceDirectories(response: JsonObject): List<WorkspaceFile> =
        response.objects("dirs").mapNotNull { item -> WorkspaceFile.from(item, true) }

    fun workspaceFiles(response: JsonObject): List<WorkspaceFile> =
        response.objects("files").mapNotNull { item -> WorkspaceFile.from(item, false) }

    /** Reads the history child-agent snapshot returned alongside messages. */
    fun childAgents(response: JsonObject): List<ChildAgentCard> =
        response.objects("childAgents").mapNotNull { record ->
            val providerChildId = record.string("providerChildId") ?: return@mapNotNull null
            ChildAgentCard(
                providerChildId = providerChildId,
                childSessionId = record.string("childSessionId") ?: "",
                agentType = record.string("agentType") ?: "",
                description = record.string("description") ?: "",
                status = childAgentStatus(record.string("status")),
                startedAt = record.long("startedAt"),
                completedAt = record.long("completedAt"),
                toolCallCount = record.long("toolCallCount").toInt(),
                turnCount = record.long("turnCount").toInt(),
                modelId = record.string("modelId") ?: "",
                tokensUsed = record.long("tokensUsed"),
            )
        }

    fun modelState(response: JsonObject, fallback: ModelState): ModelState {
        val source = response.obj("models")
            ?: response.obj("_meta")?.obj("modelState")
            ?: response.obj("modelState")
            ?: return fallback
        var model = fallback
        source.string("currentModelId")?.let { model = model.copy(currentModelId = it) }
        val current = source.objects("availableModels").firstOrNull { it.string("modelId") == model.currentModelId }
        val metadata = current?.obj("_meta")
        metadata?.string("reasoningEffort")?.let { model = model.copy(effort = it) }
        val levels = metadata?.objects("reasoningEfforts")?.mapNotNull { it.string("value", "id") }.orEmpty()
        if (levels.isNotEmpty()) model = model.copy(effortLevels = levels)
        return model
    }
}
