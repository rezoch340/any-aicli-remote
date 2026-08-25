package com.anyaicliremote.app

import androidx.compose.ui.test.hasAnyDescendant
import androidx.compose.ui.test.hasClickAction
import androidx.compose.ui.test.hasSetTextAction
import androidx.compose.ui.test.hasText
import androidx.compose.ui.test.hasTestTag
import androidx.compose.ui.test.junit4.createEmptyComposeRule
import androidx.compose.ui.test.onAllNodesWithContentDescription
import androidx.compose.ui.test.onAllNodesWithText
import androidx.compose.ui.test.onNodeWithContentDescription
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onAllNodesWithTag
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performTextInput
import androidx.compose.ui.test.performTextReplacement
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
        openLiveSessions()
    }

    @Test fun toolOutputAndPermissionTitleAgainstLiveDaemon() {
        openNewLiveChat()
        sendPrompt(
            "必须使用 file write 工具（不要使用 bash）在当前 workspace 创建隐藏文件 " +
                "`.any-ai-cli-live-permission-command-marker`，文件内容必须是 $OUTPUT_MARKER。" +
                "完成后只输出独立 marker $PERMISSION_TURN_MARKER，不调用其他工具。",
        )
        awaitText("需要确认")
        awaitText(".any-ai-cli-live-permission-command-marker")
        clickPermissionAllowOnce()
        awaitTextAbsent("需要确认")
        awaitExactText(PERMISSION_TURN_MARKER)
        sendPrompt(
            "必须调用 bash 工具执行 `cat .any-ai-cli-live-permission-command-marker && " +
                "rm -f .any-ai-cli-live-permission-command-marker`，只做该动作。" +
                "禁止复述文件内容，完成后只输出 $TURN_MARKER。",
        )
        awaitText("需要确认")
        awaitText(".any-ai-cli-live-permission-command-marker")
        clickPermissionAllowOnce()
        awaitTextAbsent("需要确认")
        awaitExactText(TURN_MARKER)
        composeRule.onNode(
            hasTestTag("tool-row") and hasAnyDescendant(hasText("Execute", substring = true)),
            useUnmergedTree = true,
        ).performClick()
        awaitText(OUTPUT_MARKER)
    }

    @Test fun askNotesSubmitAndPlanModeAgainstLiveDaemon() {
        openNewLiveChat()
        sendPrompt(
            "必须先调用 enter_plan_mode，再调用 ask_user_question，只问缓存，" +
                "选项严格为 Redis 和进程内 LRU。" +
                "收到回答后，只有选择 Redis 且 user notes 精确为 $NOTES_MARKER 才输出 $PLAN_MARKER；" +
                "不要 exit plan，不调用其他工具。",
        )
        awaitAskOrPlanSurface()
        awaitText("计划模式")
        awaitText("先聊一下")
        awaitText("跳过")
        awaitText("取消")
        awaitText("提交")
        enterAnswer("其他回答 / 备注", NOTES_MARKER)
        clickOption("Redis")
        composeRule.onNodeWithText("提交", substring = true).performClick()
        awaitExactText(PLAN_MARKER)
    }

    @Test fun askCancelAgainstLiveDaemon() {
        openNewLiveChat()
        sendPrompt("必须调用 ask_user_question，提供固定选项 Redis 和进程内 LRU。取消后只输出 $CANCEL_MARKER，不调用其他工具。")
        awaitText("助手需要你的确认")
        composeRule.onNodeWithText("取消", substring = false).performClick()
        awaitTextAbsent("助手需要你的确认")
        awaitExactText(CANCEL_MARKER)
    }

    @Test fun askChatAboutAgainstLiveDaemon() {
        openNewLiveChat()
        sendPrompt(
            "必须先调用 enter_plan_mode，再调用 ask_user_question，" +
                "提供固定选项 Redis 和进程内 LRU。" +
                "收到 chat_about_this 后只输出 $CHAT_ABOUT_MARKER，不 exit plan，不调用其他工具。",
        )
        awaitAskOrPlanSurface()
        awaitText("计划模式")
        awaitText("先聊一下")
        clickOption("Redis")
        composeRule.onNodeWithText("先聊一下", substring = true).performClick()
        awaitTextAbsent("助手需要你的确认")
        awaitExactText(CHAT_ABOUT_MARKER)
    }

    private fun openNewLiveChat() {
        openLiveSessions()
        composeRule.onNodeWithContentDescription("新建会话").performClick()
        composeRule.onNodeWithText("创建", substring = true).performClick()
        composeRule.waitUntil(TIMEOUT_MILLIS) {
            composeRule.onAllNodes(hasSetTextAction(), useUnmergedTree = true)
                .fetchSemanticsNodes()
                .isNotEmpty()
        }
    }

    private fun openLiveSessions() {
        val context = androidx.test.platform.app.InstrumentationRegistry.getInstrumentation().targetContext
        val pairingKey = synchronized(KEY_LOCK) {
            cachedPairingKey ?: run {
                val keyFile = File(context.filesDir, PAIRING_KEY_FILE)
                assumeTrue(keyFile.isFile)
                keyFile.readText().trim().also { cachedPairingKey = it; check(keyFile.delete()) }
            }
        }
        composeRule.onNodeWithText("手动添加").performClick()
        val fields = composeRule.onAllNodes(hasSetTextAction(), useUnmergedTree = true)
        fields[0].performTextInput(DEVICE_NAME)
        fields[1].performTextInput(DAEMON_ADDRESS)
        fields[2].performTextInput(pairingKey)
        composeRule.onNodeWithText("保存设备").performClick()
        awaitText(DEVICE_NAME)
        composeRule.onNode(
            hasClickAction() and hasAnyDescendant(hasText(DEVICE_NAME, substring = true)),
            useUnmergedTree = true,
        ).performClick()
        composeRule.waitUntil(TIMEOUT_MILLIS) {
            composeRule.onAllNodesWithContentDescription("新建会话")
                .fetchSemanticsNodes()
                .isNotEmpty()
        }
    }

    private fun sendPrompt(prompt: String) {
        val inputs = composeRule.onAllNodes(hasSetTextAction(), useUnmergedTree = true)
        inputs[inputs.fetchSemanticsNodes().lastIndex].performTextInput(prompt)
        composeRule.onNodeWithContentDescription("发送").performClick()
    }

    private fun awaitText(value: String) = composeRule.waitUntil(TIMEOUT_MILLIS) {
        composeRule.onAllNodesWithText(value, substring = true).fetchSemanticsNodes().isNotEmpty()
    }

    private fun awaitTextAbsent(value: String) = composeRule.waitUntil(TIMEOUT_MILLIS) {
        composeRule.onAllNodesWithText(value, substring = true).fetchSemanticsNodes().isEmpty()
    }

    private fun awaitExactText(value: String) = composeRule.waitUntil(TIMEOUT_MILLIS) {
        composeRule.onAllNodesWithText(value, substring = false).fetchSemanticsNodes().isNotEmpty()
    }

    private fun clickOption(label: String) {
        composeRule.onNode(
            hasClickAction() and hasAnyDescendant(hasText(label, substring = false)),
            useUnmergedTree = true,
        ).performClick()
    }

    private fun clickPermissionAllowOnce() {
        composeRule.onNodeWithTag("permission-option-allow-once").performClick()
    }

    private fun awaitAskOrPlanSurface() {
        composeRule.waitUntil(TIMEOUT_MILLIS) {
            val permissionVisible = composeRule.onAllNodesWithTag("permission-option-allow-once")
                .fetchSemanticsNodes()
                .isNotEmpty()
            if (permissionVisible) {
                clickPermissionAllowOnce()
                return@waitUntil false
            }
            val planVisible = composeRule.onAllNodesWithText("计划模式", substring = true)
                .fetchSemanticsNodes()
                .isNotEmpty()
            val askVisible = composeRule.onAllNodesWithText("助手需要你的确认", substring = true)
                .fetchSemanticsNodes()
                .isNotEmpty()
            planVisible || askVisible
        }
    }

    private fun enterAnswer(label: String, value: String) {
        composeRule.onNodeWithText(label, substring = true).performClick()
        val inputs = composeRule.onAllNodes(hasSetTextAction(), useUnmergedTree = true)
        inputs[inputs.fetchSemanticsNodes().lastIndex].performTextReplacement(value)
    }

    private companion object {
        const val DEVICE_NAME = "Live daemon"
        const val DAEMON_ADDRESS = "http://127.0.0.1:2421"
        const val PAIRING_KEY_FILE = "live-e2e-pairing-key"
        const val TIMEOUT_MILLIS = 120_000L
        const val OUTPUT_MARKER = "LIVE_TOOL_OUTPUT_MARKER"
        const val PERMISSION_TURN_MARKER = "LIVE_PERMISSION_TURN_MARKER"
        const val TURN_MARKER = "LIVE_TOOL_TURN_MARKER"
        const val NOTES_MARKER = "LIVE_ASK_NOTES_MARKER"
        const val PLAN_MARKER = "LIVE_PLAN_SUCCESS_MARKER"
        const val CANCEL_MARKER = "LIVE_ASK_CANCEL_MARKER"
        const val CHAT_ABOUT_MARKER = "LIVE_CHAT_ABOUT_MARKER"
        private val KEY_LOCK = Any()
        private var cachedPairingKey: String? = null
    }
}
