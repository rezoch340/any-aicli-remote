package com.anyaicliremote.app

import com.anyaicliremote.core.remote.ClientProductConfiguration

internal object ProductIdentifiers {
    val displayName: String = BuildConfig.PRODUCT_DISPLAY_NAME
    val pairingScheme: String = BuildConfig.PRODUCT_PAIRING_SCHEME
    val authorizationHeader: String = BuildConfig.PRODUCT_AUTHORIZATION_HEADER
    val profilePreferencesName: String = BuildConfig.PRODUCT_PREFERENCES_NAME
    val clientName: String = BuildConfig.PRODUCT_CLIENT_NAME
    val clientVersion: String = BuildConfig.VERSION_NAME
    val profileStorageConfiguration: com.anyaicliremote.core.storage.ProfileStorageConfiguration
        get() = com.anyaicliremote.core.storage.ProfileStorageConfiguration(
            preferencesName = profilePreferencesName,
            legacyPreferencesName = LegacyCompatibility.profilePreferencesName,
            legacyDisplayName = LegacyCompatibility.displayName,
            legacyBaseURLKey = LegacyCompatibility.legacyBaseURLKey,
            legacyPairingKey = LegacyCompatibility.legacyPairingKey,
            legacyDefaultWorkingDirectoryKey = LegacyCompatibility.legacyDefaultWorkingDirectoryKey,
        )
    val clientConfiguration: ClientProductConfiguration
        get() = ClientProductConfiguration(authorizationHeader, clientName, clientVersion)
}

internal object LegacyCompatibility {
    val displayName: String = BuildConfig.LEGACY_DISPLAY_NAME
    val pairingScheme: String = BuildConfig.LEGACY_PAIRING_SCHEME
    val authorizationHeader: String = BuildConfig.LEGACY_AUTHORIZATION_HEADER
    val profilePreferencesName: String = BuildConfig.LEGACY_PREFERENCES_NAME
    const val legacyBaseURLKey = "base_url"
    const val legacyPairingKey = "pairing_key"
    const val legacyDefaultWorkingDirectoryKey = "default_cwd"

    fun supportsPairingScheme(candidateScheme: String?): Boolean =
        candidateScheme == ProductIdentifiers.pairingScheme || candidateScheme == pairingScheme
}
