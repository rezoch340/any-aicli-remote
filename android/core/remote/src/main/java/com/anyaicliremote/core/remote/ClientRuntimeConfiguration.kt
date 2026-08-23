package com.anyaicliremote.core.remote

import kotlin.time.Duration
import kotlin.time.Duration.Companion.seconds

data class ClientRuntimeConfiguration(
    val connectTimeout: Duration = 20.seconds,
    val readTimeout: Duration = 120.seconds,
    val pingInterval: Duration = 12.seconds,
    val socketOpenTimeout: Duration = 20.seconds,
    val initializeTimeout: Duration = 20.seconds,
    val rpcTimeout: Duration = 120.seconds,
    val healthConnectTimeout: Duration = 2.seconds,
    val healthReadTimeout: Duration = 2.seconds,
    val healthCallTimeout: Duration = 3.seconds,
    val healthPollingInterval: Duration = 5.seconds,
    val sessionLoadTimeout: Duration = 90.seconds,
    val sessionCreateTimeout: Duration = 60.seconds,
) {
    companion object {
        val Default = ClientRuntimeConfiguration()
    }
}
