package com.anyaicliremote.app.data

import com.anyaicliremote.app.model.SessionMessage
import com.anyaicliremote.app.model.SessionIdentity
import com.anyaicliremote.app.model.SessionSummary
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.jsonObject
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotNull
import org.junit.Test

import java.net.URI

class GenericSessionAPIContractTest {
    private val json = Json { ignoreUnknownKeys = true }

    @Test
    fun genericMetadataUsesProviderWorkspaceAndActivityTimestamp() {
        val metadata = json.parseToJsonElement(
            """
            {
              "providerId": "provider-one",
              "sessionId": "session-one",
              "title": "First session",
              "projectDir": "/workspace/project",
              "createdAt": 1776900000000,
              "lastActiveAt": 1776900123000
            }
            """.trimIndent()
        ).jsonObject

        val session = SessionSummary.from(metadata)

        assertNotNull(session)
        assertEquals("provider-one", session?.providerId)
        assertEquals("session-one", session?.id)
        assertEquals("/workspace/project", session?.projectDirectory)
        assertEquals(1776900000000, session?.createdAt)
        assertEquals(1776900123000, session?.lastActiveAt)
        assertEquals(1776900123000, session?.updatedAt)
        assertEquals(SessionIdentity("provider-one", "session-one"), session?.identity)
    }

    @Test
    fun normalizedMessageAcceptsOnlyGenericHistoryRoles() {
        val message = SessionMessage.from(
            json.parseToJsonElement(
                """{"role":"assistant","content":"hello","ts":1776900123000}"""
            ).jsonObject
        )
        val internalMessage = SessionMessage.from(
            json.parseToJsonElement(
                """{"role":"reasoning","content":"hidden","ts":1776900123000}"""
            ).jsonObject
        )

        assertEquals(SessionMessage("assistant", "hello", 1776900123000), message)
        assertEquals(null, internalMessage)
    }

    @Test
    fun messageRequestEncodesSessionAsOnePathSegment() {
        val requestURL = buildRestURL(
            baseURL = "https://remote.example:24443/old?discarded=true",
            pathSegments = listOf("api", "sessions", "session/with space%?#", "messages"),
            query = mapOf("providerId" to "provider-one"),
        )

        assertEquals("/api/sessions/session%2Fwith%20space%25%3F%23/messages", requestURL.encodedPath)
        assertEquals("provider-one", requestURL.queryParameter("providerId"))
        assertEquals(null, requestURL.queryParameter("discarded"))
    }

    @Test
    fun webSocketURLUsesSecureSchemeAndDropsBaseURLSecrets() {
        val secureURL = URI.create(
            buildWebSocketURL(
                "https://remote.example:24443/old/path?key=secret&discarded=true#fragment",
            ),
        )
        val insecureURL = URI.create(
            buildWebSocketURL("http://remote.example:2080/old?key=secret"),
        )

        assertEquals("wss", secureURL.scheme)
        assertEquals("/ws", secureURL.path)
        assertEquals(null, secureURL.rawQuery)
        assertEquals(null, secureURL.rawFragment)
        assertFalse(secureURL.toString().contains("secret"))
        assertFalse(secureURL.toString().contains("discarded"))
        assertEquals("ws", insecureURL.scheme)
        assertEquals("/ws", insecureURL.path)
        assertEquals(null, insecureURL.rawQuery)
    }
}
