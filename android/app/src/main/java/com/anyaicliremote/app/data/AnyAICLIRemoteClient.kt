package com.anyaicliremote.app.data

import android.util.Log
import com.anyaicliremote.app.ProductIdentifiers
import com.anyaicliremote.app.model.ConnectionStatus
import com.anyaicliremote.app.model.ServerProfile
import com.agentclientprotocol.rpc.JsonRpcNotification
import com.agentclientprotocol.rpc.JsonRpcRequest
import com.agentclientprotocol.rpc.JsonRpcResponse
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
import kotlinx.serialization.json.contentOrNull
import kotlinx.serialization.json.jsonObject
import okhttp3.HttpUrl.Companion.toHttpUrl
import okhttp3.HttpUrl
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

data class ClientNotification(
    val connectionGeneration: Long,
    val message: JsonObject,
)

class AnyAICLIRemoteClient(private val configuration: ClientRuntimeConfiguration = ClientRuntimeConfiguration.Default) {
    private companion object {
        const val LOG_TAG = "AnyAICLIRemoteClient"
    }
    private val scope = CoroutineScope(SupervisorJob() + Dispatchers.IO)
    private val json = Json { ignoreUnknownKeys = true; isLenient = true }
    private val http = OkHttpClient.Builder()
        .connectTimeout(configuration.connectTimeout.inWholeMilliseconds, TimeUnit.MILLISECONDS)
        .readTimeout(configuration.readTimeout.inWholeMilliseconds, TimeUnit.MILLISECONDS)
        .pingInterval(configuration.pingInterval.inWholeMilliseconds, TimeUnit.MILLISECONDS)
        .build()

    private val _connection = MutableStateFlow(ConnectionStatus.DISCONNECTED)
    val connection = _connection.asStateFlow()
    private val notificationQueue = Channel<ClientNotification>(Channel.UNLIMITED)
    val notifications = notificationQueue.receiveAsFlow()

    private var profile: ServerProfile? = null
    private var webSocket: WebSocket? = null
    private var openSignal: CompletableDeferred<Unit>? = null
    private val ids = AtomicLong(0)
    private val connectionGenerations = AtomicLong(0)
    private val pending = ConcurrentHashMap<Long, CompletableDeferred<JsonElement>>()
    @Volatile private var activeConnectionGeneration = 0L
    private var closedByUser = false

    suspend fun connect(profile: ServerProfile): JsonObject {
        disconnect(notify = false)
        this.profile = profile
        closedByUser = false
        _connection.value = ConnectionStatus.CONNECTING

        val request = Request.Builder()
            .url(buildWebSocketURL(profile.baseUrl))
            .authorizeWithProductKey(profile.key)
            .build()
        val signal = CompletableDeferred<Unit>()
        val connectionGeneration = connectionGenerations.incrementAndGet()
        activeConnectionGeneration = connectionGeneration
        openSignal = signal
        webSocket = http.newWebSocket(request, createListener(connectionGeneration, signal))
        withTimeout(configuration.socketOpenTimeout) { signal.await() }

        val result = rpc(
            method = ACPWire.initializeMethod,
            params = ACPWire.initializeParameters(
                clientName = ProductIdentifiers.clientName,
                clientVersion = ProductIdentifiers.clientVersion,
            ),
            timeoutMs = configuration.initializeTimeout.inWholeMilliseconds,
        )
        _connection.value = ConnectionStatus.CONNECTED
        return result as? JsonObject ?: JsonObject(emptyMap())
    }

    fun disconnect(notify: Boolean = true) {
        closedByUser = true
        activeConnectionGeneration = connectionGenerations.incrementAndGet()
        webSocket?.cancel()
        webSocket = null
        profile = null
        openSignal?.cancel()
        openSignal = null
        failPending(IOException("connection closed"))
        if (notify) _connection.value = ConnectionStatus.DISCONNECTED
    }

    fun isCurrentNotification(notification: ClientNotification): Boolean =
        notification.connectionGeneration == activeConnectionGeneration && webSocket != null

