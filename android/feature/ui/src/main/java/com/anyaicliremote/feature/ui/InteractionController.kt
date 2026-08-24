package com.anyaicliremote.feature.ui

import com.anyaicliremote.core.model.InteractionAnswer
import com.anyaicliremote.core.model.PendingInteraction
import com.anyaicliremote.core.session.SessionController

/**
 * Owns answering a pending structured interaction. It sends the operator's
 * neutral answer through the session controller and clears the pending state; it
 * holds no provider-wire knowledge.
 */
internal class InteractionController(
    private val scope: ChatOperationScope,
    private val sessionController: SessionController,
) {
    fun answer(interaction: PendingInteraction, answer: InteractionAnswer) {
        if (scope.state.pendingInteraction?.rpcId != interaction.rpcId) return
        sessionController.answerInteraction(interaction.rpcId, answer)
        scope.update { it.copy(pendingInteraction = null) }
    }

    /** Dismiss without answering; the daemon re-raises the interaction on reconnect. */
    fun dismiss(interaction: PendingInteraction) {
        if (scope.state.pendingInteraction?.rpcId != interaction.rpcId) return
        scope.update { it.copy(pendingInteraction = null) }
    }
}
