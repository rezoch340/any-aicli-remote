package com.anyaicliremote.app.data

import com.anyaicliremote.app.model.SavedDevice
import kotlinx.serialization.SerializationException
import org.junit.Assert.assertEquals
import org.junit.Test

class DeviceProfileCodecTest {
    @Test
    fun roundTripPreservesMultipleDevices() {
        val devices = listOf(
            SavedDevice("first-id", "First Mac", "http://192.0.2.10:2421", "first-secret"),
            SavedDevice("second-id", "Second Mac", "https://remote.example:24443", "second-secret"),
        )

        val restoredDevices = DeviceProfileCodec.decode(DeviceProfileCodec.encode(devices))

        assertEquals(devices, restoredDevices)
    }

    @Test
    fun malformedRecordsDoNotDiscardValidDevices() {
        val encodedDevices = """
            [
              {"id":"valid-id","name":"Valid","baseUrl":"http://localhost:2421","pairingKey":"secret","defaultCwd":"~"},
              {"id":"missing-fields"}
            ]
        """.trimIndent()

        val restoredDevices = DeviceProfileCodec.decode(encodedDevices)

        assertEquals(listOf("valid-id"), restoredDevices.map(SavedDevice::id))
        assertEquals(true, DeviceProfileCodec.containsLegacyWorkspace(encodedDevices))
    }

    @Test
    fun encodedProfilesDoNotContainWorkspace() {
        val encoded = DeviceProfileCodec.encode(
            listOf(SavedDevice("device-id", "Mac", "http://localhost:2421", "secret"))
        )

        assertEquals(false, encoded.contains("defaultCwd"))
        assertEquals(false, encoded.contains("cwd"))
    }

    @Test(expected = SerializationException::class)
    fun malformedDocumentFailsExplicitly() {
        DeviceProfileCodec.decode("not-json")
    }

    @Test
    fun brandMigrationKeepsCurrentAndLegacyDevices() {
        val currentDevice = SavedDevice(
            "current-id",
            "Current Mac",
            "http://192.0.2.10:2421",
            "current-secret",
        )
        val legacyDevice = SavedDevice(
            "legacy-id",
            "Legacy Mac",
            "https://remote.example:24443",
            "legacy-secret",
        )

        val mergedDevices = DeviceProfileMigration.merge(
            currentDevices = listOf(currentDevice),
            legacyDevices = listOf(legacyDevice),
        )

        assertEquals(listOf(currentDevice, legacyDevice), mergedDevices)
    }

    @Test
    fun brandMigrationDoesNotDuplicateSamePairing() {
        val currentDevice = SavedDevice(
            "current-id",
            "Current name",
            "http://192.0.2.10:2421",
            "same-secret",
        )
        val legacyDevice = currentDevice.copy(id = "legacy-id", name = "Legacy name")

        val mergedDevices = DeviceProfileMigration.merge(
            currentDevices = listOf(currentDevice),
            legacyDevices = listOf(legacyDevice),
        )

        assertEquals(listOf(currentDevice), mergedDevices)
    }
}
