package com.anyaicliremote.core.chat

import com.anyaicliremote.core.model.ChildAgentCard

/**
 * Merges ordered child-agent updates into a stable card list. Cards are keyed by
 * providerChildId and kept in first-seen order. Out-of-order or replayed events
 * are dropped when a card already holds a newer sequence, so late-arriving stale
 * updates cannot regress a card's state.
 */
object ChildAgentReducer {
    fun apply(cards: List<ChildAgentCard>, incoming: ChildAgentCard): List<ChildAgentCard> {
        val index = cards.indexOfFirst { it.providerChildId == incoming.providerChildId }
        if (index < 0) {
            return cards + incoming
        }
        val existing = cards[index]
        if (isStale(existing.sequence, incoming.sequence)) {
            return cards
        }
        return cards.toMutableList().also { it[index] = merge(existing, incoming) }
    }

    fun replace(cards: List<ChildAgentCard>): List<ChildAgentCard> = cards

    private fun isStale(existingSequence: Long?, incomingSequence: Long?): Boolean {
        if (existingSequence == null || incomingSequence == null) return false
        return incomingSequence < existingSequence
    }

    // Later events carry the freshest status and metrics; empty fields in an
    // update do not erase values an earlier event established.
    private fun merge(existing: ChildAgentCard, incoming: ChildAgentCard): ChildAgentCard =
        incoming.copy(
            childSessionId = incoming.childSessionId.ifEmpty { existing.childSessionId },
            agentType = incoming.agentType.ifEmpty { existing.agentType },
            description = incoming.description.ifEmpty { existing.description },
            modelId = incoming.modelId.ifEmpty { existing.modelId },
            startedAt = if (incoming.startedAt != 0L) incoming.startedAt else existing.startedAt,
            completedAt = if (incoming.completedAt != 0L) incoming.completedAt else existing.completedAt,
            toolCallCount = maxOf(incoming.toolCallCount, existing.toolCallCount),
            turnCount = maxOf(incoming.turnCount, existing.turnCount),
            tokensUsed = maxOf(incoming.tokensUsed, existing.tokensUsed),
            sequence = incoming.sequence ?: existing.sequence,
        )
}
