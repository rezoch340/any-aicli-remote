package com.anyaicliremote.feature.ui.components

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.Button
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.ModalBottomSheet
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateMapOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import com.anyaicliremote.core.model.InteractionAnswer
import com.anyaicliremote.core.model.InteractionKind
import com.anyaicliremote.core.model.InteractionQuestion
import com.anyaicliremote.core.model.PendingInteraction

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun InteractionSheet(interaction: PendingInteraction?, actions: InteractionSheetActions) {
    if (interaction == null) return
    ModalBottomSheet(onDismissRequest = { actions.onDismiss(interaction) }) {
        Column(
            Modifier
                .fillMaxWidth()
                .heightIn(max = 560.dp)
                .verticalScroll(rememberScrollState())
                .padding(horizontal = 20.dp, vertical = 8.dp),
        ) {
            when (interaction.kind) {
                InteractionKind.ASK_QUESTION -> AskForm(interaction, actions)
                InteractionKind.EXIT_PLAN -> PlanApproval(interaction, actions)
            }
            Spacer(Modifier.padding(bottom = 16.dp))
        }
    }
}

/** Actions a pending interaction can produce. */
data class InteractionSheetActions(
    val onAnswer: (PendingInteraction, InteractionAnswer) -> Unit,
    val onDismiss: (PendingInteraction) -> Unit,
)

@Composable
private fun AskForm(interaction: PendingInteraction, actions: InteractionSheetActions) {
    // question index -> selected labels
    val selections = remember(interaction.rpcId) { mutableStateMapOf<Int, MutableList<String>>() }
    Text("助手需要你的确认", style = MaterialTheme.typography.titleMedium, fontWeight = FontWeight.SemiBold)
    Spacer(Modifier.padding(top = 8.dp))
    interaction.questions.forEachIndexed { index, question ->
        QuestionBlock(
            question = question,
            selected = selections[index].orEmpty(),
            onToggle = { label ->
                val current = selections.getOrPut(index) { mutableListOf() }
                if (question.multiSelect) {
                    if (!current.remove(label)) current.add(label)
                } else {
                    current.clear()
                    current.add(label)
                }
                selections[index] = current.toMutableList()
            },
        )
    }
    Row(Modifier.fillMaxWidth().padding(top = 12.dp), horizontalArrangement = Arrangement.End) {
        TextButton(onClick = {
            actions.onAnswer(interaction, InteractionAnswer.SkipInterview(emptyMap()))
        }) { Text("跳过") }
        Spacer(Modifier.padding(start = 8.dp))
        Button(
            enabled = selections.any { it.value.isNotEmpty() },
            onClick = {
                val answers = selections
                    .filterValues { it.isNotEmpty() }
                    .mapKeys { it.key.toString() }
                    .mapValues { it.value.toList() }
                actions.onAnswer(interaction, InteractionAnswer.Accept(answers))
            },
        ) { Text("提交") }
    }
}

@Composable
private fun QuestionBlock(
    question: InteractionQuestion,
    selected: List<String>,
    onToggle: (String) -> Unit,
) {
    Column(Modifier.fillMaxWidth().padding(top = 12.dp)) {
        Text(question.question, style = MaterialTheme.typography.bodyLarge, fontWeight = FontWeight.Medium)
        question.options.forEach { option ->
            val isSelected = option.label in selected
            Column(
                Modifier
                    .fillMaxWidth()
                    .padding(top = 8.dp)
                    .background(
                        if (isSelected) MaterialTheme.colorScheme.primaryContainer else Color.Transparent,
                        RoundedCornerShape(10.dp),
                    )
                    .border(1.dp, MaterialTheme.colorScheme.outline, RoundedCornerShape(10.dp))
                    .clickable { onToggle(option.label) }
                    .padding(12.dp),
            ) {
                Text(option.label, style = MaterialTheme.typography.bodyMedium, fontWeight = FontWeight.Medium)
                if (option.description.isNotEmpty()) {
                    Text(
                        option.description,
                        style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                        modifier = Modifier.padding(top = 2.dp),
                    )
                }
            }
        }
    }
}

@Composable
private fun PlanApproval(interaction: PendingInteraction, actions: InteractionSheetActions) {
    var feedback by remember(interaction.rpcId) { mutableStateOf("") }
    Text("计划待批准", style = MaterialTheme.typography.titleMedium, fontWeight = FontWeight.SemiBold)
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
        modifier = Modifier.fillMaxWidth().padding(top = 12.dp),
    )
    Row(Modifier.fillMaxWidth().padding(top = 12.dp), horizontalArrangement = Arrangement.End) {
        TextButton(onClick = {
            actions.onAnswer(interaction, InteractionAnswer.Cancel(feedback))
        }) { Text(if (feedback.isBlank()) "取消" else "请求修改") }
        Spacer(Modifier.padding(start = 8.dp))
        Button(onClick = {
            actions.onAnswer(interaction, InteractionAnswer.Approve)
        }) { Text("批准并开始") }
    }
}
