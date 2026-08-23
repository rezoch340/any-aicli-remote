package com.anyaicliremote.core.session

import com.anyaicliremote.core.model.ModelState
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.put
import org.junit.Assert.assertEquals
import org.junit.Test

class SessionPayloadMapperTest {
    @Test
    fun modelPayloadFallsBackWhenShapeIsMissing() {
        val fallback = ModelState(currentModelId = "known", effort = "high")
        assertEquals(fallback, SessionPayloadMapper.modelState(buildJsonObject { put("unexpected", true) }, fallback))
    }

    @Test
    fun sessionsPayloadSortsResidentAndUpdated() {
        val sessions = SessionPayloadMapper.sessions(buildJsonObject {
            put("sessions", kotlinx.serialization.json.buildJsonArray {
                add(buildJsonObject { put("providerId", "p"); put("sessionId", "old"); put("resident", false); put("lastActiveAt", 1) })
                add(buildJsonObject { put("providerId", "p"); put("sessionId", "resident"); put("resident", true); put("lastActiveAt", 2) })
            })
        })
        assertEquals("resident", sessions.first().id)
    }
}
