package com.grokremote.app.ui

import android.app.Application
import androidx.lifecycle.AndroidViewModel
import androidx.lifecycle.viewModelScope
import com.grokremote.app.data.GrokRemoteClient
import com.grokremote.app.data.SecureProfileStore
import com.grokremote.app.model.ChatBlock
import com.grokremote.app.model.ChatBlockKind
import com.grokremote.app.model.ConnectionStatus
import com.grokremote.app.model.ModelState
import com.grokremote.app.model.PermissionOption
import com.grokremote.app.model.ServerProfile
import com.grokremote.app.model.SessionSummary
import com.grokremote.app.model.ToolRunState
import com.grokremote.app.model.obj
import com.grokremote.app.model.objects
import com.grokremote.app.model.string
import com.grokremote.app.model.toolState
import kotlinx.coroutines.Job
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.isActive
import kotlinx.coroutines.launch
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.buildJsonArray
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.contentOrNull
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.longOrNull
import kotlinx.serialization.json.put
import java.util.UUID

data class ChatUiState(
    val connection: ConnectionStatus = ConnectionStatus.DISCONNECTED,
    val address: String = "",
    val pairingKey: String = "",
    val defaultCwd: String = "~",
    val sessions: List<SessionSummary> = emptyList(),
    val selectedSession: SessionSummary? = null,
    val blocks: List<ChatBlock> = emptyList(),
    val busy: Boolean = false,
    val status: String = "",
    val error: String? = null,
    val model: ModelState = ModelState(),
)

class ChatViewModel(application: Application) : AndroidViewModel(application) {
    private val client = GrokRemoteClient()
    private val profileStore = SecureProfileStore(application)
    private val savedProfile = profileStore.load()
    private val _state = MutableStateFlow(
        ChatUiState(
            address = savedProfile?.baseUrl.orEmpty(),
            pairingKey = savedProfile?.key.orEmpty(),
            defaultCwd = profileStore.defaultCwd(),
        )
    )
    val state = _state.asStateFlow()

    private var profile: ServerProfile? = savedProfile
    private var reconnectJob: Job? = null
    private var manualDisconnect = false
    private var activeTurnId: String? = null
    private var pendingUserEcho: String? = null
    private var pendingUserEchoOffset = 0

    init {
        viewModelScope.launch {
            client.connection.collect { connection ->
                _state.update { it.copy(connection = connection) }
                if (connection == ConnectionStatus.RECONNECTING && !manualDisconnect) scheduleReconnect()
            }
        }
        viewModelScope.launch {
            client.notifications.collect(::handleNotification)
        }
        if (savedProfile != null) viewModelScope.launch { connect(savedProfile.baseUrl, savedProfile.key, state.value.defaultCwd) }
    }

    fun updatePairing(address: String? = null, key: String? = null, cwd: String? = null) {
        _state.update {
            it.copy(
                address = address ?: it.address,
                pairingKey = key ?: it.pairingKey,
                defaultCwd = cwd ?: it.defaultCwd,
            )
        }
    }

    fun connect(address: String = state.value.address, key: String = state.value.pairingKey, cwd: String = state.value.defaultCwd) {
        viewModelScope.launch { connectInternal(address, key, cwd, reconnecting = false) }
    }

    private suspend fun connectInternal(address: String, key: String, cwd: String, reconnecting: Boolean) {
        if (!reconnecting) reconnectJob?.cancel()
        manualDisconnect = false
        _state.update { it.copy(connection = if (reconnecting) ConnectionStatus.RECONNECTING else ConnectionStatus.CONNECTING, error = null) }
        runCatching {
            val parsed = ServerProfile.parse(address, key)
            profile = parsed
            profileStore.save(parsed)
            profileStore.saveDefaultCwd(cwd)
            _state.update { it.copy(address = parsed.baseUrl, pairingKey = parsed.key, defaultCwd = cwd) }
            val initialize = client.connect(parsed)
            applyModelState(initialize)
            refreshSessionsInternal()
        }.onSuccess {
            reconnectJob = null
            _state.update { it.copy(connection = ConnectionStatus.CONNECTED, status = "已连接", error = null) }
        }.onFailure { error ->
            _state.update { it.copy(connection = if (reconnecting) ConnectionStatus.RECONNECTING else ConnectionStatus.FAILED, error = error.message ?: error.toString()) }
            if (reconnecting) throw error
        }
    }

    fun disconnect() {
        manualDisconnect = true
        reconnectJob?.cancel()
        reconnectJob = null
        resetTurnTracking()
        client.disconnect()
        _state.update { it.copy(connection = ConnectionStatus.DISCONNECTED, selectedSession = null) }
    }

    fun refreshSessions() {
        viewModelScope.launch { runCatching { refreshSessionsInternal() }.onFailure { showError(it) } }
    }

