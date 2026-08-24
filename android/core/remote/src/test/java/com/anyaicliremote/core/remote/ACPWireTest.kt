package com.anyaicliremote.core.remote

import com.agentclientprotocol.rpc.JsonRpcRequest
import com.agentclientprotocol.rpc.JsonRpcResponse
import com.anyaicliremote.core.model.WorkspaceFile
import com.anyaicliremote.core.model.workspaceFileUri
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.jsonArray
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import kotlinx.serialization.json.put
import okhttp3.Request
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

class ACPWireTest {
    @Test
    fun officialModelFramesRequestsAndResponses() {
        val encodedRequest = ACPWire.encodeRequest(
            identifier = 42,
            method = "session/prompt",
            params = buildJsonObject { put("sessionId", "session-one") },
        )
        val request = ACPWire.decode(encodedRequest) as JsonRpcRequest
        assertEquals("session/prompt", request.method.name)
        assertEquals("session-one", request.params?.jsonObject?.get("sessionId")?.jsonPrimitive?.content)

        val encodedResponse = ACPWire.encodeResponse(42, JsonPrimitive("done"))
        val response = ACPWire.decode(encodedResponse) as JsonRpcResponse
        assertEquals(42L, ACPWire.responseIdentifier(response))
        assertEquals("done", response.result?.jsonPrimitive?.content)
    }

    @Test
    fun unknownIncomingRequestGetsOfficialMethodNotFoundResponse() {
        val request = ACPWire.decode(
            """{"jsonrpc":"2.0","id":"provider-7","method":"terminal/create","params":{}}"""
        ) as JsonRpcRequest

        val response = ACPWire.decode(ACPWire.encodeMethodNotFoundResponse(request)) as JsonRpcResponse

        assertEquals("provider-7", response.id.value)
        assertEquals(-32601, response.error?.code)
        assertEquals("Method not found", response.error?.message)
        assertNull(response.result)
    }

    @Test
    fun incomingRequestClassificationLeavesPermissionRequestsForUi() {
        assertEquals(
            ACPWire.IncomingRequestDisposition.UI_HANDLED,
            ACPWire.classifyIncomingRequest("session/request_permission"),
        )
        assertEquals(
            ACPWire.IncomingRequestDisposition.UI_HANDLED,
            ACPWire.classifyIncomingRequest("session/interaction_request"),
        )
        assertEquals(
            ACPWire.IncomingRequestDisposition.METHOD_NOT_FOUND,
            ACPWire.classifyIncomingRequest("terminal/create"),
        )
    }

    @Test
    fun initializeParametersComeFromOfficialACPModel() {
        val parameters = ACPWire.initializeParameters("test-client", "1.2.3")

        assertEquals(1, parameters["protocolVersion"]?.jsonPrimitive?.content?.toInt())
        assertEquals("test-client", parameters["clientInfo"]?.jsonObject?.get("name")?.jsonPrimitive?.content)
        assertTrue(parameters["clientCapabilities"]?.jsonObject?.get("terminal")?.jsonPrimitive?.content?.toBoolean() == true)
    }

    @Test
    fun standardLifecycleParametersUseOfficialACPModels() {
        val newSession = ACPWire.newSessionParameters("/workspace/project")
        val prompt = ACPWire.promptParameters("session-one", "hello")
        val cancellation = ACPWire.cancelParameters("session-one")

        assertEquals("/workspace/project", newSession["cwd"]?.jsonPrimitive?.content)
        assertEquals("session-one", prompt["sessionId"]?.jsonPrimitive?.content)
        assertEquals(
            "hello",
            prompt["prompt"]?.let { element ->
                element.jsonArray.first().jsonObject["text"]?.jsonPrimitive?.content
            },
        )
        assertEquals("session-one", cancellation["sessionId"]?.jsonPrimitive?.content)
    }

    @Test
    fun promptEncodesWorkspaceFilesAsOfficialResourceLinks() {
        val file = WorkspaceFile("main #中文.kt", "/workspace/main #中文.kt", "src/main #中文.kt", size = 42)
        val prompt = ACPWire.promptParameters("session-one", "inspect", listOf(file))
        val resource = prompt["prompt"]!!.jsonArray[1].jsonObject

        assertEquals("resource_link", resource["type"]?.jsonPrimitive?.content)
        assertEquals(file.name, resource["name"]?.jsonPrimitive?.content)
        assertEquals("file:///workspace/main%20%23%E4%B8%AD%E6%96%87.kt", resource["uri"]?.jsonPrimitive?.content)
        assertEquals(file.relativePath, resource["description"]?.jsonPrimitive?.content)
        assertEquals("42", resource["size"]?.jsonPrimitive?.content)
    }

    @Test
    fun workspaceFileUriEscapesPathUsingJavaUri() {
        assertEquals(
            "file:///workspace/a%20b/%23%E4%B8%AD%E6%96%87.txt",
            workspaceFileUri("/workspace/a b/#中文.txt"),
        )
    }

    @Test
    fun newRequestsSendOnlyCurrentAuthorizationHeader() {
        val productConfiguration = ClientProductConfiguration("X-Test-Key", "test-client", "1.2.3")
        val request = Request.Builder()
            .url("https://remote.example/health")
            .authorizeWithProductKey(productConfiguration, "pairing-secret")
            .build()

        assertEquals("pairing-secret", request.header(productConfiguration.authorizationHeader))
        assertNull(request.header("X-Other-Key"))
    }
}
