package com.anyaicliremote.core.session

import com.anyaicliremote.core.remote.ACPWire
import com.anyaicliremote.core.remote.AnyAICLIRemoteClient
import com.anyaicliremote.core.remote.ClientRuntimeConfiguration
import com.anyaicliremote.core.model.ChatBlock
import com.anyaicliremote.core.model.ModelState
import com.anyaicliremote.core.model.SessionMessage
import com.anyaicliremote.core.model.SessionSummary
import com.anyaicliremote.core.model.WorkspaceFile
import com.anyaicliremote.core.model.obj
import com.anyaicliremote.core.model.objects
import com.anyaicliremote.core.model.string
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.put

/** Owns session RPC payloads and response parsing without holding UI state. */
class SessionController(
    private val client: AnyAICLIRemoteClient,
    private val configuration: ClientRuntimeConfiguration,
) {
    suspend fun listSessions(): List<SessionSummary> =
        SessionPayloadMapper.sessions(client.rest("/api/sessions"))

    suspend fun listWorkspace(session: SessionSummary, path: String): WorkspaceListing {
        val response = client.rest(
            "/api/fs/list",
            query = mapOf("providerId" to session.providerId, "sessionId" to session.id, "path" to path),
        )
        return WorkspaceListing(
            path = response.string("path") ?: path,
            parent = response.string("parent"),
            directories = SessionPayloadMapper.workspaceDirectories(response),
            files = SessionPayloadMapper.workspaceFiles(response),
        )
    }

    suspend fun loadHistory(session: SessionSummary): SessionHistory {
        val response = client.rest(
            pathSegments = listOf("api", "sessions", session.id, "messages"),
            query = mapOf("providerId" to session.providerId),
        )
        val resolved = response.obj("session")?.let(SessionSummary::from)
            ?.takeIf { it.identity == session.identity } ?: session
        val blocks = SessionPayloadMapper.historyBlocks(
            resolved,
            response.objects("messages").mapNotNull(SessionMessage::from),
        )
        return SessionHistory(resolved, blocks)
    }

    suspend fun mount(session: SessionSummary, model: ModelState): ModelState {
        val response = client.rpc(
            ACPWire.loadSessionMethod,
            buildJsonObject {
                put("sessionId", session.id)
                put("mcpServers", JsonArray(emptyList()))
            },
            configuration.sessionLoadTimeout.inWholeMilliseconds,
        )
        return (response as? JsonObject)?.let {
            SessionPayloadMapper.modelState(it, model)
        } ?: model
    }

    suspend fun create(workingDirectory: String): CreatedSessionResponse {
        val response = client.rpc(
            ACPWire.newSessionMethod,
            ACPWire.newSessionParameters(workingDirectory),
            configuration.sessionCreateTimeout.inWholeMilliseconds,
        ).jsonObject
        val identifier = response.string("sessionId", "session_id")
            ?: response.obj("session")?.string("sessionId")
            ?: error("session/new 未返回 sessionId")
        return CreatedSessionResponse(
            identifier,
            (response.obj("session") ?: response).let(SessionSummary::from),
            listSessions(),
        )
    }

    suspend fun prompt(session: SessionSummary, message: String, files: List<WorkspaceFile>) {
        client.rpc(ACPWire.promptMethod, ACPWire.promptParameters(session.id, message, files))
    }

    suspend fun setEffort(session: SessionSummary, modelId: String?, effort: String) {
        client.rest("/api/effort", "POST", body = buildJsonObject {
            put("providerId", session.providerId)
            put("sessionId", session.id)
            put("modelId", modelId)
            put("effort", effort)
        })
    }

    fun cancel(session: SessionSummary) {
        client.notify(ACPWire.cancelMethod, ACPWire.cancelParameters(session.id))
    }

    fun answerPermission(requestId: Long, optionId: String?) {
        client.reply(requestId, buildJsonObject {
            put("outcome", buildJsonObject {
                if (optionId == null) put("outcome", "cancelled")
                else {
                    put("outcome", "selected")
                    put("optionId", optionId)
                }
            })
        })
    }
}

data class SessionHistory(val session: SessionSummary, val blocks: List<ChatBlock>)
data class CreatedSessionResponse(
    val identifier: String,
    val responseSession: SessionSummary?,
    val sessions: List<SessionSummary>,
)


data class WorkspaceListing(
    val path: String,
    val parent: String?,
    val directories: List<WorkspaceFile>,
    val files: List<WorkspaceFile>,
)
