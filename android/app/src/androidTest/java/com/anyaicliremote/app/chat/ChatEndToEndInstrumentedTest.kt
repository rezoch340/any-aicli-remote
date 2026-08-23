package com.anyaicliremote.app.chat

import android.content.Context
import androidx.compose.ui.test.hasClickAction
import androidx.compose.ui.test.assert
import androidx.compose.ui.test.hasAnyDescendant
import androidx.compose.ui.test.hasSetTextAction
import androidx.compose.ui.test.hasScrollAction
import androidx.compose.ui.test.isEnabled
import androidx.compose.ui.test.hasText
import androidx.compose.ui.test.junit4.createEmptyComposeRule
import androidx.compose.ui.test.onAllNodesWithText
import androidx.compose.ui.test.onAllNodesWithContentDescription
import androidx.compose.ui.test.onNodeWithContentDescription
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performTextInput
import androidx.compose.ui.test.performTextReplacement
import androidx.compose.ui.test.performTouchInput
import androidx.compose.ui.test.swipeDown
import androidx.compose.ui.test.assertIsDisplayed
import androidx.test.core.app.ApplicationProvider
import androidx.test.core.app.ActivityScenario
import androidx.test.ext.junit.runners.AndroidJUnit4
import com.anyaicliremote.app.BuildConfig
import com.anyaicliremote.app.MainActivity
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import kotlinx.serialization.json.put
import org.junit.After
import org.junit.Before
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import java.util.concurrent.atomic.AtomicBoolean
import java.util.concurrent.atomic.AtomicReference

@RunWith(AndroidJUnit4::class)
class ChatEndToEndInstrumentedTest {
    private companion object {
        const val STREAM_CHUNK_COUNT = 40
        const val STREAM_CHUNK_INTERVAL_MILLIS = 50L
        const val STREAM_TIMEOUT_MILLIS = 20_000L
        const val STREAM_MARKER = "AUTOSCROLLENDMARKER"
    }
    @get:Rule val composeRule = createEmptyComposeRule()

    private lateinit var fixture: TestDaemonFixture
    private lateinit var activityScenario: ActivityScenario<MainActivity>
    private val applicationContext: Context
        get() = ApplicationProvider.getApplicationContext()

    @Before
    fun setUp() {
        clearProfileStorage()
        fixture = TestDaemonFixture()
        fixture.sessionsBody = sessionsJson()
        fixture.messagesBody = historyJson()
        fixture.filesBody = filesJson()
        activityScenario = ActivityScenario.launch(MainActivity::class.java)
        composeRule.waitForIdle()
    }

    @After
    fun tearDown() {
        activityScenario.close()
        fixture.close()
        clearProfileStorage()
    }

    @Test
    fun pairingHintExposesCameraScanAction() {
        composeRule.onNodeWithContentDescription("扫码添加，点按打开相机扫描")
            .assertExists()
            .assert(hasClickAction())
    }

    @Test
    fun websocketUpgradeKeepsAuthorizationOutOfUrl() {
        openFixtureSessions()

        val websocketRequest = fixture.requestsFor("/ws").last()
        check(websocketRequest.requestUrl?.queryParameter("key") == null)
        check(!websocketRequest.path.orEmpty().contains("test-key-not-secret"))
        check(websocketRequest.getHeader(BuildConfig.PRODUCT_AUTHORIZATION_HEADER) == "test-key-not-secret")
    }

    @Test
    fun newSessionUsesExplicitWorkspaceAndOpensMatchingSession() {
        openFixtureSessions()
        composeRule.onNodeWithContentDescription("新建会话").performClick()
        val fields = composeRule.onAllNodes(hasSetTextAction(), useUnmergedTree = true)
        fields[fields.fetchSemanticsNodes().lastIndex].performTextReplacement("/workspace/new-project")
        composeRule.onNodeWithText("创建", substring = false).performClick()

        val request = fixture.awaitRequest("session/new")
        check(request["params"]!!.jsonObject["cwd"]?.jsonPrimitive?.content == "/workspace/new-project")
        awaitText("New workspace", substring = true)
        composeRule.onNodeWithContentDescription("添加附件").assertExists()
    }

    @Test
    fun existingSession_historyLoadsWithoutCwdQuery() {
        openFixtureSession()

        awaitText("历史消息", substring = true)
        val historyRequest = fixture.requestsFor("/api/sessions/session-1/messages").last()
        check(historyRequest.requestUrl?.queryParameter("providerId") == "grok")
        check(historyRequest.requestUrl?.queryParameter("cwd") == null)
    }

