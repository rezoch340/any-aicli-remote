package com.anyaicliremote.feature.ui

import com.anyaicliremote.core.model.ConnectionStatus
import com.anyaicliremote.core.model.SessionIdentity
import kotlinx.coroutines.Job
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.update

/**
 * 界面操作的归属与取消。代际推进、进行中任务的持有与撤销、以及“这次操作是否仍然当前”
 * 的判定只在这里实现一份，协调器与 ViewModel 都调用它，不各自复制一套。
 */
internal class ChatOperationScope(
    val stateFlow: MutableStateFlow<ChatUiState>,
    private val workspaceBrowser: WorkspaceFileBrowser,
    private val chatEventReducer: ChatEventReducer,
) {
    private val operationGenerations = OperationGenerationTracker()
    var connectJob: Job? = null
    var refreshSessionsJob: Job? = null
    var openSessionJob: Job? = null
    var createSessionJob: Job? = null
    var promptJob: Job? = null
    var effortJob: Job? = null

    /** 为 true 时表示正在向 provider 挂载会话，其间收到的 session/update 是历史重放。 */
    var isMountingSession = false

    val state: ChatUiState get() = stateFlow.value

    fun update(transform: (ChatUiState) -> ChatUiState) = stateFlow.update(transform)

    fun replace(next: ChatUiState) {
        stateFlow.value = next
    }

    fun current(): OperationGeneration = operationGenerations.current()

    fun advanceConnection(): OperationGeneration = operationGenerations.advanceConnection()

    fun beginSessionOperation(): OperationGeneration {
        invalidateSessionOperations()
        return operationGenerations.current()
    }

    fun invalidateSessionOperations() {
        operationGenerations.advanceSession()
        openSessionJob?.cancel()
        openSessionJob = null
        createSessionJob?.cancel()
        createSessionJob = null
        promptJob?.cancel()
        promptJob = null
        effortJob?.cancel()
        effortJob = null
        workspaceBrowser.cancel()
        chatEventReducer.reset()
    }

    fun cancelConnectionOperations() {
        connectJob?.cancel()
        connectJob = null
        refreshSessionsJob?.cancel()
        refreshSessionsJob = null
        invalidateSessionOperations()
    }

    fun isConnectionCurrent(operationToken: OperationGeneration, deviceId: String): Boolean =
        operationGenerations.isConnectionCurrent(operationToken) && state.activeDeviceId == deviceId

    fun isSessionOperationCurrent(operationToken: OperationGeneration, deviceId: String): Boolean =
        operationGenerations.isSessionCurrent(operationToken) &&
            state.activeDeviceId == deviceId &&
            state.connection == ConnectionStatus.CONNECTED

    fun isSessionCurrent(
        operationToken: OperationGeneration,
        deviceId: String,
        sessionIdentity: SessionIdentity,
    ): Boolean =
        isSessionOperationCurrent(operationToken, deviceId) &&
            state.destination == AppDestination.CHAT &&
            state.selectedSession?.identity == sessionIdentity

    fun updateStatus(status: String) {
        update { it.copy(status = status) }
    }

    fun showError(error: Throwable) {
        update {
            it.copy(
                error = error.message ?: error.toString(),
                status = error.message.orEmpty(),
            )
        }
    }
}
