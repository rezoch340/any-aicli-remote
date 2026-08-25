package com.anyaicliremote.feature.ui

import com.anyaicliremote.core.remote.ACPEvent
import com.anyaicliremote.core.chat.ChatTranscriptReducer
import com.anyaicliremote.core.chat.ChildAgentReducer
import com.anyaicliremote.core.chat.PendingUserEchoTracker
import com.anyaicliremote.core.model.ChatBlock
import com.anyaicliremote.core.model.ChatBlockKind
import com.anyaicliremote.core.model.ConnectionStatus
import com.anyaicliremote.core.model.ToolRunState
import com.anyaicliremote.core.model.WorkspaceFile
import com.anyaicliremote.core.model.obj
import com.anyaicliremote.core.model.string
import kotlinx.serialization.json.JsonObject
import java.util.UUID

/** Owns turn-local state and reduces ACP events into immutable UI state. */
internal class ChatEventReducer {
    private val pendingUserEchoTracker = PendingUserEchoTracker()
    private var activeTurnId: String? = null

    fun beginTurn(state: ChatUiState, payload: PromptPayload): TurnStart {
        val turnId = UUID.randomUUID().toString()
        activeTurnId = turnId
        pendingUserEchoTracker.clear()
        payload.message.takeIf(String::isNotEmpty)?.let(pendingUserEchoTracker::begin)
        val userBlock = ChatBlock(
            id = UUID.randomUUID().toString(),
            kind = ChatBlockKind.USER,
            text = payload.message,
            attachments = payload.attachments,
        )
        return TurnStart(
            turnId,
            state.copy(
                blocks = state.blocks + userBlock,
                busy = true,
                status = "等待助手",
                selectedFiles = emptyList(),
                sessionNotice = "",
            ),
        )
    }

    fun isActive(turnId: String): Boolean = activeTurnId == turnId

    fun promptPayload(text: String, attachments: List<WorkspaceFile>): PromptPayload? {
        val payload = PromptPayload(text.trim(), attachments)
        return payload.takeUnless { it.message.isEmpty() && it.attachments.isEmpty() }
    }

    fun handlePromptFailure(
        state: ChatUiState,
        exception: Throwable,
        turnId: String,
    ): ChatUiState {
        if (exception.message.orEmpty().contains("cancel", ignoreCase = true)) {
            return finishTurn(state, ToolRunState.CANCELLED, "已停止")
        }
        val failureState = state.copy(
            blocks = state.blocks + ChatBlock(
                id = UUID.randomUUID().toString(),
                kind = ChatBlockKind.SYSTEM,
                text = exception.message ?: exception.toString(),
            ),
        )
        return if (isActive(turnId)) {
            finishTurn(failureState, ToolRunState.FAILED, "发送失败")
        } else {
            failureState
        }
    }

    fun reduceNotification(state: ChatUiState, event: ACPEvent): ChatEventReduction {
        return when (event) {
            is ACPEvent.SessionUpdate -> reduceSessionUpdate(state, event)
            ACPEvent.SessionsChanged -> {
                if (state.destination == AppDestination.SESSIONS) {
                    ChatEventReduction(state, ChatEventAction.RefreshSessions)
                } else {
                    ChatEventReduction(state)
                }
            }
            is ACPEvent.PermissionRequest -> reducePermissionRequest(state, event)
            is ACPEvent.ChildAgentUpdate -> reduceChildAgent(state, event)
            is ACPEvent.Interaction -> reduceInteraction(state, event)
            is ACPEvent.SessionStatus -> reduceSessionStatus(state, event)
        }
    }

    private fun reduceSessionStatus(
        state: ChatUiState,
        event: ACPEvent.SessionStatus,
    ): ChatEventReduction {
        if (!acceptsSessionEvent(state, event.identity)) return ChatEventReduction(state)
        val notice = SessionStatusFormatter.notice(event.status)
        return ChatEventReduction(state.copy(sessionNotice = notice))
    }

    private fun reduceChildAgent(
        state: ChatUiState,
        event: ACPEvent.ChildAgentUpdate,
    ): ChatEventReduction {
        if (!acceptsSessionEvent(state, event.identity)) return ChatEventReduction(state)
        return ChatEventReduction(state.copy(childAgents = ChildAgentReducer.apply(state.childAgents, event.card)))
    }

    private fun reduceInteraction(
        state: ChatUiState,
        event: ACPEvent.Interaction,
    ): ChatEventReduction {
        if (!acceptsSessionEvent(state, event.request.sessionIdentity)) return ChatEventReduction(state)
        return ChatEventReduction(state.copy(pendingInteraction = event.request))
    }