    @Test
    fun existingSession_historyReplayDoesNotShowStopButton() {
        fixture.afterSessionLoadResponse = {
            fixture.sendNotification("session/update", updateJson("agent_thought_chunk", "历史思考"))
            repeat(12) { index ->
                val title = "历史工具-${index + 1}"
                fixture.sendNotification("session/update", updateJson("tool_call", title, title = title))
            }
        }
        openFixtureSession()

        awaitText("思考过程", substring = true)
        awaitText("历史工具-12", substring = true)
        composeRule.waitUntil(5_000) {
            composeRule.onAllNodesWithContentDescription("滚动到底部").fetchSemanticsNodes().isEmpty()
        }
        composeRule.onNodeWithContentDescription("停止").assertDoesNotExist()
    }

    @Test
    fun prompt_streamsRichMarkdownAndHidesStop() {
        fixture.onRequest = { request ->
            if (request["method"]?.jsonPrimitive?.content == "session/prompt") {
                fixture.sendNotification("session/update", updateJson("agent_message_chunk", "# Done\n\n| A | B |\n| --- | --- |\n| 1 | 2 |\n\n"))
                fixture.sendNotification("session/update", updateJson("agent_message_chunk", "```kotlin\nval answer = 42\n```"))
                fixture.sendNotification("session/update", updateJson("turn_completed", ""))
                fixture.respondTo(request)
            }
        }
        openFixtureSession()
        sendChat("show markdown")

        awaitText("Done", substring = true)
        awaitText("answer = 42", substring = true)
        composeRule.waitUntil(10_000) {
            composeRule.onAllNodesWithContentDescription("停止").fetchSemanticsNodes().isEmpty()
        }
    }

    @Test
    fun longStreamFollowsBottomAndCanBeRestoredAfterUserScroll() {
        val streamStarted = AtomicBoolean(false)
        val streamCompleted = AtomicBoolean(false)
        val streamFailure = AtomicReference<Throwable?>(null)
        var streamThread: Thread? = null
        fixture.onRequest = { request ->
            if (request["method"]?.jsonPrimitive?.content == "session/prompt" && streamStarted.compareAndSet(false, true)) {
                streamThread = Thread {
                    try {
                        repeat(STREAM_CHUNK_COUNT) { index ->
                            val chunkText = "\n\n### Stream chunk ${index + 1}\n\nThis is deliberately long streaming content with enough separate Markdown paragraphs to make each chunk occupy visible vertical space. It keeps arriving while the user browses older content.\n\n"
                            fixture.sendNotification("session/update", updateJson("agent_message_chunk", chunkText))
                            Thread.sleep(STREAM_CHUNK_INTERVAL_MILLIS)
                        }
                        fixture.sendNotification("session/update", updateJson("agent_message_chunk", STREAM_MARKER))
                        fixture.sendNotification("session/update", updateJson("turn_completed", ""))
                        fixture.respondTo(request)
                        streamCompleted.set(true)
                    } catch (failure: Throwable) {
                        streamFailure.set(failure)
                    }
                }.also { it.start() }
            }
        }
        try {
            openFixtureSession()
            sendChat("long stream")
            composeRule.waitUntil(STREAM_TIMEOUT_MILLIS) {
                composeRule.onAllNodesWithText("Stream chunk 15", substring = true).fetchSemanticsNodes().isNotEmpty()
            }
            composeRule.onNodeWithContentDescription("滚动到底部").assertDoesNotExist()
            composeRule.onNode(hasScrollAction(), useUnmergedTree = true).performTouchInput { swipeDown() }
            composeRule.waitUntil(STREAM_TIMEOUT_MILLIS) {
                composeRule.onAllNodesWithContentDescription("滚动到底部").fetchSemanticsNodes().isNotEmpty()
            }
            composeRule.onNodeWithContentDescription("滚动到底部").assertIsDisplayed()
            composeRule.waitUntil(STREAM_TIMEOUT_MILLIS) { streamCompleted.get() }
            composeRule.onNodeWithContentDescription("滚动到底部").assertIsDisplayed()
            check(streamFailure.get() == null) { "stream fixture failed" }
            composeRule.onNodeWithContentDescription("滚动到底部").performClick()
            composeRule.onNodeWithText(STREAM_MARKER, substring = true).assertIsDisplayed()
            fixture.messagesBody = historyJson().replace("历史消息", STREAM_MARKER)
            composeRule.onNodeWithContentDescription("返回").performClick()
            awaitText("Fixture session", substring = true)
            composeRule.onNode(hasClickAction() and hasAnyDescendant(hasText("Fixture session", substring = true)), useUnmergedTree = true).performClick()
            awaitText(STREAM_MARKER, substring = true)
            composeRule.onNodeWithText(STREAM_MARKER, substring = true).assertIsDisplayed()
        } finally {
            streamThread?.join(STREAM_TIMEOUT_MILLIS)
            check(streamThread?.isAlive != true) { "stream fixture thread did not finish" }
            check(streamFailure.get() == null) { "stream fixture failed: ${streamFailure.get()}" }
        }
    }

