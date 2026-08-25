package com.anyaicliremote.core.remote

import com.anyaicliremote.core.model.SessionIdentity
import kotlinx.serialization.json.add
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.put
import kotlinx.serialization.json.putJsonArray
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test

class ACPEventDecoderTest {
    @Test
    fun decodesUpdateWithCompositeIdentity() {
        val event = ACPEventDecoder.decode(buildJsonObject {
            put("method", ACPWire.sessionUpdateMethod)
            put("params", buildJsonObject {
                put("providerId", "provider")
                put("sessionId", "session")
                put("update", buildJsonObject { put("sessionUpdate", "turn_completed") })
            })
        }) as ACPEvent.SessionUpdate
        assertEquals(SessionIdentity("provider", "session"), event.identity)
        assertEquals("turn_completed", event.update["sessionUpdate"]?.toString()?.trim('"'))
    }

    @Test
    fun missingIdentityIsRepresentedAndUnknownMessageIgnored() {
        val missing = ACPEventDecoder.decode(buildJsonObject { put("method", ACPWire.sessionUpdateMethod) }) as ACPEvent.SessionUpdate
        assertNull(missing.identity)
        assertNull(ACPEventDecoder.decode(buildJsonObject { put("method", "other/method") }))
    }

    @Test
    fun decodesRetryStatusUpdate() {
        val event = ACPEventDecoder.decode(buildJsonObject {
            put("method", ACPWire.statusUpdateMethod)
            put("params", buildJsonObject {
                put("providerId", "provider")
                put("sessionId", "session")
                put("retry", buildJsonObject {
                    put("phase", "retrying")
                    put("attempt", 2)
                    put("maxRetries", 5)
                    put("reason", "transient")
                })
            })
        }) as ACPEvent.SessionStatus
        val retry = event.status.retry!!
        assertEquals(2, retry.attempt)
        assertEquals(5, retry.maxRetries)
        assertEquals(com.anyaicliremote.core.model.RetryPhase.RETRYING, retry.phase)
        assertNull(event.status.modelSwitch)
    }

    @Test
    fun decodesModelSwitchAndIgnoresEmptyStatus() {
        val switch = ACPEventDecoder.decode(buildJsonObject {
            put("method", ACPWire.statusUpdateMethod)
            put("params", buildJsonObject {
                put("sessionId", "session")
                put("modelSwitch", buildJsonObject { put("current", "grok-3"); put("reason", "unavailable") })
            })
        }) as ACPEvent.SessionStatus
        assertEquals("grok-3", switch.status.modelSwitch!!.current)
        // A status update carrying neither retry nor modelSwitch is not an event.
        assertNull(
            ACPEventDecoder.decode(buildJsonObject {
                put("method", ACPWire.statusUpdateMethod)
                put("params", buildJsonObject { put("sessionId", "session") })
            }),
        )
    }

    @Test
    fun permissionQuestionComesFromToolCallTitle() {
        val event = ACPEventDecoder.decode(buildJsonObject {
            put("id", 7)
            put("method", "session/request_permission")
            put("params", buildJsonObject {
                put("sessionId", "session")
                put("toolCall", buildJsonObject { put("title", "bash: ls -la") })
                putJsonArray("options") {
                    add(buildJsonObject { put("optionId", "allow-once"); put("name", "允许一次") })
                }
            })
        }) as ACPEvent.PermissionRequest
        assertEquals("bash: ls -la", event.question)
        assertEquals("allow-once", event.options.single().id)
    }

    @Test
    fun wrongSessionIdentityRemainsDistinct() {
        val event = ACPEventDecoder.decode(buildJsonObject {
            put("method", ACPWire.sessionUpdateMethod)
            put("params", buildJsonObject { put("providerId", "other"); put("sessionId", "session") })
        }) as ACPEvent.SessionUpdate
        assertEquals(SessionIdentity("other", "session"), event.identity)
    }
}
