package com.anyaicliremote.feature.ui.components

import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.ModalBottomSheet
import androidx.compose.material3.rememberModalBottomSheetState
import androidx.compose.runtime.Composable
import androidx.compose.runtime.key
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp
import com.anyaicliremote.core.model.InteractionAnswer
import com.anyaicliremote.core.model.InteractionKind
import com.anyaicliremote.core.model.PendingInteraction

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun InteractionSheet(interaction: PendingInteraction?, actions: InteractionSheetActions) {
    if (interaction == null) return
    val isPlan = interaction.kind == InteractionKind.EXIT_PLAN
    key(interaction.rpcId) {
        val sheetState = rememberModalBottomSheetState(skipPartiallyExpanded = isPlan)
        ModalBottomSheet(
            onDismissRequest = { actions.onDismiss(interaction) },
            sheetState = sheetState,
        ) {
            val contentModifier = if (isPlan) {
                Modifier.fillMaxHeight()
            } else {
                Modifier.heightIn(max = 560.dp)
            }
            Column(
                modifier = Modifier
                    .fillMaxWidth()
                    .then(contentModifier)
                    .padding(horizontal = 20.dp, vertical = 8.dp),
            ) {
                when (interaction.kind) {
                    InteractionKind.ASK_QUESTION -> AskInteractionContent(interaction, actions)
                    InteractionKind.EXIT_PLAN -> PlanApprovalContent(interaction, actions)
                }
            }
        }
    }
}

data class InteractionSheetActions(
    val onAnswer: (PendingInteraction, InteractionAnswer) -> Unit,
    val onDismiss: (PendingInteraction) -> Unit,
)
