package com.anyaicliremote.app

import android.content.Intent
import android.net.Uri
import android.content.pm.PackageManager
import androidx.test.core.app.ActivityScenario
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import androidx.test.uiautomator.UiDevice
import com.anyaicliremote.app.data.SecureProfileStore
import java.net.InetAddress
import java.net.ServerSocket
import java.util.UUID
import org.junit.Before
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test
import org.junit.runner.RunWith

@RunWith(AndroidJUnit4::class)
class PairingPersistenceInstrumentedTest {
    private val instrumentation = InstrumentationRegistry.getInstrumentation()
    private val applicationContext = instrumentation.targetContext
    private val device = UiDevice.getInstance(instrumentation)

    @Before
    fun clearTargetState() {
        applicationContext.deleteSharedPreferences(BuildConfig.PRODUCT_PREFERENCES_NAME)
        applicationContext.deleteSharedPreferences(BuildConfig.LEGACY_PREFERENCES_NAME)
    }

    @Test
    fun applicationLabelUsesCurrentBrand() {
        val label = applicationContext.packageManager
            .getApplicationInfo(applicationContext.packageName, PackageManager.GET_META_DATA)
            .loadLabel(applicationContext.packageManager)
            .toString()
        assertEquals(BuildConfig.PRODUCT_DISPLAY_NAME, label)
        assertTrue(label != BuildConfig.LEGACY_DISPLAY_NAME)
    }

    @Test
    fun currentPairingDeepLinkPersistsAcrossActivityRestart() {
        val profile = ProfileFixture("Current device")

        val pairingActivity = launchPairing(profile, BuildConfig.PRODUCT_PAIRING_SCHEME)
        try {
            assertForeground(pairingActivity)
            assertEquals(null, pairingActivity.intent.data)
            assertPersisted(profile)
        } finally {
            pairingActivity.finishAndRemoveTask()
        }

        ActivityScenario.launch<MainActivity>(mainIntent()).use {
            assertForeground(it)
            assertPersisted(profile)
        }
    }

    @Test
    fun legacyPairingDeepLinkPersistsAcrossActivityRestart() {
        val profile = ProfileFixture("Legacy device")

        val pairingActivity = launchPairing(profile, BuildConfig.LEGACY_PAIRING_SCHEME)
        try {
            assertForeground(pairingActivity)
            assertEquals(null, pairingActivity.intent.data)
            assertPersisted(profile)
        } finally {
            pairingActivity.finishAndRemoveTask()
        }

        ActivityScenario.launch<MainActivity>(mainIntent()).use {
            assertForeground(it)
            assertPersisted(profile)
        }
    }

    @Test
    fun twoProfilesRemainVisibleWhenBothDaemonsAreOffline() {
        val currentProfile = ProfileFixture("Current daemon")
        val legacyProfile = ProfileFixture("Legacy daemon")

        launchPairing(currentProfile, BuildConfig.PRODUCT_PAIRING_SCHEME).finishAndRemoveTask()
        launchPairing(legacyProfile, BuildConfig.LEGACY_PAIRING_SCHEME).finishAndRemoveTask()

        ActivityScenario.launch<MainActivity>(mainIntent()).use {
            assertForeground(it)

            val savedProfiles = SecureProfileStore(applicationContext).loadDevices()
            assertEquals(2, savedProfiles.size)
            assertEquals(
                setOf(currentProfile.address, legacyProfile.address),
                savedProfiles.map { it.baseUrl }.toSet(),
            )
            assertEquals(2, savedProfiles.count { it.name in setOf(currentProfile.name, legacyProfile.name) })
        }
    }

    private fun launchPairing(profile: ProfileFixture, scheme: String): MainActivity {
        val intent = Intent(Intent.ACTION_VIEW, pairingUri(profile, scheme)).apply {
            setClass(applicationContext, MainActivity::class.java)
            setPackage(applicationContext.packageName)
            addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)
        }
        return instrumentation.startActivitySync(intent) as MainActivity
    }

    private fun pairingUri(profile: ProfileFixture, scheme: String): Uri =
        Uri.Builder()
            .scheme(scheme)
            .authority("pair")
            .appendQueryParameter("url", profile.address)
            .appendQueryParameter("key", profile.key)
            .appendQueryParameter("name", profile.name)
            .build()

    private fun mainIntent(): Intent = Intent(Intent.ACTION_MAIN).apply {
        setClass(applicationContext, MainActivity::class.java)
        setPackage(applicationContext.packageName)
    }

    private fun assertPersisted(profile: ProfileFixture) {
        val savedProfile = SecureProfileStore(applicationContext).loadDevices()
            .single { it.name == profile.name }
        assertEquals(profile.address, savedProfile.baseUrl)
    }

    private fun assertForeground(activity: MainActivity) {
        assertTrue("Activity unexpectedly finishing", !activity.isFinishing)
        assertEquals(applicationContext.packageName, device.currentPackageName)
    }

    private fun assertForeground(scenario: ActivityScenario<MainActivity>) {
        scenario.onActivity { activity -> assertForeground(activity) }
    }

    private class ProfileFixture(val name: String) {
        private val loopbackHost = requireNotNull(InetAddress.getLoopbackAddress().hostAddress) { "Loopback address unavailable" }.let { host ->
            if (host.contains(':')) "[$host]" else host
        }
        val address: String = "http://$loopbackHost:${unusedPort()}"
        val key: String = "fixture-${UUID.randomUUID()}"
    }

    private companion object {
        fun unusedPort(): Int = ServerSocket(0).use { it.localPort }
    }
}
