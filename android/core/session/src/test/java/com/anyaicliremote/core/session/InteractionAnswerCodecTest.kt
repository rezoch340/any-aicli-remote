package com.anyaicliremote.core.session

import com.anyaicliremote.core.model.InteractionAnswer
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.jsonArray
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

class InteractionAnswerCodecTest {
    @Test
    fun acceptEncodesAnswersAsObjectKeyedByIndex() {
        val result = InteractionAnswerCodec.toResult(
            InteractionAnswer.Accept(mapOf("0" to listOf("进程内 LRU"))),
        )
        assertEquals("accepted", result.stringOf("outcome"))
        // The daemon rejects an array; answers must be a JSON object.
        val answers = result["answers"] as JsonObject
        assertEquals(1, answers["0"]!!.jsonArray.size)
    }

    @Test
    fun approveHasNoFeedback() {
        val result = InteractionAnswerCodec.toResult(InteractionAnswer.Approve)
        assertEquals("approved", result.stringOf("outcome"))
        assertNull(result["feedback"])
    }

    @Test
    fun cancelOmitsBlankFeedbackButKeepsRealFeedback() {
        val blank = InteractionAnswerCodec.toResult(InteractionAnswer.Cancel("  "))
        assertNull(blank["feedback"])
        val real = InteractionAnswerCodec.toResult(InteractionAnswer.Cancel("加错误处理"))
        assertEquals("加错误处理", real.stringOf("feedback"))
    }

    @Test
    fun abandonEncodesOutcomeOnly() {
        val result = InteractionAnswerCodec.toResult(InteractionAnswer.Abandon)
        assertEquals("abandoned", result.stringOf("outcome"))
    }

    @Test
    fun partialOutcomesCarryPartialAnswers() {
        val chat = InteractionAnswerCodec.toResult(InteractionAnswer.ChatAbout(mapOf("0" to "看情况")))
        assertEquals("chat_about_this", chat.stringOf("outcome"))
        assertTrue(chat["partialAnswers"] is JsonObject)
        val skip = InteractionAnswerCodec.toResult(InteractionAnswer.SkipInterview(emptyMap()))
        assertEquals("skip_interview", skip.stringOf("outcome"))
    }

    private fun JsonObject.stringOf(key: String): String =
        (this[key] as JsonPrimitive).content
}
