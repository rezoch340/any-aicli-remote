package com.anyaicliremote.feature.ui

import com.anyaicliremote.core.remote.ClientRuntimeConfiguration
import com.anyaicliremote.core.remote.DeviceHealthProbe
import com.anyaicliremote.core.model.DeviceHealthStatus
import com.anyaicliremote.core.model.SavedDevice
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Job
import kotlinx.coroutines.async
import kotlinx.coroutines.awaitAll
import kotlinx.coroutines.coroutineScope
import kotlinx.coroutines.delay
import kotlinx.coroutines.isActive
import kotlinx.coroutines.launch

/** Owns the lifecycle and stale-result protection for saved-device health checks. */
internal class DeviceHealthMonitor(
    private val scope: CoroutineScope,
    private val configuration: ClientRuntimeConfiguration,
    private val healthProbe: DeviceHealthProbe,
    private val devices: () -> List<SavedDevice>,
    private val healthStatuses: () -> Map<String, DeviceHealthStatus>,
    private val publish: (Map<String, DeviceHealthStatus>) -> Unit,
) {
    private var monitoringJob: Job? = null

    fun start() {
        if (monitoringJob?.isActive == true) return
        monitoringJob = scope.launch {
            while (isActive) {
                refresh()
                delay(configuration.healthPollingInterval)
            }
        }
    }

    fun stop() {
        monitoringJob?.cancel()
        monitoringJob = null
    }

    fun restart() {
        stop()
        start()
    }

    suspend fun refresh() {
        val savedDevices = devices()
        if (savedDevices.isEmpty()) {
            publish(emptyMap())
            return
        }
        val addresses = savedDevices.associate { device -> device.id to device.baseUrl }
        publish(savedDevices.associate { device ->
            device.id to (healthStatuses()[device.id] ?: DeviceHealthStatus.CHECKING)
        })
        val results = coroutineScope {
            savedDevices.map { device ->
                async {
                    val health = if (healthProbe.isOnline(device.baseUrl)) {
                        DeviceHealthStatus.ONLINE
                    } else {
                        DeviceHealthStatus.OFFLINE
                    }
                    device.id to health
                }
            }.awaitAll().toMap()
        }
        val currentAddresses = devices().associate { device -> device.id to device.baseUrl }
        publish(savedDevices.associate { device ->
            val unchanged = addresses[device.id] == currentAddresses[device.id]
            val fallback = healthStatuses()[device.id] ?: DeviceHealthStatus.CHECKING
            device.id to if (unchanged) results[device.id] ?: fallback else fallback
        })
    }
}
