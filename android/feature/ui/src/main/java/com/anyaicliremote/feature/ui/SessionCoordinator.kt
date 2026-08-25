package com.anyaicliremote.feature.ui

import com.anyaicliremote.core.model.ConnectionStatus
import com.anyaicliremote.core.model.SessionIdentity
import com.anyaicliremote.core.model.SessionSummary
import com.anyaicliremote.core.session.CreatedSessionResponse
import com.anyaicliremote.core.session.SessionController
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.launch

/** 会话列表、打开、历史同步、挂载与新建。 */
internal class SessionCoordinator(
    private val scope: ChatOperationScope,
    private val sessionController: SessionController,
    private val coroutineScope: CoroutineScope,
) {
    fun refreshSessions() {
        if (scope.state.connection != ConnectionStatus.CONNECTED) return
        val deviceId = scope.state.activeDeviceId ?: return
        val operationToken = scope.current()
        scope.refreshSessionsJob?.cancel()
        scope.refreshSessionsJob = coroutineScope.launch {
            UiOperationRunner.run(
                isCurrent = { scope.isConnectionCurrent(operationToken, deviceId) },
                onFailure = scope::showError,
            ) {
                val sessions = fetchSessions()
                if (scope.isConnectionCurrent(operationToken, deviceId)) {
                    scope.update { it.copy(sessions = sessions, error = null) }
                }
            }
        }
    }

    suspend fun fetchSessions(): List<SessionSummary> {
        return sessionController.listSessions()
    }

    fun openSession(session: SessionSummary) {
        if (scope.state.connection != ConnectionStatus.CONNECTED) return
        val deviceId = scope.state.activeDeviceId ?: return
        val operationToken = scope.beginSessionOperation()
        showOpeningSession(session)
        scope.openSessionJob = coroutineScope.launch {
            loadSessionHistory(session, deviceId, operationToken)
            if (!scope.isSessionCurrent(operationToken, deviceId, session.identity)) return@launch
            mountSession(session, deviceId, operationToken)
        }
    }

    private fun showOpeningSession(session: SessionSummary) {
        scope.update {
            it.copy(
                destination = AppDestination.CHAT,
                selectedSession = session,
                blocks = emptyList(),
                childAgents = emptyList(),
                pendingInteraction = null,
                sessionMode = "",
                sessionNotice = "",
                busy = false,
                status = "同步历史",
                error = null,
                selectedFiles = emptyList(),
                filePickerVisible = false,
            )
        }
    }

    private suspend fun loadSessionHistory(
        session: SessionSummary,
        deviceId: String,
        operationToken: OperationGeneration,
    ) {
        UiOperationRunner.run(
            isCurrent = { scope.isSessionCurrent(operationToken, deviceId, session.identity) },
            onFailure = { scope.updateStatus("历史暂不可用：${it.message}") },
        ) {
            val history = sessionController.loadHistory(session)
            if (scope.isSessionCurrent(operationToken, deviceId, session.identity)) {
                scope.update { it.copy(selectedSession = history.session, blocks = history.blocks, childAgents = history.childAgents, busy = false) }
            }
        }
    }

    private suspend fun mountSession(
        session: SessionSummary,
        deviceId: String,
        operationToken: OperationGeneration,
    ) {
        UiOperationRunner.run(
            isCurrent = { scope.isSessionCurrent(operationToken, deviceId, session.identity) },
            onFailure = { scope.updateStatus("挂载失败：${it.message}") },
        ) {
            // 挂载期间 provider 会把整轮对话作为 session/update 重放。上面刚落地的历史
            // 已是权威内容，屏蔽重放，避免消息、思考与回复各被追加一遍。
            scope.isMountingSession = true
            val model = try {
                sessionController.mount(session, scope.state.model)
            } finally {
                scope.isMountingSession = false
            }
            if (scope.isSessionCurrent(operationToken, deviceId, session.identity)) {
                scope.update { it.copy(model = model, status = "在线") }
            }
        }
    }

    fun closeSession() {
        scope.invalidateSessionOperations()
        scope.update {
            it.copy(
                destination = AppDestination.SESSIONS,
                selectedSession = null,
                blocks = emptyList(),
                childAgents = emptyList(),
                pendingInteraction = null,
                sessionMode = "",
                sessionNotice = "",
                busy = false,
                selectedFiles = emptyList(),
                filePickerVisible = false,
            )
        }
    }

    fun createSession(workingDirectory: String) {
        if (scope.state.connection != ConnectionStatus.CONNECTED) return
        val deviceId = scope.state.activeDeviceId ?: return
        val token = scope.beginSessionOperation()
        val known = scope.state.sessions.map(SessionSummary::identity).toSet()
        scope.update { it.copy(status = "正在创建会话", error = null) }
        scope.createSessionJob = coroutineScope.launch {
            UiOperationRunner.run(
                isCurrent = { scope.isSessionOperationCurrent(token, deviceId) },
                onFailure = scope::showError,
            ) {
                val created = sessionController.create(workingDirectory)
                if (scope.isSessionOperationCurrent(token, deviceId)) {
                    showCreatedSession(created, known)
                }
            }
        }
    }

    private fun showCreatedSession(created: CreatedSessionResponse, known: Set<SessionIdentity>) {
        val session = created.sessions.singleOrNull { it.id == created.identifier && it.identity !in known }
            ?: created.responseSession?.takeIf { it.id == created.identifier }
        scope.update {
            if (session == null) {
                it.copy(
                    destination = AppDestination.SESSIONS,
                    selectedSession = null,
                    sessions = created.sessions,
                    blocks = emptyList(),
                    status = "会话已创建，等待历史索引",
                )
            } else {
                it.copy(
                    destination = AppDestination.CHAT,
                    selectedSession = session,
                    sessions = created.sessions,
                    blocks = emptyList(),
                    status = "在线",
                )
            }
        }
    }
}
