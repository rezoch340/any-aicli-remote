package com.anyaicliremote.feature.ui

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.anyaicliremote.core.remote.AnyAICLIRemoteClient
import com.anyaicliremote.core.remote.ACPEventDecoder
import com.anyaicliremote.core.remote.DeviceHealthProbe
import com.anyaicliremote.core.remote.ClientRuntimeConfiguration
import com.anyaicliremote.core.session.SessionPayloadMapper
import com.anyaicliremote.core.session.SessionController
import com.anyaicliremote.core.session.CreatedSessionResponse
import com.anyaicliremote.core.storage.DeviceProfileController
import com.anyaicliremote.core.chat.AttachmentSelection
import com.anyaicliremote.core.model.ChatBlock
import com.anyaicliremote.core.model.ConnectionStatus
import com.anyaicliremote.core.model.ModelState
import com.anyaicliremote.core.model.SavedDevice
import com.anyaicliremote.core.model.SessionIdentity
import com.anyaicliremote.core.model.SessionSummary
import com.anyaicliremote.core.model.ToolRunState
import com.anyaicliremote.core.model.WorkspaceFile
import kotlinx.coroutines.Job
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import kotlinx.serialization.json.JsonObject

class ChatViewModel(
    private val client: AnyAICLIRemoteClient,
    private val deviceHealthProbe: DeviceHealthProbe,
    private val profileController: DeviceProfileController?,
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
    private val operationGenerations = OperationGenerationTracker()
    private var connectJob: Job? = null
    private var refreshSessionsJob: Job? = null
    private var openSessionJob: Job? = null
    private var createSessionJob: Job? = null
    private var promptJob: Job? = null
    private var effortJob: Job? = null
    private val sessionController = SessionController(client, configuration)
    private val workspaceBrowser = WorkspaceFileBrowser(viewModelScope, sessionController)
    private var manualDisconnect = false
    private val chatEventReducer = ChatEventReducer()

    init {
        viewModelScope.launch {
            client.connection.collect { connection ->
                _state.update { it.copy(connection = connection) }
                if (connection == ConnectionStatus.RECONNECTING && !manualDisconnect) {
                    handleUnexpectedDisconnect()
                }
            }
        }
        viewModelScope.launch {
            client.notifications.collect { notification ->
                if (client.isCurrentNotification(notification)) handleNotification(notification.message)
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

    fun updatePairing(name: String? = null, address: String? = null, key: String? = null) {
        _state.update {
            it.copy(
                deviceName = name ?: it.deviceName,
                address = address ?: it.address,
                pairingKey = key ?: it.pairingKey,
            )
        }
    }
    fun beginAddDevice() {
        _state.update {
            it.copy(
                destination = AppDestination.PAIRING,
                editingDeviceId = null,
                deviceName = "",
                address = "",
                pairingKey = "",
                error = null,
            )
        }
    }
    fun beginEditDevice(deviceId: String) {
        val device = state.value.devices.firstOrNull { it.id == deviceId } ?: return
        _state.update {
            it.copy(
                destination = AppDestination.PAIRING,
                editingDeviceId = device.id,
                deviceName = device.name,
                address = device.baseUrl,
                pairingKey = device.pairingKey,
                error = null,
            )
        }
    }
    fun cancelPairing() {
        _state.update {
            it.copy(
                destination = AppDestination.DEVICES,
                editingDeviceId = null,
                error = null,
            )
        }
    }
    fun savePairing() {
        saveDevice(
            requestedId = state.value.editingDeviceId,
            name = state.value.deviceName,
            address = state.value.address,
            pairingKey = state.value.pairingKey,
        )
    }
    fun importPairing(address: String, key: String, name: String?) {
        disconnectAndReturnToDevices(error = null)
        saveDevice(
            requestedId = null,
            name = name.orEmpty(),
            address = address,
            pairingKey = key,
        )
    }
    private fun saveDevice(requestedId: String?, name: String, address: String, pairingKey: String) {
        UiOperationRunner.runSynchronously(::showError) {
            val devices = requireProfileController().save(
                requestedId, name, address, pairingKey, state.value.devices,
            )
            _state.update { it.copy(destination = AppDestination.DEVICES, devices = devices, error = null) }
        }
    }
    fun deleteDevice(deviceId: String) {
        if (state.value.activeDeviceId == deviceId) {
            disconnectAndReturnToDevices(error = null)
        }
        UiOperationRunner.runSynchronously(::showError) {
            _state.update {
                it.copy(
                    devices = requireProfileController().delete(deviceId),
                    error = null,
                )
            }
        }
    }
    fun connectDevice(deviceId: String) {
        if (state.value.connection == ConnectionStatus.CONNECTING || connectJob?.isActive == true) {
            return
        }
        val device = state.value.devices.firstOrNull { it.id == deviceId } ?: return
        val operationToken = beginConnection(device)
        connectJob = viewModelScope.launch { connectInternal(device, operationToken) }
    }
    private suspend fun connectInternal(device: SavedDevice, operationToken: OperationGeneration) {
        UiOperationRunner.run(
            isCurrent = { isConnectionCurrent(operationToken, device.id) },
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
            val sessions = fetchSessions()
            if (!isConnectionCurrent(operationToken, device.id)) return@run
            _state.update {
                it.copy(
                    destination = AppDestination.SESSIONS,
                    connection = ConnectionStatus.CONNECTED,
                    sessions = sessions,
                    model = model,
                    status = "已连接",
                    error = null,
                )
            }
        }
    }
    fun disconnect() {
        disconnectAndReturnToDevices(error = null)
    }
    fun refreshSessions() {
        if (state.value.connection != ConnectionStatus.CONNECTED) return
        val deviceId = state.value.activeDeviceId ?: return
        val operationToken = operationGenerations.current()
        refreshSessionsJob?.cancel()
        refreshSessionsJob = viewModelScope.launch {
            UiOperationRunner.run(
                isCurrent = { isConnectionCurrent(operationToken, deviceId) },
                onFailure = ::showError,
            ) {
                val sessions = fetchSessions()
                if (isConnectionCurrent(operationToken, deviceId)) {
                    _state.update { it.copy(sessions = sessions, error = null) }
                }
            }
        }
    }
    private suspend fun fetchSessions(): List<SessionSummary> {
        return sessionController.listSessions()
    }
    fun openSession(session: SessionSummary) {
        if (state.value.connection != ConnectionStatus.CONNECTED) return
        val deviceId = state.value.activeDeviceId ?: return
        val operationToken = beginSessionOperation()
        showOpeningSession(session)
        openSessionJob = viewModelScope.launch {
            loadSessionHistory(session, deviceId, operationToken)
            if (!isSessionCurrent(operationToken, deviceId, session.identity)) return@launch
            mountSession(session, deviceId, operationToken)
        }
    }
    private fun showOpeningSession(session: SessionSummary) {
        _state.update {
            it.copy(
                destination = AppDestination.CHAT,
                selectedSession = session,
                blocks = emptyList(),
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
            isCurrent = { isSessionCurrent(operationToken, deviceId, session.identity) },
            onFailure = { updateSessionStatus("历史暂不可用：${it.message}") },
        ) {
            val history = sessionController.loadHistory(session)
            if (isSessionCurrent(operationToken, deviceId, session.identity)) {
                _state.update { it.copy(selectedSession = history.session, blocks = history.blocks, busy = false) }
            }
        }
    }
    private suspend fun mountSession(
        session: SessionSummary,
        deviceId: String,
        operationToken: OperationGeneration,
    ) {
        UiOperationRunner.run(
            isCurrent = { isSessionCurrent(operationToken, deviceId, session.identity) },
            onFailure = { updateSessionStatus("挂载失败：${it.message}") },
        ) {
            val model = sessionController.mount(session, state.value.model)
            if (isSessionCurrent(operationToken, deviceId, session.identity)) {
                _state.update { it.copy(model = model, status = "在线") }
            }
        }
    }
    private fun updateSessionStatus(status: String) {
        _state.update { it.copy(status = status) }
    }
    fun closeSession() {
        invalidateSessionOperations()
        _state.update {
            it.copy(
                destination = AppDestination.SESSIONS,
                selectedSession = null,
                blocks = emptyList(),
                busy = false,
                selectedFiles = emptyList(),
                filePickerVisible = false,
            )
        }
    }
    fun openFilePicker() {
        val session = state.value.selectedSession ?: return
        if (state.value.connection != ConnectionStatus.CONNECTED) return
        _state.update { it.copy(filePickerVisible = true, filePickerError = null) }
        loadWorkspaceFiles(session, ".")
    }
    fun closeFilePicker() {
        workspaceBrowser.cancel()
        _state.update { it.copy(filePickerVisible = false, filePickerLoading = false) }
    }
    fun browseWorkspace(path: String) {
        val session = state.value.selectedSession ?: return
        if (!state.value.filePickerVisible) return
        loadWorkspaceFiles(session, path)
    }
    fun toggleFileAttachment(file: WorkspaceFile) {
        if (file.directory) return
        _state.update { current ->
            val selection = AttachmentSelection(current.selectedFiles).toggle(file)
            current.copy(selectedFiles = selection.files)
        }
    }
    fun removeFileAttachment(path: String) {
        _state.update { current ->
            current.copy(selectedFiles = current.selectedFiles.filterNot { it.path == path })
        }
    }
    private fun loadWorkspaceFiles(session: SessionSummary, path: String) {
        val deviceId = state.value.activeDeviceId ?: return
        val operationToken = operationGenerations.current()
        workspaceBrowser.load(
            WorkspaceLoadRequest(
                session = session,
                path = path,
                isCurrent = {
                    isSessionCurrent(operationToken, deviceId, session.identity)
                },
                onLoading = {
                    _state.update {
                        it.copy(
                            filePickerPath = path,
                            filePickerLoading = true,
                            filePickerError = null,
                        )
                    }
                },
                onLoaded = { listing ->
                    _state.update {
                        it.copy(
                            filePickerPath = listing.path,
                            filePickerParent = listing.parent,
                            filePickerDirectories = listing.directories,
                            filePickerFiles = listing.files,
                            filePickerLoading = false,
                        )
                    }
                },
                onFailure = { message ->
                    _state.update {
                        it.copy(filePickerLoading = false, filePickerError = message)
                    }
                },
            ),
        )
    }
    fun createSession(workingDirectory: String) {
        if (state.value.connection != ConnectionStatus.CONNECTED) return
        val deviceId = state.value.activeDeviceId ?: return
        val token = beginSessionOperation()
        val known = state.value.sessions.map(SessionSummary::identity).toSet()
        _state.update { it.copy(status = "正在创建会话", error = null) }
        createSessionJob = viewModelScope.launch {
            UiOperationRunner.run(
                isCurrent = { isSessionOperationCurrent(token, deviceId) },
                onFailure = ::showError,
            ) {
                val created = sessionController.create(workingDirectory)
                if (isSessionOperationCurrent(token, deviceId)) {
                    showCreatedSession(created, known)
                }
            }
        }
    }
    private fun showCreatedSession(created: CreatedSessionResponse, known: Set<SessionIdentity>) {
        val session = created.sessions.singleOrNull { it.id == created.identifier && it.identity !in known }
            ?: created.responseSession?.takeIf { it.id == created.identifier }
        _state.update {
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
    fun send(text: String) {
        val session = state.value.selectedSession ?: return
        val deviceId = state.value.activeDeviceId ?: return
        if (state.value.connection != ConnectionStatus.CONNECTED || state.value.busy) return
        val payload = chatEventReducer.promptPayload(text, state.value.selectedFiles) ?: return
        val operationToken = operationGenerations.current()
        val turnStart = chatEventReducer.beginTurn(state.value, payload)
        _state.value = turnStart.state
        val turnId = turnStart.identifier
        promptJob?.cancel()
        promptJob = viewModelScope.launch {
            UiOperationRunner.run(
                isCurrent = { isActivePrompt(operationToken, deviceId, session, turnId) },
                onFailure = { exception ->
                    _state.update {
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
        isSessionCurrent(operationToken, deviceId, session.identity) &&
            chatEventReducer.isActive(turnId)

    fun cancel() {
        val session = state.value.selectedSession ?: return
        promptJob?.cancel()
        promptJob = null
        sessionController.cancel(session)
        finishTurn(ToolRunState.CANCELLED, "已停止")
    }
    fun setEffort(effort: String) {
        val session = state.value.selectedSession ?: return
        val deviceId = state.value.activeDeviceId ?: return
        val operationToken = operationGenerations.current()
        effortJob?.cancel()
        effortJob = viewModelScope.launch {
            UiOperationRunner.run(
                isCurrent = { isSessionCurrent(operationToken, deviceId, session.identity) },
                onFailure = ::showError,
            ) {
                sessionController.setEffort(session, state.value.model.currentModelId, effort)
                if (isSessionCurrent(operationToken, deviceId, session.identity)) {
                    _state.update { it.copy(model = it.model.copy(effort = effort)) }
                }
            }
        }
    }
    fun answerPermission(block: ChatBlock, optionId: String?) {
        val requestId = block.rpcId ?: return
        sessionController.answerPermission(requestId, optionId)
        _state.update { it.copy(blocks = it.blocks.filterNot { item -> item.id == block.id }) }
    }
    private fun handleNotification(message: JsonObject) {
        val event = ACPEventDecoder.decode(message) ?: return
        val reduction = chatEventReducer.reduceNotification(state.value, event)
        _state.value = reduction.state
        if (reduction.action == ChatEventAction.RefreshSessions) refreshSessions()
    }
    private fun finishTurn(toolState: ToolRunState, status: String) {
        _state.update { chatEventReducer.finishTurn(it, toolState, status) }
    }
    private fun beginConnection(device: SavedDevice): OperationGeneration {
        cancelConnectionOperations()
        val operationToken = operationGenerations.advanceConnection()
        manualDisconnect = true
        client.disconnect()
        manualDisconnect = false
        chatEventReducer.reset()
        _state.update {
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
    private fun beginSessionOperation(): OperationGeneration {
        invalidateSessionOperations()
        return operationGenerations.current()
    }
    private fun invalidateSessionOperations() {
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
    private fun cancelConnectionOperations() {
        connectJob?.cancel()
        connectJob = null
        refreshSessionsJob?.cancel()
        refreshSessionsJob = null
        invalidateSessionOperations()
    }
    private fun disconnectAndReturnToDevices(error: String?) {
        operationGenerations.advanceConnection()
        cancelConnectionOperations()
        manualDisconnect = true
        client.disconnect()
        resetConnectedState(ConnectionStatus.DISCONNECTED, error)
    }
    private fun handleUnexpectedDisconnect() {
        disconnectAndReturnToDevices(error = "设备连接已断开")
    }
    private fun isConnectionCurrent(operationToken: OperationGeneration, deviceId: String): Boolean =
        operationGenerations.isConnectionCurrent(operationToken) && state.value.activeDeviceId == deviceId
    private fun isSessionOperationCurrent(operationToken: OperationGeneration, deviceId: String): Boolean =
        operationGenerations.isSessionCurrent(operationToken) &&
            state.value.activeDeviceId == deviceId &&
            state.value.connection == ConnectionStatus.CONNECTED
    private fun isSessionCurrent(
        operationToken: OperationGeneration,
        deviceId: String,
        sessionIdentity: SessionIdentity,
    ): Boolean =
        isSessionOperationCurrent(operationToken, deviceId) &&
            state.value.destination == AppDestination.CHAT &&
            state.value.selectedSession?.identity == sessionIdentity
    private fun requireProfileController(): DeviceProfileController =
        profileController ?: error("安全存储不可用，请重启应用后重试")

    private fun resetConnectedState(connection: ConnectionStatus, error: String?) {
        _state.update {
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
    private fun showError(error: Throwable) {
        _state.update {
            it.copy(
                error = error.message ?: error.toString(),
                status = error.message.orEmpty(),
            )
        }
    }

    override fun onCleared() {
        operationGenerations.advanceConnection()
        cancelConnectionOperations()
        stopDeviceHealthMonitoring()
        deviceHealthProbe.close()
        client.close()
        super.onCleared()
    }
}