    fun finishTurn(
        state: ChatUiState,
        toolState: ToolRunState,
        status: String,
    ): ChatUiState {
        reset()
        return state.copy(
            blocks = ChatTranscriptReducer.finalizeActiveTools(state.blocks, toolState),
            busy = false,
            status = status,
        )
    }

    fun reset() {
        activeTurnId = null
        pendingUserEchoTracker.clear()
    }

    private fun reduceSessionUpdate(
        state: ChatUiState,
        event: ACPEvent.SessionUpdate,
    ): ChatEventReduction {
        if (!acceptsSessionEvent(state, event.identity)) return ChatEventReduction(state)
        return ChatEventReduction(reduceUpdate(state, event.update))
    }

    private fun reducePermissionRequest(
        state: ChatUiState,
        event: ACPEvent.PermissionRequest,
    ): ChatEventReduction {
        if (!acceptsSessionEvent(state, event.identity)) return ChatEventReduction(state)
        val block = ChatBlock(
            id = "permission-${event.requestId}",
            kind = ChatBlockKind.PERMISSION,
            text = event.question,
            rpcId = event.requestId,
            options = event.options,
        )
        return ChatEventReduction(state.copy(blocks = state.blocks + block))
    }

    private fun acceptsSessionEvent(
        state: ChatUiState,
        identity: com.anyaicliremote.core.model.SessionIdentity?,
    ): Boolean {
        val selectedIdentity = state.selectedSession?.identity
        return state.connection == ConnectionStatus.CONNECTED &&
            state.destination == AppDestination.CHAT &&
            selectedIdentity != null &&
            identity != null &&
            selectedIdentity == identity
    }

    private fun reduceUpdate(state: ChatUiState, update: JsonObject): ChatUiState {
        val type = update.string("sessionUpdate").orEmpty()
        val text = update.obj("content")?.string("text") ?: update.string("text").orEmpty()
        return when (type) {
            "user_message_chunk" -> {
                if (pendingUserEchoTracker.consume(text)) state else appendChunk(state, ChatBlockKind.USER, text)
            }
            "current_mode_update" -> state.copy(sessionMode = update.string("currentModeId", "current_mode_id").orEmpty())
            "agent_message_chunk" -> withTurnStatus(
                // Fresh assistant output means any prior retry/switch notice is stale.
                appendChunk(state, ChatBlockKind.ASSISTANT, text).copy(sessionNotice = ""),
                "正在回复",
            )
            "agent_thought_chunk" -> withTurnStatus(
                appendChunk(state, ChatBlockKind.THINKING, text),
                "正在思考",
            )
            "tool_call", "tool_call_update" -> withTurnStatus(
                state.copy(blocks = ChatTranscriptReducer.upsertTool(state.blocks, update)),
                update.string("title", "toolName", "kind") ?: "正在使用工具",
            )
            "plan" -> appendChunk(state, ChatBlockKind.PLAN, text.ifEmpty { "Plan" })
            "session_recap" -> appendChunk(state, ChatBlockKind.SYSTEM, text)
            else -> reduceTerminal(state, type)
        }
    }

    private fun reduceTerminal(state: ChatUiState, type: String): ChatUiState = when (type) {
        "turn_completed", "task_completed" -> finishTurn(state, ToolRunState.SUCCESS, "完成")
        "cancelled", "turn_cancelled", "task_cancelled" -> finishTurn(state, ToolRunState.CANCELLED, "已停止")
        "turn_failed", "task_failed", "failed", "error" -> finishTurn(state, ToolRunState.FAILED, "执行失败")
        else -> state
    }

    private fun withTurnStatus(state: ChatUiState, status: String): ChatUiState =
        if (activeTurnId == null) state else state.copy(busy = true, status = status)

    private fun appendChunk(
        state: ChatUiState,
        kind: ChatBlockKind,
        text: String,
    ): ChatUiState = state.copy(
        blocks = ChatTranscriptReducer.appendChunk(state.blocks, kind, text),
    )
}

internal data class PromptPayload(
    val message: String,
    val attachments: List<WorkspaceFile>,
)

internal data class TurnStart(val identifier: String, val state: ChatUiState)

internal data class ChatEventReduction(
    val state: ChatUiState,
    val action: ChatEventAction? = null,
)

internal sealed interface ChatEventAction {
    data object RefreshSessions : ChatEventAction
}
