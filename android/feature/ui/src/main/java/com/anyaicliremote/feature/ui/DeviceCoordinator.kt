package com.anyaicliremote.feature.ui

import com.anyaicliremote.core.storage.DeviceProfileController

/** 配对表单状态与已保存设备的增删改。 */
internal class DeviceCoordinator(
    private val scope: ChatOperationScope,
    private val profileController: DeviceProfileController?,
    private val onActiveDeviceReleased: () -> Unit,
) {
    fun updatePairing(name: String? = null, address: String? = null, key: String? = null) {
        scope.update {
            it.copy(
                deviceName = name ?: it.deviceName,
                address = address ?: it.address,
                pairingKey = key ?: it.pairingKey,
            )
        }
    }

    fun beginAddDevice() {
        scope.update {
            it.copy(
                destination = AppDestination.PAIRING,
                editingDeviceId = null,
                deviceName = "",
                address = "",
                pairingKey = "",
                error = null,
            )
        }
    }

    fun beginEditDevice(deviceId: String) {
        val device = scope.state.devices.firstOrNull { it.id == deviceId } ?: return
        scope.update {
            it.copy(
                destination = AppDestination.PAIRING,
                editingDeviceId = device.id,
                deviceName = device.name,
                address = device.baseUrl,
                pairingKey = device.pairingKey,
                error = null,
            )
        }
    }

    fun cancelPairing() {
        scope.update {
            it.copy(
                destination = AppDestination.DEVICES,
                editingDeviceId = null,
                error = null,
            )
        }
    }

    fun savePairing() {
        saveDevice(
            requestedId = scope.state.editingDeviceId,
            name = scope.state.deviceName,
            address = scope.state.address,
            pairingKey = scope.state.pairingKey,
        )
    }

    fun importPairing(address: String, key: String, name: String?) {
        onActiveDeviceReleased()
        saveDevice(
            requestedId = null,
            name = name.orEmpty(),
            address = address,
            pairingKey = key,
        )
    }

    private fun saveDevice(requestedId: String?, name: String, address: String, pairingKey: String) {
        UiOperationRunner.runSynchronously(scope::showError) {
            val devices = requireProfileController().save(
                requestedId, name, address, pairingKey, scope.state.devices,
            )
            scope.update { it.copy(destination = AppDestination.DEVICES, devices = devices, error = null) }
        }
    }

    fun deleteDevice(deviceId: String) {
        if (scope.state.activeDeviceId == deviceId) {
            onActiveDeviceReleased()
        }
        UiOperationRunner.runSynchronously(scope::showError) {
            scope.update {
                it.copy(
                    devices = requireProfileController().delete(deviceId),
                    error = null,
                )
            }
        }
    }

    private fun requireProfileController(): DeviceProfileController =
        profileController ?: error("安全存储不可用，请重启应用后重试")
}
