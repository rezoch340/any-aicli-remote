package com.anyaicliremote.core.remote

import com.anyaicliremote.core.model.SessionIdentity
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.put
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
    fun wrongSessionIdentityRemainsDistinct() {
        val event = ACPEventDecoder.decode(buildJsonObject {
            put("method", ACPWire.sessionUpdateMethod)
            put("params", buildJsonObject { put("providerId", "other"); put("sessionId", "session") })
        }) as ACPEvent.SessionUpdate
        assertEquals(SessionIdentity("other", "session"), event.identity)
    }
}
