package com.anyaicliremote.feature.ui

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.anyaicliremote.core.remote.AnyAICLIRemoteClient
import com.anyaicliremote.core.remote.DeviceHealthProbe
import com.anyaicliremote.core.remote.ClientRuntimeConfiguration
import com.anyaicliremote.core.session.SessionController
import com.anyaicliremote.core.storage.DeviceProfileController
import com.anyaicliremote.core.model.ChatBlock
import com.anyaicliremote.core.model.PendingInteraction
import com.anyaicliremote.core.model.InteractionAnswer
import com.anyaicliremote.core.model.ConnectionStatus
import com.anyaicliremote.core.model.SavedDevice
import com.anyaicliremote.core.model.SessionSummary
import com.anyaicliremote.core.model.WorkspaceFile
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch

/**
 * 界面状态的持有者与意图入口。具体职责由协调器承担：设备与配对在 [DeviceCoordinator]，
 * 连接在 [ConnectionCoordinator]，会话在 [SessionCoordinator]，工作区文件在
 * [WorkspaceCoordinator]，一轮对话与通知在 [TurnCoordinator]，取消与代际归属在
 * [ChatOperationScope]。
 */
class ChatViewModel(
    private val client: AnyAICLIRemoteClient,
    private val deviceHealthProbe: DeviceHealthProbe,
    profileController: DeviceProfileController?,
    initialDevices: List<SavedDevice>,
    initialError: String?,
    private val configuration: ClientRuntimeConfiguration,
) : ViewModel() {
    private val _state = MutableStateFlow(
        ChatUiState(
            devices = initialDevices,
            error = initialError,
        )
    )
    val state = _state.asStateFlow()

    private val sessionController = SessionController(client, configuration)
    private val workspaceBrowser = WorkspaceFileBrowser(viewModelScope, sessionController)
    private val chatEventReducer = ChatEventReducer()
    private val operations = ChatOperationScope(_state, workspaceBrowser, chatEventReducer)
    private val sessionCoordinator = SessionCoordinator(operations, sessionController, viewModelScope)
    private val connectionCoordinator = ConnectionCoordinator(
        operations, client, sessionCoordinator, chatEventReducer, viewModelScope,
    )
    private val deviceCoordinator = DeviceCoordinator(operations, profileController) {
        connectionCoordinator.disconnectAndReturnToDevices(error = null)
    }
    private val workspaceCoordinator = WorkspaceCoordinator(operations, workspaceBrowser)
    private val turnCoordinator = TurnCoordinator(
        operations, sessionController, chatEventReducer, sessionCoordinator, viewModelScope,
    )
    private val interactionController = InteractionController(operations, sessionController)

    init {
        viewModelScope.launch {
            client.connection.collect { connection ->
                _state.update { it.copy(connection = connection) }
                if (connection == ConnectionStatus.RECONNECTING && !connectionCoordinator.manualDisconnect) {
                    connectionCoordinator.handleUnexpectedDisconnect()
                }
            }
        }
        viewModelScope.launch {
            client.notifications.collect { notification ->
                if (client.isCurrentNotification(notification)) turnCoordinator.handleNotification(notification.message)
            }
        }
    }

    private val deviceHealthMonitor = DeviceHealthMonitor(
        scope = viewModelScope,
        configuration = configuration,
        healthProbe = deviceHealthProbe,
        devices = { state.value.devices },
        healthStatuses = { state.value.deviceHealth },
        publish = { health -> _state.update { current -> current.copy(deviceHealth = health) } },
    )

    fun startDeviceHealthMonitoring() = deviceHealthMonitor.start()

    fun stopDeviceHealthMonitoring() = deviceHealthMonitor.stop()

    fun refreshDeviceHealth() = deviceHealthMonitor.restart()

    fun updatePairing(name: String? = null, address: String? = null, key: String? = null) =
        deviceCoordinator.updatePairing(name, address, key)

    fun beginAddDevice() = deviceCoordinator.beginAddDevice()

    fun beginEditDevice(deviceId: String) = deviceCoordinator.beginEditDevice(deviceId)

    fun cancelPairing() = deviceCoordinator.cancelPairing()

    fun savePairing() = deviceCoordinator.savePairing()

    fun importPairing(address: String, key: String, name: String?) =
        deviceCoordinator.importPairing(address, key, name)

    fun deleteDevice(deviceId: String) = deviceCoordinator.deleteDevice(deviceId)

    fun connectDevice(deviceId: String) = connectionCoordinator.connectDevice(deviceId)

    fun disconnect() = connectionCoordinator.disconnectAndReturnToDevices(error = null)

    fun refreshSessions() = sessionCoordinator.refreshSessions()

    fun openSession(session: SessionSummary) = sessionCoordinator.openSession(session)

    fun closeSession() = sessionCoordinator.closeSession()

    fun createSession(workingDirectory: String) = sessionCoordinator.createSession(workingDirectory)

    fun openFilePicker() = workspaceCoordinator.openFilePicker()

    fun closeFilePicker() = workspaceCoordinator.closeFilePicker()

    fun browseWorkspace(path: String) = workspaceCoordinator.browseWorkspace(path)

    fun toggleFileAttachment(file: WorkspaceFile) = workspaceCoordinator.toggleFileAttachment(file)

    fun removeFileAttachment(path: String) = workspaceCoordinator.removeFileAttachment(path)

    fun send(text: String) = turnCoordinator.send(text)

    fun cancel() = turnCoordinator.cancel()

    fun setEffort(effort: String) = turnCoordinator.setEffort(effort)

    fun answerPermission(block: ChatBlock, optionId: String?) =
        turnCoordinator.answerPermission(block, optionId)

    fun answerInteraction(interaction: PendingInteraction, answer: InteractionAnswer) =
        interactionController.answer(interaction, answer)

    fun dismissInteraction(interaction: PendingInteraction) =
        interactionController.dismiss(interaction)

    override fun onCleared() {
        connectionCoordinator.releaseOnCleared()
        stopDeviceHealthMonitoring()
        deviceHealthProbe.close()
        client.close()
        super.onCleared()
    }
}