    suspend fun rpc(method: String, params: JsonObject, timeoutMs: Long = configuration.rpcTimeout.inWholeMilliseconds): JsonElement {
        val socket = webSocket ?: error("连接已断开")
        val id = ids.incrementAndGet()
        val deferred = CompletableDeferred<JsonElement>()
        pending[id] = deferred
        if (!socket.send(ACPWire.encodeRequest(id, method, params))) {
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
        webSocket?.send(ACPWire.encodeNotification(method, params))
    }

    fun reply(id: Long, result: JsonObject) {
        webSocket?.send(ACPWire.encodeResponse(id, result))
    }

    suspend fun rest(
        path: String,
        method: String = "GET",
        query: Map<String, String> = emptyMap(),
        body: JsonObject? = null,
    ): JsonObject = executeRest(path = path, method = method, query = query, body = body)

    suspend fun rest(
        pathSegments: List<String>,
        method: String = "GET",
        query: Map<String, String> = emptyMap(),
        body: JsonObject? = null,
    ): JsonObject = executeRest(pathSegments = pathSegments, method = method, query = query, body = body)

    private suspend fun executeRest(
        path: String? = null,
        pathSegments: List<String> = emptyList(),
        method: String,
        query: Map<String, String>,
        body: JsonObject?,
    ): JsonObject = withContext(Dispatchers.IO) {
        val current = profile ?: error("连接已断开")
        val mediaType = "application/json; charset=utf-8".toMediaType()
        val requestBody = if (method == "GET" || method == "HEAD") null
        else (body ?: JsonObject(emptyMap())).toString().toRequestBody(mediaType)
        val request = Request.Builder()
            .url(buildRestURL(current.baseUrl, path, pathSegments, query))
            .authorizeWithProductKey(current.key)
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

    private fun createListener(
        connectionGeneration: Long,
        connectionSignal: CompletableDeferred<Unit>,
    ) = object : WebSocketListener() {
        override fun onOpen(webSocket: WebSocket, response: Response) {
            if (this@AnyAICLIRemoteClient.webSocket !== webSocket) return
            Log.i(LOG_TAG, "WebSocket connected (${response.code})")
            connectionSignal.complete(Unit)
        }

        override fun onMessage(webSocket: WebSocket, text: String) {
            if (this@AnyAICLIRemoteClient.webSocket !== webSocket) return
            when (val message = runCatching { ACPWire.decode(text) }.getOrNull() ?: return) {
                is JsonRpcResponse -> {
                    val identifier = ACPWire.responseIdentifier(message) ?: return
                    val deferred = pending.remove(identifier) ?: return
                    val errorObject = message.error
                    if (errorObject != null) {
                        deferred.completeExceptionally(IOException(errorObject.message))
                    } else {
                        deferred.complete(message.result ?: JsonNull)
                    }
                }
                is JsonRpcRequest -> when (ACPWire.classifyIncomingRequest(message.method.name)) {
                    ACPWire.IncomingRequestDisposition.UI_HANDLED -> notificationQueue.trySend(
                        ClientNotification(connectionGeneration, ACPWire.requestAsJson(message))
                    )
                    ACPWire.IncomingRequestDisposition.METHOD_NOT_FOUND -> {
                        webSocket.send(ACPWire.encodeMethodNotFoundResponse(message))
                    }
                }
                is JsonRpcNotification -> notificationQueue.trySend(
                    ClientNotification(connectionGeneration, ACPWire.notificationAsJson(message))
                )
            }
        }

        override fun onClosing(webSocket: WebSocket, code: Int, reason: String) {
            webSocket.close(code, reason)
        }

        override fun onClosed(webSocket: WebSocket, code: Int, reason: String) {
            if (this@AnyAICLIRemoteClient.webSocket !== webSocket) return
            Log.w(LOG_TAG, "WebSocket closed: code=$code reason=$reason")
            this@AnyAICLIRemoteClient.webSocket = null
            profile = null
            activeConnectionGeneration = connectionGenerations.incrementAndGet()
            failPending(IOException(reason.ifEmpty { "connection closed" }))
            if (!closedByUser) _connection.value = ConnectionStatus.RECONNECTING
        }

        override fun onFailure(webSocket: WebSocket, t: Throwable, response: Response?) {
            if (this@AnyAICLIRemoteClient.webSocket !== webSocket) return
            Log.e(LOG_TAG, "WebSocket failed: HTTP ${response?.code ?: "n/a"}", t)
            this@AnyAICLIRemoteClient.webSocket = null
            profile = null
            activeConnectionGeneration = connectionGenerations.incrementAndGet()
            connectionSignal.completeExceptionally(t)
            failPending(t)
            if (!closedByUser) _connection.value = ConnectionStatus.RECONNECTING
        }
    }

    private fun failPending(error: Throwable) {
        pending.values.forEach { it.completeExceptionally(error) }
        pending.clear()
    }

}

internal fun buildWebSocketURL(baseURL: String): String {
    val base = baseURL.toHttpUrl()
    val webSocketScheme = when (base.scheme) {
        "http" -> "ws"
        "https" -> "wss"
        else -> error("Unsupported WebSocket URL scheme: ${base.scheme}")
    }
    val sanitizedHTTPURL = base.newBuilder()
        .encodedPath("/ws")
        .query(null)
        .fragment(null)
        .build()
    return sanitizedHTTPURL.toString().replaceFirst(
        oldValue = "${base.scheme}://",
        newValue = "$webSocketScheme://",
    )
}

internal fun Request.Builder.authorizeWithProductKey(pairingKey: String): Request.Builder = apply {
    header(ProductIdentifiers.authorizationHeader, pairingKey)
}

internal fun buildRestURL(
    baseURL: String,
    path: String? = null,
    pathSegments: List<String> = emptyList(),
    query: Map<String, String> = emptyMap(),
): HttpUrl {
    require((path != null) xor pathSegments.isNotEmpty()) { "exactly one REST path form is required" }
    val builder = baseURL.toHttpUrl().newBuilder().query(null)
    if (path != null) {
        builder.encodedPath(path)
    } else {
        builder.encodedPath("/")
        pathSegments.forEach(builder::addPathSegment)
    }
    query.forEach { (key, value) -> builder.addQueryParameter(key, value) }
    return builder.build()
}