    @Test
    fun permissionSelectionSendsMatchingJsonRpcResponse() {
        fixture.onRequest = { request ->
            if (request["method"]?.jsonPrimitive?.content == "session/prompt") {
                fixture.sendRequest(
                    identifier = 77,
                    method = "session/request_permission",
                    params = permissionJson(),
                )
            }
        }
        openFixtureSession()
        sendChat("run the command")

        awaitText("Allow once", substring = true)
        composeRule.onNodeWithText("Allow once", substring = true).performClick()
        val response = fixture.awaitResponse(77)
        val outcome = response["result"]!!.jsonObject["outcome"]!!.jsonObject
        check(outcome["outcome"]?.jsonPrimitive?.content == "selected")
        check(outcome["optionId"]?.jsonPrimitive?.content == "allow-once")
        composeRule.waitUntil(5_000) {
            composeRule.onAllNodesWithText("Allow once", substring = true).fetchSemanticsNodes().isEmpty()
        }
    }

    @Test
    fun stopSendsSessionCancelAndHidesStop() {
        fixture.onRequest = { request ->
            if (request["method"]?.jsonPrimitive?.content == "session/prompt") {
                // Keep the request pending until the client sends session/cancel.
            }
        }
        openFixtureSession()
        sendChat("keep working")
        fixture.awaitRequest("session/prompt")
        composeRule.waitUntil(10_000) {
            composeRule.onAllNodesWithContentDescription("停止").fetchSemanticsNodes().isNotEmpty()
        }
        composeRule.onNodeWithContentDescription("停止").performClick()

        val cancel = fixture.awaitRequest("session/cancel")
        check(cancel["params"]!!.jsonObject["sessionId"]?.jsonPrimitive?.content == "session-1")
        composeRule.onNodeWithContentDescription("停止").assertDoesNotExist()
    }

    @Test
    fun unexpectedDisconnectReturnsToDevicesAndSavedProfileReconnects() {
        openFixtureSession()
        val oldGeneration = fixture.currentSocketGeneration()
        fixture.closeSocket()

        awaitText("设备", substring = false)
        awaitText("Fixture device", substring = true)
        composeRule.onNode(hasClickAction() and hasAnyDescendant(hasText("Fixture device", substring = true)), useUnmergedTree = true).performClick()
        awaitText("Fixture session", substring = true)
        check(fixture.awaitSocketGeneration(oldGeneration) > oldGeneration)
    }

    @Test
    fun workspaceFileSelectionUsesOfficialResourceLinkPrompt() {
        fixture.onRequest = { request ->
            if (request["method"]?.jsonPrimitive?.content == "session/prompt") fixture.respondTo(request)
        }
        openFixtureSession()
        composeRule.onNodeWithContentDescription("添加附件").performClick()
        awaitText("README.md", substring = true)
        composeRule.onNode(hasClickAction() and hasAnyDescendant(hasText("README.md", substring = true)), useUnmergedTree = true).performClick()
        composeRule.onNodeWithText("完成", substring = false).performClick()
        composeRule.onNodeWithContentDescription("发送").performClick()

        val prompt = fixture.awaitRequest("session/prompt")
        val blocks = prompt["params"]!!.jsonObject["prompt"]!!.toString()
        check(blocks.contains("resource_link"))
        check(blocks.contains("file:///workspace/README.md"))
        check(blocks.contains("README.md"))
        awaitText("README.md", substring = true)
    }

