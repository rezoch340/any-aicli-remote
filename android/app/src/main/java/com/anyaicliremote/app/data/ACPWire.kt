package com.anyaicliremote.app.data

import com.agentclientprotocol.model.AcpMethod
import com.agentclientprotocol.annotations.UnstableApi
import com.agentclientprotocol.model.ClientCapabilities
import com.agentclientprotocol.model.ContentBlock
import com.agentclientprotocol.model.CancelNotification
import com.agentclientprotocol.model.FileSystemCapability
import com.agentclientprotocol.model.Implementation
import com.agentclientprotocol.model.InitializeRequest
import com.agentclientprotocol.model.LATEST_PROTOCOL_VERSION
import com.agentclientprotocol.model.NewSessionRequest
import com.agentclientprotocol.model.PromptRequest
import com.agentclientprotocol.model.SessionId
import com.anyaicliremote.app.model.WorkspaceFile
import com.agentclientprotocol.rpc.ACPJson
import com.agentclientprotocol.rpc.JsonRpcMessage
import com.agentclientprotocol.rpc.JsonRpcNotification
import com.agentclientprotocol.rpc.JsonRpcError
import com.agentclientprotocol.rpc.JsonRpcErrorCode
import com.agentclientprotocol.rpc.JsonRpcRequest
import com.agentclientprotocol.rpc.JsonRpcResponse
import com.agentclientprotocol.rpc.MethodName
import com.agentclientprotocol.rpc.RequestId
import com.agentclientprotocol.rpc.decodeJsonRpcMessage
import kotlinx.serialization.encodeToString
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.jsonObject

/**
 * ACP/JSON-RPC framing is owned by the official ACP Kotlin model. OkHttp remains
 * the transport because the daemon uses an authenticated WebSocket rather than
 * the SDK's stdio transport.
 */
@OptIn(UnstableApi::class)
internal object ACPWire {
    enum class IncomingRequestDisposition {
        UI_HANDLED,
        METHOD_NOT_FOUND,
    }

    val initializeMethod: String = AcpMethod.AgentMethods.Initialize.toString()
    val loadSessionMethod: String = AcpMethod.AgentMethods.SessionLoad.toString()
    val newSessionMethod: String = AcpMethod.AgentMethods.SessionNew.toString()
    val promptMethod: String = AcpMethod.AgentMethods.SessionPrompt.toString()
    val cancelMethod: String = AcpMethod.AgentMethods.SessionCancel.toString()

    fun initializeParameters(clientName: String, clientVersion: String): JsonObject =
        ACPJson.encodeToJsonElement(
            InitializeRequest.serializer(),
            InitializeRequest(
                protocolVersion = LATEST_PROTOCOL_VERSION,
                clientCapabilities = ClientCapabilities(
                    fs = FileSystemCapability(
                        readTextFile = true,
                        writeTextFile = true,
                    ),
                    terminal = true,
                ),
                clientInfo = Implementation(
                    name = clientName,
                    version = clientVersion,
                ),
            ),
        ).jsonObject

    fun newSessionParameters(workingDirectory: String): JsonObject =
        ACPJson.encodeToJsonElement(
            NewSessionRequest.serializer(),
            NewSessionRequest(
                cwd = workingDirectory,
                mcpServers = emptyList(),
            ),
        ).jsonObject

    fun promptParameters(
        sessionIdentifier: String,
        text: String,
        attachments: List<WorkspaceFile> = emptyList(),
    ): JsonObject =
        ACPJson.encodeToJsonElement(
            PromptRequest.serializer(),
            PromptRequest(
                sessionId = SessionId(sessionIdentifier),
                prompt = buildList {
                    add(ContentBlock.Text(text))
                    attachments.forEach { file ->
                        add(
                            ContentBlock.ResourceLink(
                                name = file.name,
                                uri = file.uri,
                                description = file.relativePath,
                                size = file.size.takeIf { it > 0 },
                            )
                        )
                    }
                },
            ),
        ).jsonObject

    fun cancelParameters(sessionIdentifier: String): JsonObject =
        ACPJson.encodeToJsonElement(
            CancelNotification.serializer(),
            CancelNotification(sessionId = SessionId(sessionIdentifier)),
        ).jsonObject

    fun encodeRequest(identifier: Long, method: String, params: JsonElement): String =
        ACPJson.encodeToString(
            JsonRpcRequest.serializer(),
            JsonRpcRequest(
                id = numericRequestIdentifier(identifier),
                method = MethodName(method),
                params = params,
            ),
        )

    fun encodeNotification(method: String, params: JsonElement): String =
        ACPJson.encodeToString(
            JsonRpcNotification.serializer(),
            JsonRpcNotification(
                method = MethodName(method),
                params = params,
            ),
        )

    fun encodeResponse(identifier: Long, result: JsonElement): String =
        ACPJson.encodeToString(
            JsonRpcResponse.serializer(),
            JsonRpcResponse(
                id = numericRequestIdentifier(identifier),
                result = result,
            ),
        )

    fun classifyIncomingRequest(method: String): IncomingRequestDisposition {
        val normalizedMethod = method.lowercase()
        return if (normalizedMethod.contains("permission") || normalizedMethod.contains("ask_user")) {
            IncomingRequestDisposition.UI_HANDLED
        } else {
            IncomingRequestDisposition.METHOD_NOT_FOUND
        }
    }

    fun encodeMethodNotFoundResponse(request: JsonRpcRequest): String =
        ACPJson.encodeToString(
            JsonRpcResponse.serializer(),
            JsonRpcResponse(
                id = request.id,
                result = null,
                error = JsonRpcError(
                    code = JsonRpcErrorCode.METHOD_NOT_FOUND.code,
                    message = JsonRpcErrorCode.METHOD_NOT_FOUND.message,
                    data = null,
                ),
            ),
        )

    fun decode(message: String): JsonRpcMessage = decodeJsonRpcMessage(message)

    fun responseIdentifier(response: JsonRpcResponse): Long? =
        when (val value = response.id.value) {
            is Int -> value.toLong()
            is String -> value.toLongOrNull()
            else -> null
        }

    fun requestAsJson(request: JsonRpcRequest): JsonObject =
        ACPJson.encodeToJsonElement(JsonRpcRequest.serializer(), request).jsonObject

    fun notificationAsJson(notification: JsonRpcNotification): JsonObject =
        ACPJson.encodeToJsonElement(JsonRpcNotification.serializer(), notification).jsonObject

    private fun numericRequestIdentifier(identifier: Long): RequestId {
        require(identifier in 0..Int.MAX_VALUE.toLong()) { "JSON-RPC request identifier exhausted" }
        return RequestId.create(identifier.toInt())
    }
}
