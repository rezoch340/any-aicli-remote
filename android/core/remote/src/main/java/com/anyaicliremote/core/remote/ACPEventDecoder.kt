package com.anyaicliremote.core.remote

import com.anyaicliremote.core.model.ChildAgentCard
import com.anyaicliremote.core.model.InteractionKind
import com.anyaicliremote.core.model.InteractionOption
import com.anyaicliremote.core.model.InteractionQuestion
import com.anyaicliremote.core.model.PendingInteraction
import com.anyaicliremote.core.model.PermissionOption
import com.anyaicliremote.core.model.SessionIdentity
import com.anyaicliremote.core.model.bool
import com.anyaicliremote.core.model.childAgentStatus
import com.anyaicliremote.core.model.long
import com.anyaicliremote.core.model.obj
import com.anyaicliremote.core.model.objects
import com.anyaicliremote.core.model.string
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.doubleOrNull
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
    data class ChildAgentUpdate(
        val identity: SessionIdentity?,
        val card: ChildAgentCard,
    ) : ACPEvent
    data class Interaction(val request: PendingInteraction) : ACPEvent
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
            method == ACPWire.childAgentUpdateMethod -> decodeChildAgent(parameters)
            method == ACPWire.interactionRequestMethod -> decodeInteraction(message, parameters)
            ACPWire.isPermissionMethod(method) -> decodePermission(message, parameters)
            else -> null
        }
    }

    private fun decodeChildAgent(parameters: JsonObject): ACPEvent.ChildAgentUpdate? {
        val event = parameters.obj("event") ?: return null
        val agent = event.obj("agent") ?: return null
        val providerChildId = agent.string("providerChildId") ?: return null
        val card = ChildAgentCard(
            providerChildId = providerChildId,
            childSessionId = agent.string("childSessionId") ?: "",
            agentType = agent.string("agentType") ?: "",
            description = agent.string("description") ?: "",
            status = childAgentStatus(agent.string("status")),
            startedAt = agent.long("startedAt"),
            completedAt = agent.long("completedAt"),
            toolCallCount = agent.long("toolCallCount").toInt(),
            turnCount = agent.long("turnCount").toInt(),
            modelId = agent.string("modelId") ?: "",
            tokensUsed = agent.long("tokensUsed"),
            contextUsagePercent = (event.obj("agent")?.get("contextUsagePercent") as? JsonPrimitive)?.doubleOrNull ?: 0.0,
            sequence = (event["sequence"] as? JsonPrimitive)?.longOrNull,
        )
        return ACPEvent.ChildAgentUpdate(identity = parameters.sessionIdentity(), card = card)
    }

    private fun decodeInteraction(message: JsonObject, parameters: JsonObject): ACPEvent.Interaction? {
        val requestId = (message["id"] as? JsonPrimitive)?.longOrNull ?: return null
        val identity = parameters.sessionIdentity() ?: return null
        val kind = when (parameters.string("kind")) {
            "ask_question" -> InteractionKind.ASK_QUESTION
            "exit_plan" -> InteractionKind.EXIT_PLAN
            else -> return null
        }
        val toolCallId = parameters.string("toolCallId") ?: return null
        val questions = parameters.objects("questions").map { question ->
            InteractionQuestion(
                question = question.string("question") ?: "",
                options = question.objects("options").map {
                    InteractionOption(label = it.string("label") ?: "", description = it.string("description") ?: "")
                },
                multiSelect = question.bool("multiSelect") ?: false,
            )
        }
        return ACPEvent.Interaction(
            PendingInteraction(
                rpcId = requestId,
                kind = kind,
                sessionIdentity = identity,
                toolCallId = toolCallId,
                questions = questions,
                planContent = parameters.string("planContent") ?: "",
                mode = parameters.string("mode") ?: "",
            ),
        )
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
