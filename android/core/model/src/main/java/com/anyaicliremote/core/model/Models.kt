package com.anyaicliremote.core.model

import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonNull
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.booleanOrNull
import kotlinx.serialization.json.contentOrNull
import kotlinx.serialization.json.doubleOrNull
import okhttp3.HttpUrl.Companion.toHttpUrlOrNull
import java.net.URI

private const val HTTP_DEFAULT_PORT = 80
private const val HTTPS_DEFAULT_PORT = 443
private const val MILLISECOND_TIMESTAMP_THRESHOLD = 1_000_000_000_000L
private const val MILLISECONDS_PER_SECOND = 1_000L

enum class ConnectionStatus { DISCONNECTED, CONNECTING, CONNECTED, RECONNECTING, FAILED }
enum class DeviceHealthStatus { CHECKING, ONLINE, OFFLINE }

data class ServerProfile(val baseUrl: String, val key: String) {
    companion object {
        fun parse(address: String, fallbackKey: String): ServerProfile {
            var normalizedAddress = address.trim()
            if (!normalizedAddress.contains("://")) normalizedAddress = "http://$normalizedAddress"
            val url = normalizedAddress.toHttpUrlOrNull()
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

data class SavedDevice(
    val id: String,
    val name: String,
    val baseUrl: String,
    val pairingKey: String,
) {
    val serverProfile: ServerProfile
        get() = ServerProfile(baseUrl, pairingKey)

    companion object {
        fun normalized(
            id: String,
            name: String,
            address: String,
            pairingKey: String,
        ): SavedDevice {
            val profile = ServerProfile.parse(address, pairingKey)
            val displayName = name.trim().ifEmpty { defaultDeviceName(profile.baseUrl) }
            return SavedDevice(
                id = id,
                name = displayName,
                baseUrl = profile.baseUrl,
                pairingKey = profile.key,
            )
        }

        fun defaultDeviceName(baseUrl: String): String {
            val parsedUrl = baseUrl.toHttpUrlOrNull()
            return parsedUrl?.let { url ->
                val defaultPort = if (url.scheme == "https") HTTPS_DEFAULT_PORT else HTTP_DEFAULT_PORT
                if (url.port == defaultPort) url.host else "${url.host}:${url.port}"
            } ?: baseUrl.trim()
        }
    }
}

data class SessionIdentity(
    val providerId: String,
    val sessionId: String,
)

data class SessionSummary(
    val providerId: String,
    val id: String,
    val title: String,
    val projectDirectory: String,
    val resident: Boolean,
    val activity: String,
    val createdAt: Long,
    val lastActiveAt: Long,
) {
    val identity: SessionIdentity
        get() = SessionIdentity(providerId, id)

    val updatedAt: Long
        get() = lastActiveAt.takeIf { it > 0 } ?: createdAt

    companion object {
        fun from(json: JsonObject): SessionSummary? {
            val providerId = json.string("providerId") ?: return null
            val id = json.string("sessionId", "session_id", "id") ?: return null
            return SessionSummary(
                providerId = providerId,
                id = id,
                title = json.string("remote_title", "title", "generated_title")?.takeIf { it.isNotBlank() } ?: "未命名会话",
                projectDirectory = json.string("projectDir", "cwd").orEmpty(),
                resident = json.bool("resident") ?: false,
                activity = json.string("activity", "status").orEmpty(),
                createdAt = json.timestamp("createdAt"),
                lastActiveAt = json.timestamp("lastActiveAt"),
            )
        }
    }
}

data class SessionMessage(
    val role: String,
    val content: String,
    val timestamp: Long,
) {
    companion object {
        fun from(json: JsonObject): SessionMessage? {
            val role = json.string("role") ?: return null
            if (role !in setOf("system", "user", "assistant", "tool")) return null
            val content = json.string("content") ?: return null
            return SessionMessage(
                role = role,
                content = content,
                timestamp = json.timestamp("ts"),
            )
        }
    }
}

data class WorkspaceFile(
    val name: String,
    val path: String,
    val relativePath: String,
    val size: Long = 0,
    val text: Boolean = false,
    val directory: Boolean = false,
    val modifiedAt: Long = 0,
) {
    val uri: String
        get() = workspaceFileUri(path)

    companion object {
        fun from(json: JsonObject, directory: Boolean): WorkspaceFile? {
            val path = json.string("path") ?: return null
            val name = json.string("name") ?: path.substringAfterLast('/')
            return WorkspaceFile(
                name = name,
                path = path,
                relativePath = json.string("rel", "relativePath") ?: path,
                size = json.long("size"),
                text = json.bool("text") ?: false,
                directory = directory,
                modifiedAt = json.timestamp("mtime"),
            )
        }
    }
}

fun workspaceFileUri(path: String): String {
    if (path.startsWith("file:")) return URI.create(path).toASCIIString()
    val absolutePath = if (path.startsWith('/')) path else "/$path"
    return URI("file", "", absolutePath, null).toASCIIString()
}

enum class ChatBlockKind { USER, ASSISTANT, THINKING, TOOL, PERMISSION, PLAN, SYSTEM }
enum class ToolRunState { PENDING, RUNNING, SUCCESS, FAILED, CANCELLED }

data class PermissionOption(val id: String, val label: String)

data class ChatBlock(
    val id: String,
    val kind: ChatBlockKind,
    val text: String = "",
    val attachments: List<WorkspaceFile> = emptyList(),
    val title: String = "",
    val detail: String = "",
    val toolState: ToolRunState = ToolRunState.PENDING,
    val rpcId: Long? = null,
    val options: List<PermissionOption> = emptyList(),
)

data class ModelState(
    val currentModelId: String = "",
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
fun JsonObject.long(key: String): Long = (this[key] as? JsonPrimitive)?.contentOrNull?.toLongOrNull() ?: 0
fun JsonObject.obj(key: String): JsonObject? = this[key] as? JsonObject
fun JsonObject.objects(key: String): List<JsonObject> = (this[key] as? JsonArray)?.mapNotNull { it as? JsonObject }.orEmpty()

fun JsonObject.timestamp(vararg requestedKeys: String): Long {
    val keys = if (requestedKeys.isNotEmpty()) requestedKeys.asList() else DEFAULT_TIMESTAMP_KEYS
    return keys.asSequence().mapNotNull { key ->
        val value = this[key]
        if (value !is JsonPrimitive) return@mapNotNull null
        value.doubleOrNull?.let(::normalizeTimestamp)
            ?: value.contentOrNull?.toLongOrNull()?.let(::normalizeTimestamp)
    }.firstOrNull() ?: 0
}

private val DEFAULT_TIMESTAMP_KEYS = listOf(
    "lastActiveAt", "createdAt", "lastChangeUnixMs", "last_change_unix_ms", "updatedAt", "updated_at", "mtime",
)

private fun normalizeTimestamp(value: Double): Long =
    if (value < MILLISECOND_TIMESTAMP_THRESHOLD) (value * MILLISECONDS_PER_SECOND).toLong() else value.toLong()

private fun normalizeTimestamp(value: Long): Long =
    if (value < MILLISECOND_TIMESTAMP_THRESHOLD) value * MILLISECONDS_PER_SECOND else value

fun toolState(statusValue: String?): ToolRunState {
    val value = statusValue.orEmpty().lowercase()
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
