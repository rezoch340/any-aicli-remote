package com.anyaicliremote.app.chat

import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import okhttp3.WebSocket
import okhttp3.WebSocketListener
import okhttp3.mockwebserver.Dispatcher
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import okhttp3.mockwebserver.RecordedRequest
import okio.ByteString
import java.io.Closeable
import java.util.concurrent.CopyOnWriteArrayList
import java.util.concurrent.TimeUnit

/** A deterministic in-process daemon: MockWebServer is the only transport implementation. */
internal class TestDaemonFixture : Closeable {
    private val json = Json { ignoreUnknownKeys = true; isLenient = true }
    private val generationMonitor = Object()
    private val requests = CopyOnWriteArrayList<RecordedRequest>()
    private val messages = CopyOnWriteArrayList<JsonObject>()
    private val responses = CopyOnWriteArrayList<JsonObject>()
    private val server = MockWebServer()
    private var webSocket: WebSocket? = null
    private var webSocketGeneration = 0

    @Volatile var sessionsBody: String = "{\"sessions\":[]}"
    @Volatile var messagesBody: String = "{\"messages\":[]}"
    @Volatile var filesBody: String = "{\"path\":\".\",\"parent\":null,\"dirs\":[],\"files\":[]}"
    @Volatile var onRequest: ((JsonObject) -> Unit)? = null
    @Volatile var afterSessionLoadResponse: (() -> Unit)? = null

    val baseUrl: String
        get() = server.url("/").toString().trimEnd('/')

    init {
        server.dispatcher = object : Dispatcher() {
            override fun dispatch(request: RecordedRequest): MockResponse {
                requests += request
                return when {
                    request.path?.substringBefore('?') == "/ws" ->
                        MockResponse().withWebSocketUpgrade(socketListener)
                    request.path?.substringBefore('?') == "/api/sessions" -> jsonResponse(sessionsBody)
                    request.path?.contains("/messages") == true -> jsonResponse(messagesBody)
                    request.path?.substringBefore('?') == "/api/fs/list" -> jsonResponse(filesBody)
                    request.path?.substringBefore('?') == "/health" -> jsonResponse("{\"ok\":true}")
                    else -> MockResponse().setResponseCode(404).setBody("{}")
                }
            }
        }
        server.start()
    }

    private val socketListener = object : WebSocketListener() {
        override fun onOpen(socket: WebSocket, response: okhttp3.Response) {
            synchronized(generationMonitor) {
                webSocket = socket
                webSocketGeneration += 1
                generationMonitor.notifyAll()
            }
        }

        override fun onMessage(socket: WebSocket, text: String) {
            val message = runCatching { json.parseToJsonElement(text).jsonObject }.getOrNull() ?: return
            messages += message
            val method = message["method"]?.jsonPrimitive?.content.orEmpty()
            if (message["id"] != null && method.isEmpty()) {
                responses += message
            } else if (message["id"] != null && method.isNotEmpty()) {
                onRequest?.invoke(message)
                when (method) {
                    "initialize" -> respond(message, "{}")
                    "session/load" -> {
                        respond(message, "{}")
                        afterSessionLoadResponse?.let { callback ->
                            afterSessionLoadResponse = null
                            callback()
                        }
                    }
                    "session/new" -> {
                        val sessionId = "new-session"
                        sessionsBody = "{\"sessions\":[{\"providerId\":\"grok\",\"sessionId\":\"$sessionId\",\"title\":\"New workspace\",\"projectDir\":\"/workspace/new-project\",\"resident\":true,\"activity\":\"idle\",\"createdAt\":1,\"lastActiveAt\":2}]}"
                        respond(message, "{\"sessionId\":\"$sessionId\"}")
                    }
                    "session/prompt" -> Unit
                    else -> Unit
                }
            }
        }

        override fun onMessage(socket: WebSocket, bytes: ByteString) = Unit

        override fun onClosed(socket: WebSocket, code: Int, reason: String) {
            synchronized(generationMonitor) {
                if (webSocket === socket) webSocket = null
                generationMonitor.notifyAll()
            }
        }

        override fun onFailure(socket: WebSocket, throwable: Throwable, response: okhttp3.Response?) {
            synchronized(generationMonitor) {
                if (webSocket === socket) webSocket = null
                generationMonitor.notifyAll()
            }
        }
    }

    fun currentSocketGeneration(): Int = synchronized(generationMonitor) { webSocketGeneration }

    fun awaitSocket(timeoutSeconds: Long = 10): Boolean =
        awaitSocketGeneration(0, timeoutSeconds) > 0

    fun awaitSocketGeneration(afterGeneration: Int, timeoutSeconds: Long = 10): Int {
        val deadline = System.nanoTime() + TimeUnit.SECONDS.toNanos(timeoutSeconds)
        synchronized(generationMonitor) {
            while (webSocketGeneration <= afterGeneration || webSocket == null) {
                val remainingNanos = deadline - System.nanoTime()
                if (remainingNanos <= 0) return 0
                val waitMillis = (remainingNanos / 1_000_000).coerceAtLeast(1)
                generationMonitor.wait(waitMillis)
            }
            return webSocketGeneration
        }
    }

    fun awaitRequest(method: String, timeoutSeconds: Long = 10): JsonObject {
        val deadline = System.nanoTime() + TimeUnit.SECONDS.toNanos(timeoutSeconds)
        while (System.nanoTime() < deadline) {
            messages.firstOrNull { it["method"]?.jsonPrimitive?.content == method }?.let { return it }
            Thread.sleep(20)
        }
        error("Timed out waiting for WebSocket method $method; received ${messages.map { it["method"]?.jsonPrimitive?.content }}")
    }

    fun awaitResponse(identifier: Long, timeoutSeconds: Long = 10): JsonObject {
        val deadline = System.nanoTime() + TimeUnit.SECONDS.toNanos(timeoutSeconds)
        while (System.nanoTime() < deadline) {
            responses.firstOrNull { it["id"]?.jsonPrimitive?.content?.toLongOrNull() == identifier }?.let { return it }
            Thread.sleep(20)
        }
        error("Timed out waiting for JSON-RPC response $identifier")
    }

    fun requestsFor(pathFragment: String): List<RecordedRequest> =
        requests.filter { it.path?.contains(pathFragment) == true }

    fun sendNotification(method: String, params: String) {
        check(awaitSocket())
        webSocket!!.send("{\"jsonrpc\":\"2.0\",\"method\":\"$method\",\"params\":$params}")
    }

    fun sendRequest(identifier: Long, method: String, params: String) {
        check(awaitSocket())
        webSocket!!.send("{\"jsonrpc\":\"2.0\",\"id\":$identifier,\"method\":\"$method\",\"params\":$params}")
    }

    fun closeSocket() {
        webSocket?.close(1001, "fixture disconnect")
    }

    fun respondTo(request: JsonObject, result: String = "{}") {
        val identifier = request["id"] ?: return
        webSocket?.send("{\"jsonrpc\":\"2.0\",\"id\":$identifier,\"result\":$result}")
    }

    private fun respond(request: JsonObject, result: String) = respondTo(request, result)

    private fun jsonResponse(body: String): MockResponse = MockResponse()
        .setHeader("Content-Type", "application/json")
        .setBody(body)

    override fun close() {
        webSocket?.let { runCatching { it.cancel() } }
        webSocket = null
        server.shutdown()
    }
}
