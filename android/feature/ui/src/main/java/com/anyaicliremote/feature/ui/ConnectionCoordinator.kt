package com.anyaicliremote.feature.ui

import com.anyaicliremote.core.model.ConnectionStatus
import com.anyaicliremote.core.model.ModelState
import com.anyaicliremote.core.model.SavedDevice
import com.anyaicliremote.core.remote.AnyAICLIRemoteClient
import com.anyaicliremote.core.session.SessionPayloadMapper
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.launch

/** 设备连接的建立、断开与断线恢复。 */
internal class ConnectionCoordinator(
    private val scope: ChatOperationScope,
    private val client: AnyAICLIRemoteClient,
    private val sessions: SessionCoordinator,
    private val chatEventReducer: ChatEventReducer,
    private val coroutineScope: CoroutineScope,
) {
    /** 主动断开时置位，用于区分用户断开与意外掉线。 */
    var manualDisconnect = false
        private set

    fun connectDevice(deviceId: String) {
        if (scope.state.connection == ConnectionStatus.CONNECTING || scope.connectJob?.isActive == true) {
            return
        }
        val device = scope.state.devices.firstOrNull { it.id == deviceId } ?: return
        val operationToken = beginConnection(device)
        scope.connectJob = coroutineScope.launch { connectInternal(device, operationToken) }
    }

    private suspend fun connectInternal(device: SavedDevice, operationToken: OperationGeneration) {
        UiOperationRunner.run(
            isCurrent = { scope.isConnectionCurrent(operationToken, device.id) },
            onFailure = { exception ->
                manualDisconnect = true
                client.disconnect()
                resetConnectedState(
                    ConnectionStatus.FAILED,
                    exception.message ?: exception.toString(),
                )
            },
        ) {
            val initialize = client.connect(device.serverProfile)
            val model = SessionPayloadMapper.modelState(initialize, ModelState())
            val sessionList = sessions.fetchSessions()
            if (!scope.isConnectionCurrent(operationToken, device.id)) return@run
            scope.update {
                it.copy(
                    destination = AppDestination.SESSIONS,
                    connection = ConnectionStatus.CONNECTED,
                    sessions = sessionList,
                    model = model,
                    status = "已连接",
                    error = null,
                )
            }
        }
    }

    private fun beginConnection(device: SavedDevice): OperationGeneration {
        scope.cancelConnectionOperations()
        val operationToken = scope.advanceConnection()
        manualDisconnect = true
        client.disconnect()
        manualDisconnect = false
        chatEventReducer.reset()
        scope.update {
            it.copy(
                connection = ConnectionStatus.CONNECTING,
                activeDeviceId = device.id,
                selectedSession = null,
                sessions = emptyList(),
                blocks = emptyList(),
                busy = false,
                error = null,
                model = ModelState(),
                status = "正在连接 ${device.name}",
                selectedFiles = emptyList(),
                filePickerVisible = false,
            )
        }
        return operationToken
    }

    fun disconnectAndReturnToDevices(error: String?) {
        scope.advanceConnection()
        scope.cancelConnectionOperations()
        manualDisconnect = true
        client.disconnect()
        resetConnectedState(ConnectionStatus.DISCONNECTED, error)
    }

    fun handleUnexpectedDisconnect() {
        disconnectAndReturnToDevices(error = "设备连接已断开")
    }

    private fun resetConnectedState(connection: ConnectionStatus, error: String?) {
        scope.update {
            it.copy(
                destination = AppDestination.DEVICES,
                connection = connection,
                activeDeviceId = null,
                sessions = emptyList(),
                selectedSession = null,
                blocks = emptyList(),
                busy = false,
                model = ModelState(),
                status = if (error == null) "已断开" else "设备离线",
                error = error,
                selectedFiles = emptyList(),
                filePickerVisible = false,
            )
        }
    }

    fun releaseOnCleared() {
        scope.advanceConnection()
        scope.cancelConnectionOperations()
    }
}
