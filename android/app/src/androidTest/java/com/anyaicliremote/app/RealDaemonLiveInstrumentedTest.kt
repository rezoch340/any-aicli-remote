package com.anyaicliremote.app

import androidx.compose.ui.test.hasAnyDescendant
import androidx.compose.ui.test.hasClickAction
import androidx.compose.ui.test.hasSetTextAction
import androidx.compose.ui.test.hasText
import androidx.compose.ui.test.junit4.createEmptyComposeRule
import androidx.compose.ui.test.onAllNodesWithContentDescription
import androidx.compose.ui.test.onAllNodesWithText
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performTextInput
import androidx.test.core.app.ActivityScenario
import androidx.test.ext.junit.runners.AndroidJUnit4
import java.io.File
import org.junit.After
import org.junit.Assume.assumeTrue
import org.junit.Before
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

@RunWith(AndroidJUnit4::class)
class RealDaemonLiveInstrumentedTest {
    @get:Rule val composeRule = createEmptyComposeRule()
    private lateinit var activityScenario: ActivityScenario<MainActivity>

    @Before fun setUp() {
        val context = androidx.test.platform.app.InstrumentationRegistry.getInstrumentation().targetContext
        context.deleteSharedPreferences(BuildConfig.PRODUCT_PREFERENCES_NAME)
        context.deleteSharedPreferences(BuildConfig.LEGACY_PREFERENCES_NAME)
        activityScenario = ActivityScenario.launch(MainActivity::class.java)
    }

    @After fun tearDown() { activityScenario.close() }

    @Test fun manualPairingReachesLiveDaemonSessions() {
        val keyFile = File(androidx.test.platform.app.InstrumentationRegistry.getInstrumentation().targetContext.filesDir, PAIRING_KEY_FILE)
        assumeTrue(keyFile.isFile)
        val pairingKey = keyFile.readText().trim()
        check(keyFile.delete())
        composeRule.onNodeWithText("手动添加").performClick()
        val fields = composeRule.onAllNodes(hasSetTextAction(), useUnmergedTree = true)
        fields[0].performTextInput(DEVICE_NAME)
        fields[1].performTextInput(DAEMON_ADDRESS)
        fields[2].performTextInput(pairingKey)
        composeRule.onNodeWithText("保存设备").performClick()
        composeRule.waitUntil(TIMEOUT_MILLIS) { composeRule.onAllNodesWithText(DEVICE_NAME, substring = true).fetchSemanticsNodes().isNotEmpty() }
        composeRule.onNode(hasClickAction() and hasAnyDescendant(hasText(DEVICE_NAME, substring = true)), useUnmergedTree = true).performClick()
        composeRule.waitUntil(TIMEOUT_MILLIS) { composeRule.onAllNodesWithContentDescription("新建会话").fetchSemanticsNodes().isNotEmpty() }
    }

    private companion object {
        const val DEVICE_NAME = "Live daemon"
        const val DAEMON_ADDRESS = "http://127.0.0.1:2421"
        const val PAIRING_KEY_FILE = "live-e2e-pairing-key"
        const val TIMEOUT_MILLIS = 20_000L
    }
}
