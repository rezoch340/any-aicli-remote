package com.anyaicliremote.core.storage

import com.anyaicliremote.core.model.SavedDevice
import com.anyaicliremote.core.model.ServerProfile
import java.util.UUID

/** Validates and persists device profiles independently from screen state. */
class DeviceProfileController(private val store: SecureProfileStore) {
    fun save(requestedId: String?, name: String, address: String, pairingKey: String, existing: List<SavedDevice>): List<SavedDevice> {
        val profile = ServerProfile.parse(address, pairingKey)
        val matching = existing.firstOrNull { it.baseUrl == profile.baseUrl }
        require(requestedId == null || matching == null || matching.id == requestedId) {
            "该服务地址已由设备“${matching?.name}”使用"
        }
        val identifier = requestedId ?: matching?.id ?: UUID.randomUUID().toString()
        val device = SavedDevice.normalized(identifier, name.trim().ifEmpty { matching?.name.orEmpty() }, profile.baseUrl, profile.key)
        return store.saveDevice(device)
    }
    fun delete(identifier: String): List<SavedDevice> = store.deleteDevice(identifier)
}
