package com.anyaicliremote.core.model

/** Lifecycle status of a child agent, mirroring the daemon's neutral contract. */
enum class ChildAgentStatus { RUNNING, COMPLETED, FAILED, CANCELLED, UNKNOWN }

/**
 * Provider-neutral child-agent card. The daemon supplies structured metadata
 * only; prompts and generated output are never carried here. Cards are keyed by
 * [providerChildId] and updated in place as ordered events arrive.
 */
data class ChildAgentCard(
    val providerChildId: String,
    val childSessionId: String = "",
    val agentType: String = "",
    val description: String = "",
    val status: ChildAgentStatus = ChildAgentStatus.RUNNING,
    val startedAt: Long = 0,
    val completedAt: Long = 0,
    val toolCallCount: Int = 0,
    val turnCount: Int = 0,
    val modelId: String = "",
    val tokensUsed: Long = 0,
    val contextUsagePercent: Double = 0.0,
    val sequence: Long? = null,
)

fun childAgentStatus(raw: String?): ChildAgentStatus = when (raw?.lowercase()) {
    "running" -> ChildAgentStatus.RUNNING
    "completed" -> ChildAgentStatus.COMPLETED
    "failed" -> ChildAgentStatus.FAILED
    "cancelled" -> ChildAgentStatus.CANCELLED
    else -> ChildAgentStatus.UNKNOWN
}
