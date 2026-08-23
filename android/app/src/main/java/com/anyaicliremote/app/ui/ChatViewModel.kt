package com.anyaicliremote.app.ui

import android.app.Application
import androidx.lifecycle.AndroidViewModel
import androidx.lifecycle.viewModelScope
import com.anyaicliremote.app.data.AnyAICLIRemoteClient
import com.anyaicliremote.app.data.ACPWire
import com.anyaicliremote.app.data.DeviceHealthProbe
import com.anyaicliremote.app.data.ClientRuntimeConfiguration
import com.anyaicliremote.app.data.SecureProfileStore
import com.anyaicliremote.app.model.ChatBlock
import com.anyaicliremote.app.model.ChatBlockKind
import com.anyaicliremote.app.model.ConnectionStatus
import com.anyaicliremote.app.model.DeviceHealthStatus
import com.anyaicliremote.app.model.ModelState
import com.anyaicliremote.app.model.PermissionOption
import com.anyaicliremote.app.model.SavedDevice
import com.anyaicliremote.app.model.ServerProfile
import com.anyaicliremote.app.model.SessionMessage
import com.anyaicliremote.app.model.SessionIdentity
import com.anyaicliremote.app.model.SessionSummary
import com.anyaicliremote.app.model.ToolRunState
import com.anyaicliremote.app.model.WorkspaceFile
import com.anyaicliremote.app.model.obj
import com.anyaicliremote.app.model.objects
import com.anyaicliremote.app.model.string
import com.anyaicliremote.app.model.toolState
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.Job
import kotlinx.coroutines.async
import kotlinx.coroutines.awaitAll
import kotlinx.coroutines.coroutineScope
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import kotlinx.coroutines.isActive
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.contentOrNull
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.longOrNull
import kotlinx.serialization.json.put
import java.util.UUID

data class ChatUiState(
    val destination: AppDestination = AppDestination.DEVICES,
    val connection: ConnectionStatus = ConnectionStatus.DISCONNECTED,
    val devices: List<SavedDevice> = emptyList(),
    val activeDeviceId: String? = null,
    val deviceHealth: Map<String, DeviceHealthStatus> = emptyMap(),
    val editingDeviceId: String? = null,
    val deviceName: String = "",
    val address: String = "",
    val pairingKey: String = "",
    val sessions: List<SessionSummary> = emptyList(),
    val selectedSession: SessionSummary? = null,
    val blocks: List<ChatBlock> = emptyList(),
    val busy: Boolean = false,
    val status: String = "",
    val error: String? = null,
    val model: ModelState = ModelState(),
    val selectedFiles: List<WorkspaceFile> = emptyList(),
    val filePickerVisible: Boolean = false,
    val filePickerPath: String = ".",
    val filePickerParent: String? = null,
    val filePickerDirectories: List<WorkspaceFile> = emptyList(),
    val filePickerFiles: List<WorkspaceFile> = emptyList(),
    val filePickerLoading: Boolean = false,
    val filePickerError: String? = null,
)

enum class AppDestination { DEVICES, PAIRING, SESSIONS, CHAT }

