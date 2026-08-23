package com.anyaicliremote.core.storage

import android.content.Context
import android.content.SharedPreferences
import androidx.security.crypto.EncryptedSharedPreferences
import androidx.security.crypto.MasterKey
import com.anyaicliremote.core.model.SavedDevice
import kotlinx.serialization.Serializable
import kotlinx.serialization.SerializationException
import kotlinx.serialization.decodeFromString
import kotlinx.serialization.encodeToString
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.decodeFromJsonElement
import java.io.File
import java.security.GeneralSecurityException
import java.security.KeyStore
import java.util.UUID

data class ProfileStorageConfiguration(
    val preferencesName: String,
    val legacyPreferencesName: String,
    val legacyDisplayName: String,
    val legacyBaseURLKey: String,
    val legacyPairingKey: String,
    val legacyDefaultWorkingDirectoryKey: String,
)

class ProfileStorageException(message: String, cause: Throwable? = null) : IllegalStateException(message, cause)

class SecureProfileStore(context: Context, private val configuration: ProfileStorageConfiguration) {
    private companion object {
        const val DEVICES_KEY = "devices_v2"
    }

    private val applicationContext = context.applicationContext

    var recoveredCorruptedStorage: Boolean = false
        private set

    private var preferences: SharedPreferences = createPreferencesWithRecovery()
    private var legacyBrandStorageChecked = false

    // A platform crypto implementation may wrap security failures in RuntimeException.
    @Suppress("TooGenericExceptionCaught", "InstanceOfCheckForException")
    fun loadDevices(): List<SavedDevice> {
        return try {
            migrateLegacyBrandStorageIfNeeded()
            loadDevicesUnchecked()
        } catch (error: RuntimeException) {
            when {
                error is SerializationException -> recoverMalformedDeviceList()
                error is ClassCastException || error.hasSecurityCause() -> recoverUnreadablePreferences(error)
                else -> throw ProfileStorageException("无法读取设备配对信息", error)
            }
        }
    }

    fun saveDevice(device: SavedDevice): List<SavedDevice> {
        val currentDevices = loadDevices().toMutableList()
        val existingIndex = currentDevices.indexOfFirst { it.id == device.id }
        if (existingIndex >= 0) currentDevices[existingIndex] = device else currentDevices += device
        saveDevices(currentDevices)
        return currentDevices
    }

    fun deleteDevice(deviceId: String): List<SavedDevice> {
        val remainingDevices = loadDevices().filterNot { it.id == deviceId }
        saveDevices(remainingDevices)
        return remainingDevices
    }

    // Invalid legacy values are intentionally discarded after clearing their persisted source.
    @Suppress("SwallowedException")
    private fun loadDevicesUnchecked(): List<SavedDevice> {
        val encodedDevices = preferences.getString(DEVICES_KEY, null)
        if (encodedDevices != null) {
            val devices = DeviceProfileCodec.decode(encodedDevices)
            if (DeviceProfileCodec.containsLegacyWorkspace(encodedDevices)) saveDevices(devices)
            return devices
        }

        val legacyUrl = preferences.getString(configuration.legacyBaseURLKey, null) ?: return emptyList()
        val legacyKey = preferences.getString(configuration.legacyPairingKey, null) ?: return emptyList()
        val migratedDevice = try {
            SavedDevice.normalized(
                id = UUID.randomUUID().toString(),
                name = "",
                address = legacyUrl,
                pairingKey = legacyKey,
            )
        } catch (_: RuntimeException) {
            recoveredCorruptedStorage = true
            removeLegacyProfile()
            return emptyList()
        }
        commitOrThrow(
            preferences.edit()
                .putString(DEVICES_KEY, DeviceProfileCodec.encode(listOf(migratedDevice)))
                .remove(configuration.legacyBaseURLKey)
                .remove(configuration.legacyPairingKey)
                .remove(configuration.legacyDefaultWorkingDirectoryKey),
            "无法迁移旧设备配对信息",
        )
        return listOf(migratedDevice)
    }

    private fun saveDevices(devices: List<SavedDevice>) {
        commitOrThrow(
            preferences.edit().putString(DEVICES_KEY, DeviceProfileCodec.encode(devices)),
            "无法保存设备配对信息",
        )
    }

    private fun recoverMalformedDeviceList(): List<SavedDevice> {
        recoveredCorruptedStorage = true
        commitOrThrow(
            preferences.edit().remove(DEVICES_KEY),
            "设备配对信息已损坏，且无法清理",
        )
        migrateLegacyBrandStorageIfNeeded()
        return loadDevicesUnchecked()
    }

