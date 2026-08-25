package com.anyaicliremote.core.model

/** Whether a pending interaction is a question or a plan-approval request. */
enum class InteractionKind { ASK_QUESTION, EXIT_PLAN }

/** One selectable choice for an interaction question. */
data class InteractionOption(
    val label: String,
    val description: String = "",
)

/** One question with its choices. */
data class InteractionQuestion(
    val question: String,
    val options: List<InteractionOption> = emptyList(),
    val multiSelect: Boolean = false,
)

/** Free-text operator note attached to one answered question. */
data class InteractionAnnotation(val notes: String)

/**
 * A provider-neutral interaction the agent is blocking on. The client renders it
 * and sends a neutral answer keyed by [rpcId]; it never parses a provider wire.
 * For an ask, [questions] is populated. For an exit-plan, [planContent] is the
 * plan text (may be empty).
 */
data class PendingInteraction(
    val rpcId: Long,
    val kind: InteractionKind,
    val sessionIdentity: SessionIdentity,
    val toolCallId: String,
    val questions: List<InteractionQuestion> = emptyList(),
    val planContent: String = "",
    val mode: String = "",
)

/**
 * The operator's neutral answer to a [PendingInteraction]. It is encoded to the
 * daemon's neutral response shape; the client never builds a provider payload.
 */
sealed interface InteractionAnswer {
    /** Ask answer: question index -> selected option labels, plus optional per-question notes. */
    data class Accept(
        val answers: Map<String, List<String>>,
        val annotations: Map<String, InteractionAnnotation> = emptyMap(),
    ) : InteractionAnswer
    data class ChatAbout(val partialAnswers: Map<String, String>) : InteractionAnswer
    data class SkipInterview(val partialAnswers: Map<String, String>) : InteractionAnswer

    /** Dismiss an ask without answering. */
    data object CancelAsk : InteractionAnswer

    /** Exit-plan answers. */
    data object Approve : InteractionAnswer
    data class Cancel(val feedback: String = "") : InteractionAnswer
    data object Abandon : InteractionAnswer
}
