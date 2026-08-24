package com.anyaicliremote.feature.ui.components

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.ScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.Button
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import com.anyaicliremote.core.model.InteractionAnswer
import com.anyaicliremote.core.model.PendingInteraction

@Composable
internal fun PlanApprovalContent(
    interaction: PendingInteraction,
    actions: InteractionSheetActions,
) {
    var feedback by remember(interaction.rpcId) { mutableStateOf("") }
    val scrollState = remember(interaction.rpcId) { ScrollState(0) }
    Column(
        modifier = Modifier
            .fillMaxWidth()
            .verticalScroll(scrollState),
    ) {
        Text(
            text = "计划待批准",
            style = MaterialTheme.typography.titleMedium,
            fontWeight = FontWeight.SemiBold,
        )
        Spacer(Modifier.padding(top = 8.dp))
        Text(
            text = interaction.planContent.ifBlank { "（尚未写出计划）" },
            style = MaterialTheme.typography.bodyMedium,
            modifier = Modifier
                .fillMaxWidth()
                .background(MaterialTheme.colorScheme.surfaceVariant, RoundedCornerShape(10.dp))
                .padding(12.dp),
        )
        OutlinedTextField(
            value = feedback,
            onValueChange = { feedback = it },
            label = { Text("修改意见（可选，用于请求修改）") },
            modifier = Modifier
                .fillMaxWidth()
                .padding(top = 12.dp),
        )
        Row(
            modifier = Modifier
                .fillMaxWidth()
                .padding(top = 12.dp),
            horizontalArrangement = Arrangement.End,
        ) {
            TextButton(onClick = { actions.onAnswer(interaction, InteractionAnswer.Cancel(feedback)) }) {
                Text(if (feedback.isBlank()) "取消" else "请求修改")
            }
            Spacer(Modifier.padding(start = 8.dp))
            Button(onClick = { actions.onAnswer(interaction, InteractionAnswer.Approve) }) {
                Text("批准并开始")
            }
        }
    }
}