    // AndroidX crypto exposes implementation-specific exception subtypes during recovery.
    @Suppress("TooGenericExceptionCaught")
    private fun recoverUnreadablePreferences(error: Throwable): List<SavedDevice> {
        recoveredCorruptedStorage = true
        preferences = try {
            recreateCurrentPreferencesPreservingLegacyStorage(error)
        } catch (recoveryError: Exception) {
            recoveryError.addSuppressed(error)
            throw ProfileStorageException("设备配对信息无法解密，且安全存储恢复失败", recoveryError)
        }
        migrateLegacyBrandStorageIfNeeded()
        return loadDevicesUnchecked()
    }

    private fun removeLegacyProfile() {
        commitOrThrow(
            preferences.edit()
                .remove(configuration.legacyBaseURLKey)
                .remove(configuration.legacyPairingKey)
                .remove(configuration.legacyDefaultWorkingDirectoryKey),
            "旧设备配对信息已损坏，且无法清理",
        )
    }

    // Initialization must preserve the original platform exception as the recovery cause.
    @Suppress("TooGenericExceptionCaught")
    private fun createPreferencesWithRecovery(): SharedPreferences {
        return try {
            createPreferences(configuration.preferencesName)
        } catch (initialError: Exception) {
            recoveredCorruptedStorage = true
            try {
                recreateCurrentPreferencesPreservingLegacyStorage(initialError)
            } catch (recoveryError: Exception) {
                recoveryError.addSuppressed(initialError)
                throw ProfileStorageException("安全存储初始化失败", recoveryError)
            }
        }
    }

    private fun createPreferences(preferencesName: String): SharedPreferences {
        val masterKey = MasterKey.Builder(applicationContext)
            .setKeyScheme(MasterKey.KeyScheme.AES256_GCM)
            .build()
        return EncryptedSharedPreferences.create(
            applicationContext,
            preferencesName,
            masterKey,
            EncryptedSharedPreferences.PrefKeyEncryptionScheme.AES256_SIV,
            EncryptedSharedPreferences.PrefValueEncryptionScheme.AES256_GCM,
        )
    }

    // Keystore and encrypted-preference backends use heterogeneous exception types.
    @Suppress("TooGenericExceptionCaught")
    private fun recreateCurrentPreferencesPreservingLegacyStorage(initialError: Throwable): SharedPreferences {
        applicationContext.deleteSharedPreferences(configuration.preferencesName)
        try {
            return createPreferences(configuration.preferencesName)
        } catch (fileRecoveryError: Exception) {
            if (legacyBrandPreferencesFile().exists()) {
                fileRecoveryError.addSuppressed(initialError)
                throw ProfileStorageException(
                    "当前安全存储无法恢复；已保留旧版配对信息以避免数据丢失",
                    fileRecoveryError,
                )
            }
        }

        applicationContext.deleteSharedPreferences(configuration.preferencesName)
        val keyStore = KeyStore.getInstance("AndroidKeyStore").apply { load(null) }
        if (keyStore.containsAlias(MasterKey.DEFAULT_MASTER_KEY_ALIAS)) {
            keyStore.deleteEntry(MasterKey.DEFAULT_MASTER_KEY_ALIAS)
        }
        return createPreferences(configuration.preferencesName)
    }

    // Legacy encrypted storage can fail with backend-specific exceptions while migrating.
    @Suppress("TooGenericExceptionCaught")
    private fun migrateLegacyBrandStorageIfNeeded() {
        if (legacyBrandStorageChecked) return
        val legacyPreferencesFile = legacyBrandPreferencesFile()
        if (!legacyPreferencesFile.exists()) {
            legacyBrandStorageChecked = true
            return
        }

        val currentDevices = loadDevicesUnchecked()
        val legacyPreferences = try {
            createPreferences(configuration.legacyPreferencesName)
        } catch (error: Exception) {
            throw ProfileStorageException("无法读取旧版 ${configuration.legacyDisplayName} 配对信息", error)
        }
        val legacyDevices = try {
            readLegacyBrandDevices(legacyPreferences)
        } catch (error: RuntimeException) {
            throw ProfileStorageException("无法迁移旧版 ${configuration.legacyDisplayName} 配对信息", error)
        }
        val mergedDevices = DeviceProfileMigration.merge(currentDevices, legacyDevices)

        if (legacyDevices.isNotEmpty()) saveDevices(mergedDevices)
        commitOrThrow(
            legacyPreferences.edit().clear(),
            "旧版配对信息已迁移，但无法清理旧安全存储",
        )
        applicationContext.deleteSharedPreferences(configuration.legacyPreferencesName)
        legacyBrandStorageChecked = true
    }

