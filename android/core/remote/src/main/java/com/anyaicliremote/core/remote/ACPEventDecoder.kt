package com.anyaicliremote.core.remote

import com.anyaicliremote.core.model.PermissionOption
import com.anyaicliremote.core.model.SessionIdentity
import com.anyaicliremote.core.model.obj
import com.anyaicliremote.core.model.objects
import com.anyaicliremote.core.model.string
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.longOrNull

sealed interface ACPEvent {
    data class SessionUpdate(val identity: SessionIdentity?, val update: JsonObject) : ACPEvent
    data object SessionsChanged : ACPEvent
    data class PermissionRequest(
        val requestId: Long,
        val identity: SessionIdentity?,
        val question: String,
        val options: List<PermissionOption>,
    ) : ACPEvent
}

/** Decodes notification/request JSON without deciding whether the current UI accepts it. */
object ACPEventDecoder {
    fun decode(message: JsonObject): ACPEvent? {
        val method = message.string("method") ?: return null
        val parameters = message.obj("params") ?: JsonObject(emptyMap())
        return when {
            method == ACPWire.sessionUpdateMethod -> ACPEvent.SessionUpdate(
                identity = parameters.sessionIdentity(),
                update = parameters.obj("update") ?: parameters,
            )
            method == ACPWire.sessionsChangedMethod -> ACPEvent.SessionsChanged
            ACPWire.isPermissionMethod(method) -> decodePermission(message, parameters)
            else -> null
        }
    }

    private fun decodePermission(message: JsonObject, parameters: JsonObject): ACPEvent.PermissionRequest? {
        val requestId = (message["id"] as? JsonPrimitive)?.longOrNull ?: return null
        val options = parameters.objects("options").map {
            PermissionOption(
                id = it.string("optionId", "id") ?: "allow",
                label = it.string("name", "label") ?: "允许",
            )
        }.ifEmpty { listOf(PermissionOption("allow", "允许")) }
        return ACPEvent.PermissionRequest(
            requestId = requestId,
            identity = parameters.sessionIdentity(),
            question = parameters.string("question", "message") ?: "AI CLI 需要你的确认",
            options = options,
        )
    }

    private fun JsonObject.sessionIdentity(): SessionIdentity? {
        val providerIdentifier = string("providerId")
        val sessionIdentifier = string("sessionId")
        return if (providerIdentifier == null || sessionIdentifier == null) null else SessionIdentity(providerIdentifier, sessionIdentifier)
    }
}