    private suspend fun refreshSessionsInternal() {
        val raw = client.rpc("_x.ai/sessions/list", buildJsonObject { }, 30_000)
        val sessions = unwrapSessions(raw).mapNotNull(SessionSummary::from)
            .sortedWith(compareByDescending<SessionSummary> { it.resident }.thenByDescending { it.updatedAt })
        _state.update { it.copy(sessions = sessions) }
    }

    fun openSession(session: SessionSummary) {
        viewModelScope.launch {
            resetTurnTracking()
            _state.update { it.copy(selectedSession = session, blocks = emptyList(), busy = false, status = "同步历史") }
            runCatching {
                val history = client.rest(
                    path = "/api/session/history",
                    query = mapOf(
                        "sessionId" to session.id,
                        "cwd" to session.cwd,
                        "limit" to "400",
                        "chat_only" to "1",
                    ),
                )
                history.objects("events").forEach(::ingestHistory)
            }.onFailure { _state.update { current -> current.copy(status = "历史暂不可用：${it.message}") } }

            runCatching {
                val loaded = client.rpc("session/load", buildJsonObject {
                    put("sessionId", session.id)
                    put("cwd", session.cwd)
                    put("mcpServers", JsonArray(emptyList()))
                }, 90_000)
                (loaded as? JsonObject)?.let(::applyModelState)
            }.onSuccess {
                _state.update { it.copy(status = "在线") }
            }.onFailure {
                _state.update { current -> current.copy(status = "挂载失败：${it.message}") }
            }
        }
    }

    fun closeSession() {
        resetTurnTracking()
        _state.update { it.copy(selectedSession = null, blocks = emptyList(), busy = false) }
    }

    fun createSession(cwd: String) {
        viewModelScope.launch {
            runCatching {
                val raw = client.rpc("session/new", buildJsonObject {
                    put("cwd", cwd)
                    put("mcpServers", JsonArray(emptyList()))
                }, 60_000).jsonObject
                val id = raw.string("sessionId", "session_id") ?: raw.obj("session")?.string("sessionId")
                    ?: error("session/new 未返回 sessionId")
                val session = SessionSummary(id, "新会话", cwd, true, "live", System.currentTimeMillis())
                refreshSessionsInternal()
                resetTurnTracking()
                _state.update { it.copy(selectedSession = session, blocks = emptyList()) }
            }.onFailure(::showError)
        }
    }

    fun send(text: String) {
        val session = state.value.selectedSession ?: return
        val message = text.trim()
        if (message.isEmpty()) return
        val turnId = UUID.randomUUID().toString()
        activeTurnId = turnId
        pendingUserEcho = message
        pendingUserEchoOffset = 0
        append(ChatBlock(UUID.randomUUID().toString(), ChatBlockKind.USER, text = message))
        _state.update { it.copy(busy = true, status = "等待 Grok") }
        viewModelScope.launch {
            runCatching {
                client.rpc("session/prompt", buildJsonObject {
                    put("sessionId", session.id)
                    put("prompt", buildJsonArray {
                        add(buildJsonObject { put("type", "text"); put("text", message) })
                    })
                })
            }.onSuccess {
                if (activeTurnId == turnId) {
                    activeTurnId = null
                    _state.update { it.copy(busy = false, status = "完成") }
                }
            }.onFailure { error ->
                if (activeTurnId != turnId) return@onFailure
                activeTurnId = null
                pendingUserEcho = null
                pendingUserEchoOffset = 0
                if (!error.message.orEmpty().contains("cancel", ignoreCase = true) && state.value.selectedSession?.id == session.id) {
                    append(ChatBlock(UUID.randomUUID().toString(), ChatBlockKind.SYSTEM, text = error.message ?: error.toString()))
                    _state.update { it.copy(busy = false, status = "发送失败") }
                } else {
                    _state.update { it.copy(busy = false, status = "已停止") }
                }
            }
        }
    }

    fun cancel() {
        val session = state.value.selectedSession ?: return
        resetTurnTracking()
        client.notify("session/cancel", buildJsonObject { put("sessionId", session.id) })
        _state.update { it.copy(busy = false, status = "已停止") }
    }

