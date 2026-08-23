package com.anyaicliremote.app.data

import org.junit.Test
import org.junit.Assert.assertEquals
import kotlin.time.Duration.Companion.seconds

class ClientRuntimeConfigurationTest {
    @Test
    fun defaultsAreCentralizedAndDependenciesAcceptConfiguration() {
        val defaults = ClientRuntimeConfiguration.Default
        assertEquals(20.seconds, defaults.connectTimeout)
        assertEquals(120.seconds, defaults.readTimeout)
        assertEquals(12.seconds, defaults.pingInterval)
        assertEquals(20.seconds, defaults.socketOpenTimeout)
        assertEquals(20.seconds, defaults.initializeTimeout)
        assertEquals(120.seconds, defaults.rpcTimeout)
        assertEquals(2.seconds, defaults.healthConnectTimeout)
        assertEquals(2.seconds, defaults.healthReadTimeout)
        assertEquals(3.seconds, defaults.healthCallTimeout)
        assertEquals(5.seconds, defaults.healthPollingInterval)
        assertEquals(90.seconds, defaults.sessionLoadTimeout)
        assertEquals(60.seconds, defaults.sessionCreateTimeout)

        val custom = defaults.copy(rpcTimeout = 7.seconds)
        val client = AnyAICLIRemoteClient(custom)
        val probe = DeviceHealthProbe(custom)
        client.close()
        probe.close()
    }
}
