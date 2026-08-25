package com.anyaicliremote.core.model

/** Neutral phase of a request retry reported by the daemon. */
enum class RetryPhase { RETRYING, EXHAUSTED, FAILED }

/** Provider-neutral view of a request retry in progress. */
data class RetryStatus(
    val phase: RetryPhase,
    val attempt: Int = 0,
    val maxRetries: Int = 0,
    val reason: String = "",
    val rateLimit: Boolean = false,
)

/** Provider-neutral view of an automatic model switch. */
data class ModelSwitch(
    val previous: String = "",
    val current: String = "",
    val reason: String = "",
)

/**
 * A provider-neutral, transient session status update. Exactly one of [retry] or
 * [modelSwitch] is populated. The client renders it as a passing notice; it never
 * parses a provider's private wire.
 */
data class SessionStatusUpdate(
    val retry: RetryStatus? = null,
    val modelSwitch: ModelSwitch? = null,
)

fun retryPhase(raw: String?): RetryPhase = when (raw?.lowercase()) {
    "exhausted" -> RetryPhase.EXHAUSTED
    "failed" -> RetryPhase.FAILED
    else -> RetryPhase.RETRYING
}