    private fun openFixtureSessions() {
        composeRule.onNodeWithText("手动添加").performClick()
        val fields = composeRule.onAllNodes(hasSetTextAction(), useUnmergedTree = true)
        fields[0].performTextInput("Fixture device")
        fields[1].performTextInput(fixture.baseUrl)
        fields[2].performTextInput("test-key-not-secret")
        composeRule.onNodeWithText("保存设备").performClick()
        composeRule.waitUntil(10_000) { composeRule.onAllNodesWithText("Fixture device", substring = true).fetchSemanticsNodes().isNotEmpty() }
        composeRule.onNode(hasClickAction() and hasAnyDescendant(hasText("Fixture device", substring = true)), useUnmergedTree = true).performClick()
        fixture.awaitSocket()
        awaitText("Fixture session", substring = true)
    }

    private fun openFixtureSession() {
        composeRule.onNodeWithText("手动添加").performClick()
        val fields = composeRule.onAllNodes(hasSetTextAction(), useUnmergedTree = true)
        fields[0].performTextInput("Fixture device")
        fields[1].performTextInput(fixture.baseUrl)
        fields[2].performTextInput("test-key-not-secret")
        composeRule.onNodeWithText("保存设备").performClick()
        composeRule.waitUntil(10_000) {
            composeRule.onAllNodesWithText("Fixture device", substring = true).fetchSemanticsNodes().isNotEmpty()
        }
        composeRule.onNode(hasClickAction() and hasAnyDescendant(hasText("Fixture device", substring = true)), useUnmergedTree = true).performClick()
        fixture.awaitSocket()
        awaitText("Fixture session", substring = true)
        composeRule.onNode(hasClickAction() and hasAnyDescendant(hasText("Fixture session", substring = true)), useUnmergedTree = true).performClick()
        fixture.awaitRequest("session/load")
        composeRule.onNodeWithContentDescription("返回").assertExists()
        composeRule.waitUntil(10_000) {
            composeRule.onAllNodes(hasSetTextAction(), useUnmergedTree = true).fetchSemanticsNodes().isNotEmpty()
        }
    }

    private fun sendChat(text: String) {
        val inputs = composeRule.onAllNodes(hasSetTextAction(), useUnmergedTree = true)
        val lastInput = inputs[inputs.fetchSemanticsNodes().lastIndex]
        lastInput.performTextReplacement(text)
        composeRule.waitUntil(10_000) {
            composeRule.onAllNodesWithContentDescription("发送").fetchSemanticsNodes().isNotEmpty()
        }
        composeRule.onNodeWithContentDescription("发送").performClick()
    }

    private fun awaitText(value: String, substring: Boolean = false) {
        composeRule.waitUntil(15_000) {
            composeRule.onAllNodesWithText(value, substring = substring).fetchSemanticsNodes().isNotEmpty()
        }
    }

    private fun clearProfileStorage() {
        applicationContext.deleteSharedPreferences(BuildConfig.PRODUCT_PREFERENCES_NAME)
        applicationContext.deleteSharedPreferences(BuildConfig.LEGACY_PREFERENCES_NAME)
    }

    private fun sessionsJson() = """
        {"sessions":[{"providerId":"grok","sessionId":"session-1","title":"Fixture session","projectDir":"/workspace","resident":true,"activity":"idle","createdAt":1,"lastActiveAt":2}]}
    """.trimIndent()

    private fun historyJson() = """
        {"session":{"providerId":"grok","sessionId":"session-1","title":"Fixture session","projectDir":"/workspace","resident":true,"createdAt":1,"lastActiveAt":2},"messages":[{"role":"assistant","content":"历史消息"}]}
    """.trimIndent()

    private fun filesJson() = """
        {"path":".","parent":null,"dirs":[],"files":[{"name":"README.md","path":"/workspace/README.md","rel":"README.md","size":123,"text":true}]}
    """.trimIndent()

    private fun updateJson(type: String, text: String, title: String? = null): String = buildJsonObject {
        put("providerId", "grok")
        put("sessionId", "session-1")
        put("update", buildJsonObject {
            put("sessionUpdate", type)
            title?.let { put("title", it) }
            put("content", buildJsonObject {
                put("type", "text")
                put("text", text)
            })
        })
    }.toString()

    private fun permissionJson(): String = buildJsonObject {
        put("providerId", "grok")
        put("sessionId", "session-1")
        put("question", "Run command?")
        put("options", kotlinx.serialization.json.buildJsonArray {
            add(buildJsonObject {
                put("optionId", "allow-once")
                put("name", "Allow once")
            })
            add(buildJsonObject {
                put("optionId", "deny")
                put("name", "Deny")
            })
        })
    }.toString()
}
