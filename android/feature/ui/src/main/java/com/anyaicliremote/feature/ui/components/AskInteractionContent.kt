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
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.mutableStateMapOf
import androidx.compose.runtime.remember
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import com.anyaicliremote.core.model.InteractionAnnotation
import com.anyaicliremote.core.model.InteractionAnswer
import com.anyaicliremote.core.model.InteractionQuestion
import com.anyaicliremote.core.model.PendingInteraction

@Composable
internal fun AskInteractionContent(
    interaction: PendingInteraction,
    actions: InteractionSheetActions,
) {
    // question index -> selected option labels; question index -> free-text note.
    val selections = remember(interaction.rpcId) { mutableStateMapOf<Int, MutableList<String>>() }
    val customAnswers = remember(interaction.rpcId) { mutableStateMapOf<Int, String>() }
    val scrollState = remember(interaction.rpcId) { ScrollState(initial = 0) }

    val draft = AskDraft(
        accepted = acceptedAnswers(interaction, selections, customAnswers),
        annotations = annotations(customAnswers),
        partial = partialAnswers(interaction, selections, customAnswers),
    )
    Column(
        modifier = Modifier
            .fillMaxWidth()
            .verticalScroll(scrollState),
    ) {
        Text("助手需要你的确认", style = MaterialTheme.typography.titleMedium, fontWeight = FontWeight.SemiBold)
        Spacer(Modifier.padding(top = 8.dp))
        interaction.questions.forEachIndexed { index, question ->
            QuestionBlock(
                question = question,
                selected = selections[index].orEmpty(),
                note = customAnswers[index].orEmpty(),
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
                onNote = { customAnswers[index] = it },
            )
        }
        AskActions(interaction, draft, actions.onAnswer)
    }
}

/** The operator's in-progress ask answer, computed from current selections + notes. */
private data class AskDraft(
    val accepted: Map<String, List<String>>,
    val annotations: Map<String, InteractionAnnotation>,
    val partial: Map<String, String>,
)

@Composable
private fun AskActions(
    interaction: PendingInteraction,
    draft: AskDraft,
    onAnswer: (PendingInteraction, InteractionAnswer) -> Unit,
) {
    // 先聊一下 / 跳过 only make sense while planning; cancel + submit are always offered.
    if (interaction.mode == "plan") {
        Row(Modifier.fillMaxWidth().padding(top = 12.dp), horizontalArrangement = Arrangement.Start) {
            TextButton(
                enabled = draft.partial.isNotEmpty(),
                onClick = { onAnswer(interaction, InteractionAnswer.ChatAbout(draft.partial)) },
            ) { Text("先聊一下") }
            Spacer(Modifier.padding(start = 8.dp))
            TextButton(onClick = { onAnswer(interaction, InteractionAnswer.SkipInterview(draft.partial)) }) {
                Text("跳过")
            }
        }
    }
    Row(Modifier.fillMaxWidth().padding(top = 8.dp), horizontalArrangement = Arrangement.End) {
        TextButton(onClick = { onAnswer(interaction, InteractionAnswer.CancelAsk) }) { Text("取消") }
        Spacer(Modifier.padding(start = 8.dp))
        Button(
            enabled = draft.accepted.isNotEmpty(),
            onClick = { onAnswer(interaction, InteractionAnswer.Accept(draft.accepted, draft.annotations)) },
        ) { Text("提交") }
    }
}

@Composable
private fun QuestionBlock(
    question: InteractionQuestion,
    selected: List<String>,
    note: String,
    onToggle: (String) -> Unit,
    onNote: (String) -> Unit,
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
                        text = option.description,
                        style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                        modifier = Modifier.padding(top = 2.dp),
                    )
                }
            }
        }
        OutlinedTextField(
            value = note,
            onValueChange = onNote,
            label = { Text("其他回答 / 备注") },
            modifier = Modifier.fillMaxWidth().padding(top = 8.dp),
        )
    }
}

// The free-text field doubles as an "Other" answer and as a per-question note.
private fun acceptedAnswers(
    interaction: PendingInteraction,
    selections: Map<Int, List<String>>,
    customAnswers: Map<Int, String>,
): Map<String, List<String>> = buildMap {
    interaction.questions.forEachIndexed { index, _ ->
        val labels = selections[index].orEmpty()
        val custom = customAnswers[index].orEmpty().trim()
        val answers = if (labels.isEmpty() && custom.isNotEmpty()) listOf("Other") else labels
        if (answers.isNotEmpty()) put(index.toString(), answers)
    }
}

private fun partialAnswers(
    interaction: PendingInteraction,
    selections: Map<Int, List<String>>,
    customAnswers: Map<Int, String>,
): Map<String, String> = buildMap {
    interaction.questions.forEachIndexed { index, _ ->
        val labels = selections[index].orEmpty()
        val custom = customAnswers[index].orEmpty().trim()
        if (labels.isNotEmpty() || custom.isNotEmpty()) {
            put(index.toString(), if (labels.isEmpty()) "Other" else labels.joinToString(", "))
        }
    }
}

private fun annotations(customAnswers: Map<Int, String>): Map<String, InteractionAnnotation> = buildMap {
    customAnswers.forEach { (index, answer) ->
        val trimmed = answer.trim()
        if (trimmed.isNotEmpty()) put(index.toString(), InteractionAnnotation(trimmed))
    }
}
