package com.anyaicliremote.core.session

import com.anyaicliremote.core.model.InteractionAnnotation
import com.anyaicliremote.core.model.InteractionAnswer
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.put

/**
 * Encodes a typed [InteractionAnswer] into the daemon's neutral interaction
 * response. The daemon denormalizes it into the provider shape; the client keeps
 * no provider wire knowledge here.
 */
object InteractionAnswerCodec {
    fun toResult(answer: InteractionAnswer): JsonObject = when (answer) {
        is InteractionAnswer.Accept -> buildJsonObject {
            put("outcome", "accepted")
            put("answers", answersObject(answer.answers))
            val annotations = annotationsObject(answer.annotations)
            if (annotations.isNotEmpty()) put("annotations", annotations)
        }
        is InteractionAnswer.ChatAbout -> partial("chat_about_this", answer.partialAnswers)
        is InteractionAnswer.SkipInterview -> partial("skip_interview", answer.partialAnswers)
        InteractionAnswer.CancelAsk -> buildJsonObject { put("outcome", "cancelled") }
        InteractionAnswer.Approve -> buildJsonObject { put("outcome", "approved") }
        is InteractionAnswer.Cancel -> buildJsonObject {
            put("outcome", "cancelled")
            if (answer.feedback.isNotBlank()) put("feedback", answer.feedback)
        }
        InteractionAnswer.Abandon -> buildJsonObject { put("outcome", "abandoned") }
    }

    private fun answersObject(answers: Map<String, List<String>>): JsonObject = buildJsonObject {
        answers.forEach { (index, labels) ->
            put(index, JsonArray(labels.map(::JsonPrimitive)))
        }
    }

    // Per-question notes, keyed by the same decimal question index as answers.
    private fun annotationsObject(annotations: Map<String, InteractionAnnotation>): JsonObject = buildJsonObject {
        annotations.forEach { (index, annotation) ->
            val notes = annotation.notes.trim()
            if (notes.isNotEmpty()) put(index, buildJsonObject { put("notes", notes) })
        }
    }

    private fun partial(outcome: String, partialAnswers: Map<String, String>): JsonObject = buildJsonObject {
        put("outcome", outcome)
        put("partialAnswers", buildJsonObject { partialAnswers.forEach { (index, text) -> put(index, text) } })
    }
}