class ChatViewModel @JvmOverloads constructor(
    application: Application,
    private val configuration: ClientRuntimeConfiguration = ClientRuntimeConfiguration.Default,
) : AndroidViewModel(application) {
    private val client = AnyAICLIRemoteClient(configuration)
    private val deviceHealthProbe = DeviceHealthProbe(configuration)
    private val profileStoreInitialization = runCatching { SecureProfileStore(application) }
    private val profileStore = profileStoreInitialization.getOrNull()
    private val initialDevices = profileStore?.let { runCatching { it.loadDevices() } }
    private val _state = MutableStateFlow(
        ChatUiState(
            devices = initialDevices?.getOrNull().orEmpty(),
            error = profileStoreInitialization.exceptionOrNull()?.message
                ?: initialDevices?.exceptionOrNull()?.message
                ?: if (profileStore?.recoveredCorruptedStorage == true) "已清理无法读取的旧配对信息，请重新配对设备" else null,
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
    private var fileBrowserJob: Job? = null
    private var deviceHealthMonitorJob: Job? = null
    private var manualDisconnect = false
    private var activeTurnId: String? = null
    private val pendingUserEchoTracker = PendingUserEchoTracker()

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

    fun startDeviceHealthMonitoring() {
        if (deviceHealthMonitorJob?.isActive == true) return
        deviceHealthMonitorJob = viewModelScope.launch {
            while (isActive) {
                probeSavedDevices()
                delay(configuration.healthPollingInterval)
            }
        }
    }

    fun stopDeviceHealthMonitoring() {
        deviceHealthMonitorJob?.cancel()
        deviceHealthMonitorJob = null
    }

    fun refreshDeviceHealth() {
        stopDeviceHealthMonitoring()
        startDeviceHealthMonitoring()
    }

    private suspend fun probeSavedDevices() {
        val devices = state.value.devices
        if (devices.isEmpty()) {
            _state.update { it.copy(deviceHealth = emptyMap()) }
            return
        }
        val addressesById = devices.associate { it.id to it.baseUrl }
        _state.update { current ->
            current.copy(
                deviceHealth = devices.associate { device ->
                    device.id to (current.deviceHealth[device.id] ?: DeviceHealthStatus.CHECKING)
                }
            )
        }
        val results = coroutineScope {
            devices.map { device ->
                async {
                    val status = if (deviceHealthProbe.isOnline(device.baseUrl)) {
                        DeviceHealthStatus.ONLINE
                    } else {
                        DeviceHealthStatus.OFFLINE
                    }
                    device.id to status
                }
            }.awaitAll().toMap()
        }
        _state.update { current ->
            val currentAddresses = current.devices.associate { it.id to it.baseUrl }
            current.copy(
                deviceHealth = current.devices.associate { device ->
                    val status = results[device.id]
                        ?.takeIf { addressesById[device.id] == currentAddresses[device.id] }
                        ?: current.deviceHealth[device.id]
                        ?: DeviceHealthStatus.CHECKING
                    device.id to status
                }
            )
        }
    }

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
        _state.update { it.copy(destination = AppDestination.DEVICES, editingDeviceId = null, error = null) }
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

    private fun saveDevice(
        requestedId: String?,
        name: String,
        address: String,
        pairingKey: String,
    ) {
        try {
            val normalizedProfile = ServerProfile.parse(address, pairingKey)
            val matchingDevice = state.value.devices.firstOrNull { it.baseUrl == normalizedProfile.baseUrl }
            require(requestedId == null || matchingDevice == null || matchingDevice.id == requestedId) {
                "该服务地址已由设备“${matchingDevice?.name}”使用"
            }
            val deviceId = requestedId ?: matchingDevice?.id ?: UUID.randomUUID().toString()
            val resolvedName = name.trim().ifEmpty { matchingDevice?.name.orEmpty() }
            val device = SavedDevice.normalized(
                deviceId,
                resolvedName,
                normalizedProfile.baseUrl,
                normalizedProfile.key,
            )
            val savedDevices = requireProfileStore().saveDevice(device)
            _state.update {
                it.copy(
                    destination = AppDestination.DEVICES,
                    devices = savedDevices,
                    editingDeviceId = null,
                    deviceName = "",
                    address = "",
                    pairingKey = "",
                    error = null,
                    status = "已保存 ${device.name}",
                )
            }
        } catch (error: RuntimeException) {
            showError(error)
        }
    }

    fun deleteDevice(deviceId: String) {
        if (state.value.activeDeviceId == deviceId) disconnectAndReturnToDevices(error = null)
        try {
            val savedDevices = requireProfileStore().deleteDevice(deviceId)
            _state.update { it.copy(devices = savedDevices, error = null) }
        } catch (error: RuntimeException) {
            showError(error)
        }
    }

    fun connectDevice(deviceId: String) {
        if (state.value.connection == ConnectionStatus.CONNECTING || connectJob?.isActive == true) return
        val device = state.value.devices.firstOrNull { it.id == deviceId } ?: return
        val operationToken = beginConnection(device)
        connectJob = viewModelScope.launch { connectInternal(device, operationToken) }
    }

    private suspend fun connectInternal(device: SavedDevice, operationToken: OperationGeneration) {
        try {
            val initialize = client.connect(device.serverProfile)
            val model = modelStateFrom(initialize, ModelState())
            val sessions = fetchSessions()
            if (!isConnectionCurrent(operationToken, device.id)) return
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
        } catch (error: CancellationException) {
            throw error
        } catch (error: Throwable) {
            if (!isConnectionCurrent(operationToken, device.id)) return
            manualDisconnect = true
            client.disconnect()
            resetConnectedState(ConnectionStatus.FAILED, error.message ?: error.toString())
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
            try {
                val sessions = fetchSessions()
                if (isConnectionCurrent(operationToken, deviceId)) {
                    _state.update { it.copy(sessions = sessions, error = null) }
                }
            } catch (error: CancellationException) {
                throw error
            } catch (error: Throwable) {
                if (isConnectionCurrent(operationToken, deviceId)) showError(error)
            }
        }
    }

    private suspend fun fetchSessions(): List<SessionSummary> {
        val response = client.rest("/api/sessions")
        return response.objects("sessions").mapNotNull(SessionSummary::from)
            .sortedWith(compareByDescending<SessionSummary> { it.resident }.thenByDescending { it.updatedAt })
    }

    fun openSession(session: SessionSummary) {
        if (state.value.connection != ConnectionStatus.CONNECTED) return
        val deviceId = state.value.activeDeviceId ?: return
        val operationToken = beginSessionOperation()
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
        openSessionJob = viewModelScope.launch {
            try {
                val history = client.rest(
                    pathSegments = listOf("api", "sessions", session.id, "messages"),
                    query = mapOf(
                        "providerId" to session.providerId,
                    ),
                )
                if (!isSessionCurrent(operationToken, deviceId, session.identity)) return@launch
                val authoritativeSession = history.obj("session")?.let(SessionSummary::from)
                    ?.takeIf { metadata ->
                        metadata.identity == session.identity
                    } ?: session
                val historyBlocks = history.objects("messages").mapNotNull(SessionMessage::from)
                    .mapIndexed { index, message -> historyBlock(authoritativeSession, index, message) }
                _state.update { current ->
                    if (!isSessionCurrent(operationToken, deviceId, session.identity)) current
                    else current.copy(selectedSession = authoritativeSession, blocks = historyBlocks, busy = false)
                }
            } catch (error: CancellationException) {
                throw error
            } catch (error: Throwable) {
                if (isSessionCurrent(operationToken, deviceId, session.identity)) {
                    _state.update { current -> current.copy(status = "历史暂不可用：${error.message}") }
                } else {
                    return@launch
                }
            }

            if (!isSessionCurrent(operationToken, deviceId, session.identity)) return@launch
            try {
                val loaded = client.rpc(ACPWire.loadSessionMethod, buildJsonObject {
                    put("sessionId", session.id)
                    put("mcpServers", JsonArray(emptyList()))
                }, configuration.sessionLoadTimeout.inWholeMilliseconds)
                if (!isSessionCurrent(operationToken, deviceId, session.identity)) return@launch
                val model = (loaded as? JsonObject)?.let { modelStateFrom(it, state.value.model) }
                    ?: state.value.model
                _state.update { it.copy(model = model, status = "在线") }
            } catch (error: CancellationException) {
                throw error
            } catch (error: Throwable) {
                if (isSessionCurrent(operationToken, deviceId, session.identity)) {
                    _state.update { current -> current.copy(status = "挂载失败：${error.message}") }
                }
            }
        }
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
        fileBrowserJob?.cancel()
        fileBrowserJob = null
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
        fileBrowserJob?.cancel()
        _state.update {
            it.copy(
                filePickerPath = path,
                filePickerLoading = true,
                filePickerError = null,
            )
        }
        fileBrowserJob = viewModelScope.launch {
            try {
                val response = client.rest(
                    "/api/fs/list",
                    query = mapOf(
                        "providerId" to session.providerId,
                        "sessionId" to session.id,
                        "path" to path,
                    ),
                )
                if (!isSessionCurrent(operationToken, deviceId, session.identity)) return@launch
                _state.update {
                    it.copy(
                        filePickerPath = response.string("path") ?: path,
                        filePickerParent = response.string("parent"),
                        filePickerDirectories = response.objects("dirs").mapNotNull { item -> WorkspaceFile.from(item, true) },
                        filePickerFiles = response.objects("files").mapNotNull { item -> WorkspaceFile.from(item, false) },
                        filePickerLoading = false,
                    )
                }
            } catch (error: CancellationException) {
                throw error
            } catch (error: Throwable) {
                if (isSessionCurrent(operationToken, deviceId, session.identity)) {
                    _state.update { it.copy(filePickerLoading = false, filePickerError = error.message ?: "无法读取工作区") }
                }
            }
        }
    }

    fun createSession(cwd: String) {
        if (state.value.connection != ConnectionStatus.CONNECTED) return
        val deviceId = state.value.activeDeviceId ?: return
        val operationToken = beginSessionOperation()
        val knownSessionKeys = state.value.sessions.map(SessionSummary::identity).toSet()
        _state.update { it.copy(status = "正在创建会话", error = null) }
        createSessionJob = viewModelScope.launch {
            try {
                val raw = client.rpc(
                    ACPWire.newSessionMethod,
                    ACPWire.newSessionParameters(cwd),
                    configuration.sessionCreateTimeout.inWholeMilliseconds,
                ).jsonObject
                val id = raw.string("sessionId", "session_id") ?: raw.obj("session")?.string("sessionId")
                    ?: error("session/new 未返回 sessionId")
                val sessions = fetchSessions()
                if (!isSessionOperationCurrent(operationToken, deviceId)) return@launch
                val indexedSession = sessions.singleOrNull { candidate ->
                    candidate.id == id && candidate.identity !in knownSessionKeys
                }
                val responseSession = (raw.obj("session") ?: raw).let(SessionSummary::from)
                    ?.takeIf { candidate -> candidate.id == id }
                val session = indexedSession ?: responseSession
                _state.update {
                    if (session == null) {
                        it.copy(
                            destination = AppDestination.SESSIONS,
                            selectedSession = null,
                            sessions = sessions,
                            blocks = emptyList(),
                            status = "会话已创建，等待历史索引",
                        )
                    } else {
                        it.copy(
                            destination = AppDestination.CHAT,
                            selectedSession = session,
                            sessions = sessions,
                            blocks = emptyList(),
                            status = "在线",
                        )
                    }
                }
            } catch (error: CancellationException) {
                throw error
            } catch (error: Throwable) {
                if (isSessionOperationCurrent(operationToken, deviceId)) showError(error)
            }
        }
    }

    fun send(text: String) {
        val session = state.value.selectedSession ?: return
        val deviceId = state.value.activeDeviceId ?: return
        if (state.value.connection != ConnectionStatus.CONNECTED || state.value.busy) return
        val message = text.trim()
        val attachments = state.value.selectedFiles
        if (message.isEmpty() && attachments.isEmpty()) return
        val operationToken = operationGenerations.current()
        val turnId = UUID.randomUUID().toString()
        activeTurnId = turnId
        pendingUserEchoTracker.clear()
        message.takeIf { it.isNotEmpty() }?.let(pendingUserEchoTracker::begin)
        append(
            ChatBlock(
                id = UUID.randomUUID().toString(),
                kind = ChatBlockKind.USER,
                text = message,
                attachments = attachments,
            )
        )
        _state.update { it.copy(busy = true, status = "等待助手", selectedFiles = emptyList()) }
        promptJob?.cancel()
        promptJob = viewModelScope.launch {
            try {
                client.rpc(
                    ACPWire.promptMethod,
                    ACPWire.promptParameters(session.id, message, attachments),
                )
                if (isSessionCurrent(operationToken, deviceId, session.identity) && activeTurnId == turnId) {
                    finishTurn(ToolRunState.SUCCESS, "完成")
                }
            } catch (error: CancellationException) {
                throw error
            } catch (error: Throwable) {
                if (!isSessionCurrent(operationToken, deviceId, session.identity) || activeTurnId != turnId) return@launch
                if (!error.message.orEmpty().contains("cancel", ignoreCase = true)) {
                    append(ChatBlock(UUID.randomUUID().toString(), ChatBlockKind.SYSTEM, text = error.message ?: error.toString()))
                    finishTurn(ToolRunState.FAILED, "发送失败")
                } else {
                    finishTurn(ToolRunState.CANCELLED, "已停止")
                }
            }
        }
    }

    fun cancel() {
        val session = state.value.selectedSession ?: return
        promptJob?.cancel()
        promptJob = null
        client.notify(ACPWire.cancelMethod, ACPWire.cancelParameters(session.id))
        finishTurn(ToolRunState.CANCELLED, "已停止")
    }

    fun setEffort(effort: String) {
        val session = state.value.selectedSession ?: return
        val deviceId = state.value.activeDeviceId ?: return
        val operationToken = operationGenerations.current()
        effortJob?.cancel()
        effortJob = viewModelScope.launch {
            try {
                client.rest("/api/effort", "POST", body = buildJsonObject {
                    put("providerId", session.providerId)
                    put("sessionId", session.id)
                    put("modelId", state.value.model.currentModelId)
                    put("effort", effort)
                })
                if (isSessionCurrent(operationToken, deviceId, session.identity)) {
                    _state.update { it.copy(model = it.model.copy(effort = effort)) }
                }
            } catch (error: CancellationException) {
                throw error
            } catch (error: Throwable) {
                if (isSessionCurrent(operationToken, deviceId, session.identity)) showError(error)
            }
        }
    }

    fun answerPermission(block: ChatBlock, optionId: String?) {
        val rpcId = block.rpcId ?: return
        val result = buildJsonObject {
            put("outcome", buildJsonObject {
                if (optionId == null) put("outcome", "cancelled")
                else { put("outcome", "selected"); put("optionId", optionId) }
            })
        }
        client.reply(rpcId, result)
        _state.update { it.copy(blocks = it.blocks.filterNot { item -> item.id == block.id }) }
    }

    private fun handleNotification(message: JsonObject) {
        val method = message.string("method").orEmpty()
        when {
            method == "session/update" -> {
                val currentState = state.value
                val selectedSession = currentState.selectedSession
                if (
                    currentState.connection != ConnectionStatus.CONNECTED ||
                    currentState.destination != AppDestination.CHAT ||
                    selectedSession == null
                ) return
                val params = message.obj("params") ?: JsonObject(emptyMap())
                if (matchesSessionIdentity(params, selectedSession.identity)) {
                    applyUpdate(params.obj("update") ?: params)
                }
            }
            method == "sessions/changed" && state.value.destination == AppDestination.SESSIONS -> refreshSessions()
            method.contains("permission") || method.contains("ask_user") -> {
                val currentState = state.value
                val selectedSession = currentState.selectedSession
                if (
                    currentState.connection != ConnectionStatus.CONNECTED ||
                    currentState.destination != AppDestination.CHAT ||
                    selectedSession == null
                ) return
                val rpcId = (message["id"] as? JsonPrimitive)?.longOrNull ?: return
                val params = message.obj("params") ?: JsonObject(emptyMap())
                if (!matchesSessionIdentity(params, selectedSession.identity)) return
                val options = params.objects("options").map {
                    PermissionOption(it.string("optionId", "id") ?: "allow", it.string("name", "label") ?: "允许")
                }.ifEmpty { listOf(PermissionOption("allow", "允许")) }
                append(ChatBlock(
                    id = "permission-$rpcId",
                    kind = ChatBlockKind.PERMISSION,
                    text = params.string("question", "message") ?: "AI CLI 需要你的确认",
                    rpcId = rpcId,
                    options = options,
                ))
            }
        }
    }

    private fun applyUpdate(update: JsonObject) {
        val type = update.string("sessionUpdate").orEmpty()
        val text = update.obj("content")?.string("text") ?: update.string("text").orEmpty()
        when (type) {
            "user_message_chunk" -> if (!consumePendingUserEcho(text)) appendChunk(ChatBlockKind.USER, text)
            "agent_message_chunk" -> {
                appendChunk(ChatBlockKind.ASSISTANT, text)
                if (activeTurnId != null) _state.update { it.copy(busy = true, status = "正在回复") }
            }
            "agent_thought_chunk" -> {
                appendChunk(ChatBlockKind.THINKING, text)
                if (activeTurnId != null) _state.update { it.copy(busy = true, status = "正在思考") }
            }
            "tool_call", "tool_call_update" -> {
                upsertTool(update)
                if (activeTurnId != null) {
                    _state.update { it.copy(busy = true, status = update.string("title", "toolName", "kind") ?: "正在使用工具") }
                }
            }
            "plan" -> appendChunk(ChatBlockKind.PLAN, text.ifEmpty { "Plan" })
            "session_recap" -> appendChunk(ChatBlockKind.SYSTEM, text)
            "turn_completed", "task_completed" -> {
                finishTurn(ToolRunState.SUCCESS, "完成")
            }
            "cancelled", "turn_cancelled", "task_cancelled" -> {
                finishTurn(ToolRunState.CANCELLED, "已停止")
            }
            "turn_failed", "task_failed", "failed", "error" -> {
                finishTurn(ToolRunState.FAILED, "执行失败")
            }
        }
    }

    private fun consumePendingUserEcho(chunk: String): Boolean = pendingUserEchoTracker.consume(chunk)

    private fun appendChunk(kind: ChatBlockKind, text: String) {
        if (text.isEmpty()) return
        _state.update { current ->
            val blocks = current.blocks.toMutableList()
            val last = blocks.lastOrNull()
            if (last?.kind == kind && kind in setOf(ChatBlockKind.USER, ChatBlockKind.ASSISTANT, ChatBlockKind.THINKING)) {
                blocks[blocks.lastIndex] = last.copy(text = last.text + text)
            } else {
                blocks += ChatBlock(UUID.randomUUID().toString(), kind, text = text)
            }
            current.copy(blocks = blocks)
        }
    }

    private fun upsertTool(update: JsonObject) {
        val id = update.string("toolCallId", "tool_call_id", "id") ?: UUID.randomUUID().toString()
        val blockId = "tool-$id"
        val title = update.string("title", "toolName", "kind")
        val rawStatus = update.string("status", "toolStatus")
        val detail = update.string("result") ?: update.obj("content")?.string("text") ?: ""
        _state.update { current ->
            val blocks = current.blocks.toMutableList()
            val index = blocks.indexOfFirst { it.id == blockId }
            if (index >= 0) {
                val previous = blocks[index]
                blocks[index] = previous.copy(
                    title = title ?: previous.title,
                    detail = detail.ifEmpty { previous.detail },
                    toolState = rawStatus?.let(::toolState) ?: previous.toolState,
                )
            } else {
                blocks += ChatBlock(
                    blockId,
                    ChatBlockKind.TOOL,
                    title = title ?: "工具",
                    detail = detail,
                    toolState = toolState(rawStatus),
                )
            }
            current.copy(blocks = blocks)
        }
    }

    private fun resetTurnTracking() {
        activeTurnId = null
        pendingUserEchoTracker.clear()
    }

    private fun finishTurn(toolState: ToolRunState, status: String) {
        resetTurnTracking()
        _state.update { current ->
            current.copy(
                blocks = finalizeActiveTools(current.blocks, toolState),
                busy = false,
                status = status,
            )
        }
    }

    private fun beginConnection(device: SavedDevice): OperationGeneration {
        cancelConnectionOperations()
        val operationToken = operationGenerations.advanceConnection()
        manualDisconnect = true
        client.disconnect()
        manualDisconnect = false
        resetTurnTracking()
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
        fileBrowserJob?.cancel()
        fileBrowserJob = null
        resetTurnTracking()
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

    private fun requireProfileStore(): SecureProfileStore =
        profileStore ?: throw IllegalStateException("安全存储不可用，请重启应用后重试")

    private fun modelStateFrom(objectValue: JsonObject, fallback: ModelState): ModelState {
        val source = objectValue.obj("models")
            ?: objectValue.obj("_meta")?.obj("modelState")
            ?: objectValue.obj("modelState")
            ?: return fallback
        var model = fallback
        source.string("currentModelId")?.let { model = model.copy(currentModelId = it) }
        val current = source.objects("availableModels").firstOrNull { it.string("modelId") == model.currentModelId }
        val meta = current?.obj("_meta")
        meta?.string("reasoningEffort")?.let { model = model.copy(effort = it) }
        val levels = meta?.objects("reasoningEfforts")?.mapNotNull { it.string("value", "id") }.orEmpty()
        if (levels.isNotEmpty()) model = model.copy(effortLevels = levels)
        return model
    }

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

    private fun historyBlock(session: SessionSummary, index: Int, message: SessionMessage): ChatBlock {
        val blockID = "history:${session.providerId}:${session.id}:$index"
        return when (message.role) {
            "user" -> ChatBlock(blockID, ChatBlockKind.USER, text = message.content)
            "assistant" -> ChatBlock(blockID, ChatBlockKind.ASSISTANT, text = message.content)
            "tool" -> ChatBlock(
                id = blockID,
                kind = ChatBlockKind.TOOL,
                title = "工具",
                detail = message.content,
                toolState = ToolRunState.SUCCESS,
            )
            else -> ChatBlock(blockID, ChatBlockKind.SYSTEM, text = message.content)
        }
    }

    private fun append(block: ChatBlock) { _state.update { it.copy(blocks = it.blocks + block) } }
    private fun showError(error: Throwable) { _state.update { it.copy(error = error.message ?: error.toString(), status = error.message.orEmpty()) } }

    override fun onCleared() {
        operationGenerations.advanceConnection()
        cancelConnectionOperations()
        stopDeviceHealthMonitoring()
        deviceHealthProbe.close()
        client.close()
        super.onCleared()
    }
}

internal fun matchesSessionIdentity(params: JsonObject, expected: SessionIdentity): Boolean =
    params.string("providerId") == expected.providerId &&
        params.string("sessionId") == expected.sessionId

internal fun finalizeActiveTools(
    blocks: List<ChatBlock>,
    finalState: ToolRunState,
): List<ChatBlock> = blocks.map { block ->
    if (
        block.kind == ChatBlockKind.TOOL &&
        block.toolState in setOf(ToolRunState.PENDING, ToolRunState.RUNNING)
    ) {
        block.copy(toolState = finalState)
    } else {
        block
    }
}

internal data class OperationGeneration(
    val connection: Long,
    val session: Long,
)

internal class OperationGenerationTracker {
    private var connectionGeneration = 0L
    private var sessionGeneration = 0L

    fun current(): OperationGeneration = OperationGeneration(connectionGeneration, sessionGeneration)

    fun advanceConnection(): OperationGeneration {
        connectionGeneration += 1
        sessionGeneration += 1
        return current()
    }

    fun advanceSession(): OperationGeneration {
        sessionGeneration += 1
        return current()
    }

    fun isConnectionCurrent(operationGeneration: OperationGeneration): Boolean =
        operationGeneration.connection == connectionGeneration

    fun isSessionCurrent(operationGeneration: OperationGeneration): Boolean =
        isConnectionCurrent(operationGeneration) && operationGeneration.session == sessionGeneration
}