    private fun readLegacyBrandDevices(legacyPreferences: SharedPreferences): List<SavedDevice> {
        val encodedDevices = legacyPreferences.getString(DEVICES_KEY, null)
        if (encodedDevices != null) return DeviceProfileCodec.decode(encodedDevices)

        val legacyUrl = legacyPreferences.getString(configuration.legacyBaseURLKey, null)
        val legacyKey = legacyPreferences.getString(configuration.legacyPairingKey, null)
        if (legacyUrl == null && legacyKey == null) return emptyList()
        if (legacyUrl == null || legacyKey == null) {
            throw SerializationException("旧版设备配对信息不完整")
        }
        return listOf(
            SavedDevice.normalized(
                id = UUID.randomUUID().toString(),
                name = "",
                address = legacyUrl,
                pairingKey = legacyKey,
            ),
        )
    }

    private fun legacyBrandPreferencesFile(): File =
        File(
            applicationContext.applicationInfo.dataDir,
            "shared_prefs/${configuration.legacyPreferencesName}.xml",
        )

    // SharedPreferences implementations may throw vendor-specific runtime failures on commit.
    @Suppress("TooGenericExceptionCaught")
    private fun commitOrThrow(editor: SharedPreferences.Editor, message: String) {
        val committed = try {
            editor.commit()
        } catch (error: RuntimeException) {
            throw ProfileStorageException(message, error)
        }
        if (!committed) throw ProfileStorageException(message)
    }

    private fun Throwable.hasSecurityCause(): Boolean =
        generateSequence(this) { it.cause }
            .any { cause -> cause is SecurityException || cause is GeneralSecurityException }
}

object DeviceProfileMigration {
    fun merge(currentDevices: List<SavedDevice>, legacyDevices: List<SavedDevice>): List<SavedDevice> {
        val mergedDevices = currentDevices.toMutableList()
        legacyDevices.forEach { legacyDevice ->
            val alreadyPresent = mergedDevices.any { currentDevice ->
                currentDevice.id == legacyDevice.id ||
                    (currentDevice.baseUrl == legacyDevice.baseUrl &&
                        currentDevice.pairingKey == legacyDevice.pairingKey)
            }
            if (!alreadyPresent) mergedDevices += legacyDevice
        }
        return mergedDevices
    }
}

object DeviceProfileCodec {
    private val json = Json { ignoreUnknownKeys = true }

    fun encode(devices: List<SavedDevice>): String =
        json.encodeToString(devices.map(StoredDeviceProfile::from))

    fun decode(encodedDevices: String): List<SavedDevice> {
        val records = decodeRecords(encodedDevices)
        return records.mapNotNull { element ->
            val storedDevice = runCatching {
                json.decodeFromJsonElement<StoredDeviceProfile>(element)
            }.getOrNull() ?: return@mapNotNull null
            val id = storedDevice.id ?: return@mapNotNull null
            val name = storedDevice.name ?: return@mapNotNull null
            val baseUrl = storedDevice.baseUrl ?: return@mapNotNull null
            val pairingKey = storedDevice.pairingKey ?: return@mapNotNull null
            try {
                SavedDevice.normalized(id, name, baseUrl, pairingKey)
            } catch (_: RuntimeException) {
                null
            }
        }
    }

    fun containsLegacyWorkspace(encodedDevices: String): Boolean {
        val records = decodeRecords(encodedDevices)
        return records.any { element ->
            val record = element as? JsonObject
            record?.containsKey("defaultCwd") == true || record?.containsKey("cwd") == true
        }
    }

    private fun decodeRecords(encodedDevices: String): JsonArray =
        runCatching { json.decodeFromString<JsonArray>(encodedDevices) }
            .getOrElse { error ->
                if (error is SerializationException) throw error
                throw SerializationException("设备列表不是 JSON 数组", error)
            }
}

@Serializable
private data class StoredDeviceProfile(
    val id: String? = null,
    val name: String? = null,
    val baseUrl: String? = null,
    val pairingKey: String? = null,
) {
    companion object {
        fun from(device: SavedDevice): StoredDeviceProfile = StoredDeviceProfile(
            id = device.id,
            name = device.name,
            baseUrl = device.baseUrl,
            pairingKey = device.pairingKey,
        )
    }
}
