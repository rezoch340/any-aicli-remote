package com.anyaicliremote.core.remote

import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import okhttp3.HttpUrl
import okhttp3.HttpUrl.Companion.toHttpUrl
import okhttp3.OkHttpClient
import okhttp3.Request
import java.io.IOException
import java.util.concurrent.TimeUnit

class DeviceHealthProbe(private val configuration: ClientRuntimeConfiguration = ClientRuntimeConfiguration.Default) {
    private val httpClient = OkHttpClient.Builder()
        .connectTimeout(configuration.healthConnectTimeout.inWholeMilliseconds, TimeUnit.MILLISECONDS)
        .readTimeout(configuration.healthReadTimeout.inWholeMilliseconds, TimeUnit.MILLISECONDS)
        .callTimeout(configuration.healthCallTimeout.inWholeMilliseconds, TimeUnit.MILLISECONDS)
        .build()

    suspend fun isOnline(baseUrl: String): Boolean = withContext(Dispatchers.IO) {
        val request = Request.Builder()
            .url(healthEndpoint(baseUrl))
            .get()
            .header("Accept", "application/json")
            .build()
        try {
            httpClient.newCall(request).execute().use { response -> response.isSuccessful }
        } catch (_: IOException) {
            false
        }
    }

    fun close() {
        httpClient.dispatcher.cancelAll()
        httpClient.dispatcher.executorService.shutdown()
        httpClient.connectionPool.evictAll()
    }

    internal companion object {
        fun healthEndpoint(baseUrl: String): HttpUrl = baseUrl.toHttpUrl().newBuilder()
            .encodedPath("/health")
            .query(null)
            .fragment(null)
            .build()
    }
}
