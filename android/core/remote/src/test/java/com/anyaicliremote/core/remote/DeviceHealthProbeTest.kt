package com.anyaicliremote.core.remote

import org.junit.Assert.assertEquals
import org.junit.Test

class DeviceHealthProbeTest {
    @Test
    fun healthEndpointUsesOnlyBaseOriginAndHealthPath() {
        val endpoint = DeviceHealthProbe.healthEndpoint(
            "https://example.test:24443/pair?key=secret#fragment"
        )

        assertEquals("https://example.test:24443/health", endpoint.toString())
    }
}
