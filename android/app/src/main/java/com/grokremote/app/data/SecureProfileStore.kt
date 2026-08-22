package com.grokremote.app.data

import android.content.Context
import androidx.security.crypto.EncryptedSharedPreferences
import androidx.security.crypto.MasterKey
import com.grokremote.app.model.ServerProfile

class SecureProfileStore(context: Context) {
    private val masterKey = MasterKey.Builder(context)
        .setKeyScheme(MasterKey.KeyScheme.AES256_GCM)
        .build()

    private val prefs = EncryptedSharedPreferences.create(
        context,
        "grok_remote_profile",
        masterKey,
        EncryptedSharedPreferences.PrefKeyEncryptionScheme.AES256_SIV,
        EncryptedSharedPreferences.PrefValueEncryptionScheme.AES256_GCM,
    )

    fun load(): ServerProfile? {
        val url = prefs.getString("base_url", null) ?: return null
        val key = prefs.getString("pairing_key", null) ?: return null
        return ServerProfile(url, key)
    }

    fun save(profile: ServerProfile) {
        prefs.edit().putString("base_url", profile.baseUrl).putString("pairing_key", profile.key).apply()
    }

    fun defaultCwd(): String = prefs.getString("default_cwd", "~") ?: "~"
    fun saveDefaultCwd(cwd: String) { prefs.edit().putString("default_cwd", cwd).apply() }
}
