package com.anyaicliremote.app.model

import org.junit.Assert.assertEquals
import org.junit.Test

class SavedDeviceTest {
    @Test
    fun normalizationUsesPairingKeyFromUrlAndKeepsStableId() {
        val device = SavedDevice.normalized(
            id = "device-id",
            name = "Desk Mac",
            address = "https://mac.example:24443/connect?key=url-secret",
            pairingKey = "fallback-secret",
        )

        assertEquals("device-id", device.id)
        assertEquals("Desk Mac", device.name)
        assertEquals("https://mac.example:24443", device.baseUrl)
        assertEquals("url-secret", device.pairingKey)
    }

    @Test
    fun emptyNameFallsBackToHostAndNonDefaultPort() {
        val device = SavedDevice.normalized(
            id = "second-device",
            name = "",
            address = "http://192.0.2.10:2421",
            pairingKey = "secret",
        )

        assertEquals("192.0.2.10:2421", device.name)
    }
}
