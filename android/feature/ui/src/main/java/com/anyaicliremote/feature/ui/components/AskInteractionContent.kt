package com.anyaicliremote.feature.ui.components

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.ScrollState
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.Button
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.mutableStateMapOf
import androidx.compose.runtime.remember
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import com.anyaicliremote.core.model.InteractionAnswer
import com.anyaicliremote.core.model.InteractionQuestion
import com.anyaicliremote.core.model.PendingInteraction

@Composable
internal fun AskInteractionContent(
    interaction: PendingInteraction,
    actions: InteractionSheetActions,
) {
    val selections = remember(interaction.rpcId) { mutableStateMapOf<Int, MutableList<String>>() }
    val scrollState = remember(interaction.rpcId) { ScrollState(initial = 0) }
    Column(
        modifier = Modifier
            .fillMaxWidth()
            .verticalScroll(scrollState),
    ) {
        Text("助手需要你的确认", style = MaterialTheme.typography.titleMedium, fontWeight = FontWeight.SemiBold)
        Spacer(Modifier.padding(top = 8.dp))
        interaction.questions.forEachIndexed { index, question ->
            QuestionBlock(question, selections[index].orEmpty()) { label ->
                val current = selections.getOrPut(index) { mutableListOf() }
                if (question.multiSelect) {
                    if (!current.remove(label)) current.add(label)
                } else {
                    current.clear()
                    current.add(label)
                }
                selections[index] = current.toMutableList()
            }
        }
        Row(
            modifier = Modifier
                .fillMaxWidth()
                .padding(top = 12.dp),
            horizontalArrangement = Arrangement.End,
        ) {
            TextButton(onClick = { actions.onAnswer(interaction, InteractionAnswer.SkipInterview(emptyMap())) }) {
                Text("跳过")
            }
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
            ) {
                Text("提交")
            }
        }
    }
}

@Composable
private fun QuestionBlock(
    question: InteractionQuestion,
    selected: List<String>,
    onToggle: (String) -> Unit,
) {
    Column(
        modifier = Modifier
            .fillMaxWidth()
            .padding(top = 12.dp),
    ) {
        Text(question.question, style = MaterialTheme.typography.bodyLarge, fontWeight = FontWeight.Medium)
        question.options.forEach { option ->
            val isSelected = option.label in selected
            Column(
                modifier = Modifier
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
                        text = option.description,
                        style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                        modifier = Modifier.padding(top = 2.dp),
                    )
                }
            }
        }
    }
}