    fun setEffort(effort: String) {
        val session = state.value.selectedSession ?: return
        viewModelScope.launch {
            runCatching {
                client.rest("/api/effort", "POST", body = buildJsonObject {
                    put("sessionId", session.id)
                    put("modelId", state.value.model.currentModelId)
                    put("effort", effort)
                })
            }.onSuccess {
                _state.update { it.copy(model = it.model.copy(effort = effort)) }
            }.onFailure(::showError)
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

    private fun scheduleReconnect() {
        if (reconnectJob?.isActive == true) return
        val saved = profile ?: return
        reconnectJob = viewModelScope.launch {
            var waitMs = 1_000L
            while (isActive && !manualDisconnect) {
                delay(waitMs)
                val connected = runCatching {
                    connectInternal(saved.baseUrl, saved.key, state.value.defaultCwd, reconnecting = true)
                }.isSuccess && state.value.connection == ConnectionStatus.CONNECTED
                if (connected) return@launch
                waitMs = (waitMs * 2).coerceAtMost(15_000L)
            }
        }
    }

    private fun handleNotification(message: JsonObject) {
        val method = message.string("method").orEmpty()
        when {
            method == "session/update" || method == "_x.ai/session/update" || method == "x.ai/session/update" -> {
                val params = message.obj("params") ?: JsonObject(emptyMap())
                val sid = params.string("sessionId")
                if (sid == null || sid == state.value.selectedSession?.id) applyUpdate(params.obj("update") ?: params)
            }
            method == "_x.ai/sessions/changed" -> refreshSessions()
            method.contains("permission") || method.contains("ask_user") -> {
                val rpcId = (message["id"] as? JsonPrimitive)?.longOrNull ?: return
                val params = message.obj("params") ?: JsonObject(emptyMap())
                val options = params.objects("options").map {
                    PermissionOption(it.string("optionId", "id") ?: "allow", it.string("name", "label") ?: "允许")
                }.ifEmpty { listOf(PermissionOption("allow", "允许")) }
                append(ChatBlock(
                    id = "permission-$rpcId",
                    kind = ChatBlockKind.PERMISSION,
                    text = params.string("question", "message") ?: "Grok 需要你的确认",
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
                _state.update { it.copy(busy = true, status = "正在回复") }
            }
            "agent_thought_chunk" -> {
                appendChunk(ChatBlockKind.THINKING, text)
                _state.update { it.copy(busy = true, status = "正在思考") }
            }
            "tool_call", "tool_call_update" -> {
                upsertTool(update)
                _state.update { it.copy(busy = true, status = update.string("title", "toolName", "kind") ?: "正在使用工具") }
            }
            "plan" -> appendChunk(ChatBlockKind.PLAN, text.ifEmpty { "Plan" })
            "session_recap" -> appendChunk(ChatBlockKind.SYSTEM, text)
            "turn_completed", "task_completed" -> {
                activeTurnId = null
                _state.update { it.copy(busy = false, status = "完成") }
            }
        }
    }

    private fun consumePendingUserEcho(chunk: String): Boolean {
        val expected = pendingUserEcho ?: return false
        if (chunk.isEmpty()) return true
        val remaining = expected.substring(pendingUserEchoOffset.coerceIn(0, expected.length))
        val nextOffset = when {
            chunk == expected -> expected.length
            remaining.startsWith(chunk) -> pendingUserEchoOffset + chunk.length
            expected.startsWith(chunk) -> maxOf(pendingUserEchoOffset, chunk.length)
            else -> {
                pendingUserEcho = null
                pendingUserEchoOffset = 0
                return false
            }
        }
        pendingUserEchoOffset = nextOffset.coerceAtMost(expected.length)
        if (pendingUserEchoOffset == expected.length) {
            pendingUserEcho = null
            pendingUserEchoOffset = 0
        }
        return true
    }

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
        pendingUserEcho = null
        pendingUserEchoOffset = 0
    }

    private fun ingestHistory(event: JsonObject) {
        val update = event.obj("params")?.let { it.obj("update") ?: it } ?: event.obj("update") ?: event
        applyUpdate(update)
        _state.update { it.copy(busy = false) }
    }

    private fun unwrapSessions(raw: JsonElement): List<JsonObject> = when (raw) {
        is JsonArray -> raw.mapNotNull { it as? JsonObject }
        is JsonObject -> when {
            raw["sessions"] is JsonArray -> (raw["sessions"] as JsonArray).mapNotNull { it as? JsonObject }
            raw["result"] != null -> unwrapSessions(raw["result"]!!)
            else -> emptyList()
        }
        else -> emptyList()
    }

    private fun applyModelState(objectValue: JsonObject) {
        val source = objectValue.obj("models") ?: objectValue.obj("_meta")?.obj("modelState") ?: objectValue.obj("modelState") ?: return
        var model = state.value.model
        source.string("currentModelId")?.let { model = model.copy(currentModelId = it) }
        val current = source.objects("availableModels").firstOrNull { it.string("modelId") == model.currentModelId }
        val meta = current?.obj("_meta")
        meta?.string("reasoningEffort")?.let { model = model.copy(effort = it) }
        val levels = meta?.objects("reasoningEfforts")?.mapNotNull { it.string("value", "id") }.orEmpty()
        if (levels.isNotEmpty()) model = model.copy(effortLevels = levels)
        _state.update { it.copy(model = model) }
    }

    private fun append(block: ChatBlock) { _state.update { it.copy(blocks = it.blocks + block) } }
    private fun showError(error: Throwable) { _state.update { it.copy(error = error.message ?: error.toString(), status = error.message.orEmpty()) } }

    override fun onCleared() {
        client.close()
        super.onCleared()
    }
}
