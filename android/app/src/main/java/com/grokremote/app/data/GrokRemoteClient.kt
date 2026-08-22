package com.grokremote.app.data

import android.util.Log
import com.grokremote.app.model.ConnectionStatus
import com.grokremote.app.model.ServerProfile
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.cancel
import kotlinx.coroutines.channels.Channel
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.receiveAsFlow
import kotlinx.coroutines.withContext
import kotlinx.coroutines.withTimeout
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonNull
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.contentOrNull
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.longOrNull
import kotlinx.serialization.json.put
import okhttp3.HttpUrl.Companion.toHttpUrl
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.RequestBody.Companion.toRequestBody
import okhttp3.Response
import okhttp3.WebSocket
import okhttp3.WebSocketListener
import java.io.IOException
import java.util.concurrent.ConcurrentHashMap
import java.util.concurrent.TimeUnit
import java.util.concurrent.atomic.AtomicLong

class GrokRemoteClient {
	private companion object {
		const val LOG_TAG = "GrokRemoteClient"
	}
    private val scope = CoroutineScope(SupervisorJob() + Dispatchers.IO)
    private val json = Json { ignoreUnknownKeys = true; isLenient = true }
    private val http = OkHttpClient.Builder()
        .connectTimeout(20, TimeUnit.SECONDS)
        .readTimeout(120, TimeUnit.SECONDS)
        .pingInterval(12, TimeUnit.SECONDS)
        .build()

    private val _connection = MutableStateFlow(ConnectionStatus.DISCONNECTED)
    val connection = _connection.asStateFlow()
    private val notificationQueue = Channel<JsonObject>(Channel.UNLIMITED)
    val notifications = notificationQueue.receiveAsFlow()

    private var profile: ServerProfile? = null
    private var webSocket: WebSocket? = null
    private var openSignal: CompletableDeferred<Unit>? = null
    private val ids = AtomicLong(0)
    private val pending = ConcurrentHashMap<Long, CompletableDeferred<JsonElement>>()
    private var closedByUser = false

    suspend fun connect(profile: ServerProfile): JsonObject {
        disconnect(notify = false)
        this.profile = profile
        closedByUser = false
        _connection.value = ConnectionStatus.CONNECTING

        val base = profile.baseUrl.toHttpUrl()
        val httpSocketUrl = base.newBuilder()
            .encodedPath("/ws")
            .query(null)
            .addQueryParameter("key", profile.key)
            .build()
            .toString()
        val wsUrl = if (httpSocketUrl.startsWith("https://")) {
            "wss://" + httpSocketUrl.removePrefix("https://")
        } else {
            "ws://" + httpSocketUrl.removePrefix("http://")
        }
        val request = Request.Builder()
            .url(wsUrl)
            .header("X-Grok-Remote-Key", profile.key)
            .build()
        val signal = CompletableDeferred<Unit>()
        openSignal = signal
        webSocket = http.newWebSocket(request, listener)
        withTimeout(20_000) { signal.await() }

        val result = rpc(
            method = "initialize",
            params = buildJsonObject {
                put("protocolVersion", 1)
                put("clientInfo", buildJsonObject {
                    put("name", "grok-remote-app-android")
                    put("version", "0.1.0")
                })
                put("clientCapabilities", buildJsonObject {
                    put("fs", buildJsonObject {
                        put("readTextFile", true)
                        put("writeTextFile", true)
                    })
                    put("terminal", true)
                })
            },
            timeoutMs = 20_000,
        )
        _connection.value = ConnectionStatus.CONNECTED
        return result as? JsonObject ?: JsonObject(emptyMap())
    }

    fun disconnect(notify: Boolean = true) {
        closedByUser = true
        webSocket?.close(1000, "client disconnect")
        webSocket = null
        openSignal?.cancel()
        openSignal = null
        failPending(IOException("connection closed"))
        if (notify) _connection.value = ConnectionStatus.DISCONNECTED
    }

    suspend fun rpc(method: String, params: JsonObject, timeoutMs: Long = 120_000): JsonElement {
        val socket = webSocket ?: error("连接已断开")
        val id = ids.incrementAndGet()
        val deferred = CompletableDeferred<JsonElement>()
        pending[id] = deferred
        val payload = buildJsonObject {
            put("jsonrpc", "2.0")
            put("id", id)
            put("method", method)
            put("params", params)
        }
        if (!socket.send(payload.toString())) {
            pending.remove(id)
            error("WebSocket 发送失败")
        }
        return try {
            withTimeout(timeoutMs) { deferred.await() }
        } finally {
            pending.remove(id)
        }
    }

