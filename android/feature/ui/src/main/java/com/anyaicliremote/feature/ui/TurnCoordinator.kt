package com.anyaicliremote.feature.ui

import com.anyaicliremote.core.model.ChatBlock
import com.anyaicliremote.core.model.ConnectionStatus
import com.anyaicliremote.core.model.SessionSummary
import com.anyaicliremote.core.model.ToolRunState
import com.anyaicliremote.core.remote.ACPEvent
import com.anyaicliremote.core.remote.ACPEventDecoder
import com.anyaicliremote.core.session.SessionController
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.launch
import kotlinx.serialization.json.JsonObject

/** 一轮对话的发起、取消、努力度切换、权限应答与通知归并。 */
internal class TurnCoordinator(
    private val scope: ChatOperationScope,
    private val sessionController: SessionController,
    private val chatEventReducer: ChatEventReducer,
    private val sessions: SessionCoordinator,
    private val coroutineScope: CoroutineScope,
) {
    fun send(text: String) {
        val session = scope.state.selectedSession ?: return
        val deviceId = scope.state.activeDeviceId ?: return
        if (scope.state.connection != ConnectionStatus.CONNECTED || scope.state.busy) return
        val payload = chatEventReducer.promptPayload(text, scope.state.selectedFiles) ?: return
        val operationToken = scope.current()
        val turnStart = chatEventReducer.beginTurn(scope.state, payload)
        scope.replace(turnStart.state)
        val turnId = turnStart.identifier
        scope.promptJob?.cancel()
        scope.promptJob = coroutineScope.launch {
            UiOperationRunner.run(
                isCurrent = { isActivePrompt(operationToken, deviceId, session, turnId) },
                onFailure = { exception ->
                    scope.update {
                        chatEventReducer.handlePromptFailure(it, exception, turnId)
                    }
                },
            ) {
                sessionController.prompt(session, payload.message, payload.attachments)
                if (isActivePrompt(operationToken, deviceId, session, turnId)) {
                    finishTurn(ToolRunState.SUCCESS, "完成")
                }
            }
        }
    }

    private fun isActivePrompt(
        operationToken: OperationGeneration,
        deviceId: String,
        session: SessionSummary,
        turnId: String,
    ): Boolean =
        scope.isSessionCurrent(operationToken, deviceId, session.identity) &&
            chatEventReducer.isActive(turnId)

    fun cancel() {
        val session = scope.state.selectedSession ?: return
        scope.promptJob?.cancel()
        scope.promptJob = null
        sessionController.cancel(session)
        finishTurn(ToolRunState.CANCELLED, "已停止")
    }

    fun setEffort(effort: String) {
        val session = scope.state.selectedSession ?: return
        val deviceId = scope.state.activeDeviceId ?: return
        val operationToken = scope.current()
        scope.effortJob?.cancel()
        scope.effortJob = coroutineScope.launch {
            UiOperationRunner.run(
                isCurrent = { scope.isSessionCurrent(operationToken, deviceId, session.identity) },
                onFailure = scope::showError,
            ) {
                sessionController.setEffort(session, scope.state.model.currentModelId, effort)
                if (scope.isSessionCurrent(operationToken, deviceId, session.identity)) {
                    scope.update { it.copy(model = it.model.copy(effort = effort)) }
                }
            }
        }
    }

    fun answerPermission(block: ChatBlock, optionId: String?) {
        val requestId = block.rpcId ?: return
        sessionController.answerPermission(requestId, optionId)
        scope.update { it.copy(blocks = it.blocks.filterNot { item -> item.id == block.id }) }
    }

    fun handleNotification(message: JsonObject) {
        val event = ACPEventDecoder.decode(message) ?: return
        // 挂载期间 provider 会重放整轮对话，历史快照已经包含这些内容；只丢弃重放的
        // session/update，权限请求等仍需照常处理。
        if (scope.isMountingSession && event is ACPEvent.SessionUpdate) return
        val reduction = chatEventReducer.reduceNotification(scope.state, event)
        scope.replace(reduction.state)
        if (reduction.action == ChatEventAction.RefreshSessions) sessions.refreshSessions()
    }

    private fun finishTurn(toolState: ToolRunState, status: String) {
        scope.update { chatEventReducer.finishTurn(it, toolState, status) }
    }
}
