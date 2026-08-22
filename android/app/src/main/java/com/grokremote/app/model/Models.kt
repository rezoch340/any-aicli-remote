package com.grokremote.app.model

import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonNull
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.booleanOrNull
import kotlinx.serialization.json.contentOrNull
import kotlinx.serialization.json.doubleOrNull
import okhttp3.HttpUrl.Companion.toHttpUrlOrNull

enum class ConnectionStatus { DISCONNECTED, CONNECTING, CONNECTED, RECONNECTING, FAILED }

data class ServerProfile(val baseUrl: String, val key: String) {
    companion object {
        fun parse(address: String, fallbackKey: String): ServerProfile {
            var raw = address.trim()
            if (!raw.contains("://")) raw = "http://$raw"
            val url = raw.toHttpUrlOrNull()
                ?: error("服务地址无效")
            require(url.scheme == "http" || url.scheme == "https") { "仅支持 HTTP/HTTPS 地址" }
            val pairedKey = url.queryParameter("key")
            val key = (pairedKey ?: fallbackKey).trim()
            require(key.isNotEmpty()) { "缺少配对 Key" }
            val base = url.newBuilder().encodedPath("/").query(null).fragment(null).build().toString().trimEnd('/')
            return ServerProfile(base, key)
        }
    }
}

data class SessionSummary(
    val id: String,
    val title: String,
    val cwd: String,
    val resident: Boolean,
    val activity: String,
    val updatedAt: Long,
) {
    companion object {
        fun from(json: JsonObject): SessionSummary? {
            val id = json.string("sessionId", "session_id", "id") ?: return null
            return SessionSummary(
                id = id,
                title = json.string("remote_title", "title", "generated_title")?.takeIf { it.isNotBlank() } ?: "未命名会话",
                cwd = json.string("cwd").orEmpty(),
                resident = json.bool("resident") ?: false,
                activity = json.string("activity", "status").orEmpty(),
                updatedAt = json.timestamp(),
            )
        }
    }
}

enum class ChatBlockKind { USER, ASSISTANT, THINKING, TOOL, PERMISSION, PLAN, SYSTEM }
enum class ToolRunState { PENDING, RUNNING, SUCCESS, FAILED, CANCELLED }

data class PermissionOption(val id: String, val label: String)

data class ChatBlock(
    val id: String,
    val kind: ChatBlockKind,
    val text: String = "",
    val title: String = "",
    val detail: String = "",
    val toolState: ToolRunState = ToolRunState.PENDING,
    val rpcId: Long? = null,
    val options: List<PermissionOption> = emptyList(),
)

data class ModelState(
    val currentModelId: String = "grok",
    val effort: String = "low",
    val effortLevels: List<String> = listOf("low", "medium", "high", "xhigh"),
)

fun JsonObject.string(vararg keys: String): String? {
    for (key in keys) {
        val value = (this[key] as? JsonPrimitive)?.contentOrNull
        if (value != null) return value
    }
    return null
}

fun JsonObject.bool(key: String): Boolean? = (this[key] as? JsonPrimitive)?.booleanOrNull
fun JsonObject.obj(key: String): JsonObject? = this[key] as? JsonObject
fun JsonObject.objects(key: String): List<JsonObject> = (this[key] as? JsonArray)?.mapNotNull { it as? JsonObject }.orEmpty()

fun JsonObject.timestamp(): Long {
    val keys = listOf("lastChangeUnixMs", "last_change_unix_ms", "updatedAt", "updated_at", "mtime", "createdAt")
    for (key in keys) {
        val value = this[key]
        if (value is JsonPrimitive) {
            value.doubleOrNull?.let { return if (it < 1_000_000_000_000) (it * 1000).toLong() else it.toLong() }
            value.contentOrNull?.toLongOrNull()?.let { return if (it < 1_000_000_000_000) it * 1000 else it }
        }
    }
    return 0
}

fun toolState(raw: String?): ToolRunState {
    val value = raw.orEmpty().lowercase()
    return when {
        value.contains("success") || value.contains("complete") || value == "done" -> ToolRunState.SUCCESS
        value.contains("fail") || value.contains("error") || value.contains("timeout") -> ToolRunState.FAILED
        value.contains("cancel") -> ToolRunState.CANCELLED
        value.contains("run") || value.contains("stream") || value.contains("progress") -> ToolRunState.RUNNING
        else -> ToolRunState.PENDING
    }
}

fun Any?.toJsonElement(): JsonElement = when (this) {
    null -> JsonNull
    is JsonElement -> this
    is String -> JsonPrimitive(this)
    is Number -> JsonPrimitive(this)
    is Boolean -> JsonPrimitive(this)
    is Map<*, *> -> JsonObject(entries.associate { it.key.toString() to it.value.toJsonElement() })
    is Iterable<*> -> JsonArray(map { it.toJsonElement() })
    else -> JsonPrimitive(toString())
}