    fun notify(method: String, params: JsonObject) {
        val payload = buildJsonObject {
            put("jsonrpc", "2.0")
            put("method", method)
            put("params", params)
        }
        webSocket?.send(payload.toString())
    }

    fun reply(id: Long, result: JsonObject) {
        val payload = buildJsonObject {
            put("jsonrpc", "2.0")
            put("id", id)
            put("result", result)
        }
        webSocket?.send(payload.toString())
    }

    suspend fun rest(
        path: String,
        method: String = "GET",
        query: Map<String, String> = emptyMap(),
        body: JsonObject? = null,
    ): JsonObject = withContext(Dispatchers.IO) {
        val current = profile ?: error("连接已断开")
        val builder = current.baseUrl.toHttpUrl().newBuilder().encodedPath(path).query(null)
        query.forEach { (key, value) -> builder.addQueryParameter(key, value) }
        val mediaType = "application/json; charset=utf-8".toMediaType()
        val requestBody = if (method == "GET" || method == "HEAD") null
        else (body ?: JsonObject(emptyMap())).toString().toRequestBody(mediaType)
        val request = Request.Builder()
            .url(builder.build())
            .header("X-Grok-Remote-Key", current.key)
            .header("Accept", "application/json")
            .method(method, requestBody)
            .build()
        http.newCall(request).execute().use { response ->
            val text = response.body?.string().orEmpty()
            val parsed = runCatching { json.parseToJsonElement(text).jsonObject }.getOrElse { JsonObject(emptyMap()) }
            if (!response.isSuccessful) {
                val message = (parsed["error"] as? JsonPrimitive)?.contentOrNull
                    ?: (parsed["message"] as? JsonPrimitive)?.contentOrNull
                    ?: "HTTP ${response.code}"
                error(message)
            }
            parsed
        }
    }

    fun close() {
        disconnect()
        notificationQueue.close()
        scope.cancel()
        http.dispatcher.executorService.shutdown()
    }

    private val listener = object : WebSocketListener() {
        override fun onOpen(webSocket: WebSocket, response: Response) {
			Log.i(LOG_TAG, "WebSocket connected (${response.code})")
            openSignal?.complete(Unit)
        }

        override fun onMessage(webSocket: WebSocket, text: String) {
            val objectValue = runCatching { json.parseToJsonElement(text) as? JsonObject }.getOrNull() ?: return
            val id = (objectValue["id"] as? JsonPrimitive)?.longOrNull
            if (id != null && objectValue["method"] == null) {
                val deferred = pending.remove(id) ?: return
                val errorObject = objectValue["error"] as? JsonObject
                if (errorObject != null) {
                    val message = (errorObject["message"] as? JsonPrimitive)?.contentOrNull ?: "RPC error"
                    deferred.completeExceptionally(IOException(message))
                } else {
                    deferred.complete(objectValue["result"] ?: JsonNull)
                }
                return
            }
            notificationQueue.trySend(objectValue)
        }

        override fun onClosing(webSocket: WebSocket, code: Int, reason: String) {
            webSocket.close(code, reason)
        }

        override fun onClosed(webSocket: WebSocket, code: Int, reason: String) {
			Log.w(LOG_TAG, "WebSocket closed: code=$code reason=$reason")
            this@GrokRemoteClient.webSocket = null
            failPending(IOException(reason.ifEmpty { "connection closed" }))
            if (!closedByUser) _connection.value = ConnectionStatus.RECONNECTING
        }

        override fun onFailure(webSocket: WebSocket, t: Throwable, response: Response?) {
			Log.e(LOG_TAG, "WebSocket failed: HTTP ${response?.code ?: "n/a"}", t)
            this@GrokRemoteClient.webSocket = null
            openSignal?.completeExceptionally(t)
            failPending(t)
            if (!closedByUser) _connection.value = ConnectionStatus.RECONNECTING
        }
    }

    private fun failPending(error: Throwable) {
        pending.values.forEach { it.completeExceptionally(error) }
        pending.clear()
    }
}
