package com.anyaicliremote.feature.ui.components

import androidx.compose.foundation.background
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.BasicTextField
import androidx.compose.foundation.rememberScrollState
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Add
import androidx.compose.material.icons.filled.ArrowUpward
import androidx.compose.material.icons.filled.Stop
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.FilledIconButton
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.IconButtonDefaults
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.SolidColor
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import com.anyaicliremote.core.model.ChatBlock
import com.anyaicliremote.core.model.WorkspaceFile
import com.anyaicliremote.feature.ui.theme.AnyAIColors
import com.anyaicliremote.feature.ui.theme.AnyAIMetrics

internal data class ChatComposerState(
    val text: String,
    val busy: Boolean,
    val status: String,
    val attachments: List<WorkspaceFile> = emptyList(),
)

internal data class ChatComposerActions(
    val onTextChange: (String) -> Unit,
    val onSend: () -> Unit,
    val onStop: () -> Unit,
    val onOpenFilePicker: () -> Unit = {},
    val onRemoveAttachment: (String) -> Unit = {},
)

@Composable
internal fun ChatComposer(state: ChatComposerState, actions: ChatComposerActions) {
    Surface(color = MaterialTheme.colorScheme.background) {
        Column(Modifier.fillMaxWidth().padding(horizontal = AnyAIMetrics.composerHorizontalPadding.dp, vertical = AnyAIMetrics.composerVerticalPadding.dp)) {
            ComposerSurface(state, actions)
        }
    }
}

@Composable
private fun ComposerSurface(state: ChatComposerState, actions: ChatComposerActions) {
    Surface(
        color = AnyAIColors.composerSurface,
        shape = RoundedCornerShape(AnyAIMetrics.composerCornerRadius.dp),
        shadowElevation = AnyAIMetrics.composerShadowElevation.dp,
    ) {
        Column(Modifier.fillMaxWidth().padding(top = 10.dp, bottom = 8.dp)) {
            AttachmentRow(state.attachments, actions.onRemoveAttachment)
            ComposerInput(state, actions)
            ComposerActions(state, actions)
        }
    }
}

@Composable
private fun AttachmentRow(attachments: List<WorkspaceFile>, onRemove: (String) -> Unit) {
    if (attachments.isNotEmpty()) {
        Row(
            Modifier.fillMaxWidth()
                .horizontalScroll(rememberScrollState())
                .padding(horizontal = 10.dp, vertical = 3.dp),
        ) {
            attachments.forEach { file ->
                FileCard(
                    file = file,
                    modifier = Modifier.width(AnyAIMetrics.attachmentWidth.dp).padding(end = 6.dp),
                    onRemove = { onRemove(file.path) },
                )
            }
        }
    }
}

@Composable
private fun ComposerInput(state: ChatComposerState, actions: ChatComposerActions) {
    BasicTextField(
        value = state.text,
        onValueChange = actions.onTextChange,
        minLines = 1,
        maxLines = AnyAIMetrics.messageMaxLines,
        textStyle = MaterialTheme.typography.bodyLarge.copy(color = MaterialTheme.colorScheme.onSurface),
        cursorBrush = SolidColor(MaterialTheme.colorScheme.onSurface),
        modifier = Modifier.fillMaxWidth().heightIn(min = 34.dp, max = AnyAIMetrics.messageMaxHeight.dp),
        decorationBox = { innerTextField ->
            Box(Modifier.fillMaxWidth().padding(horizontal = 14.dp, vertical = 5.dp), contentAlignment = Alignment.CenterStart) {
                if (state.text.isBlank()) {
                    Text(
                        "给助手发送消息",
                        style = MaterialTheme.typography.bodyLarge,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                }
                innerTextField()
            }
        },
    )
}

@Composable
private fun AttachmentButton(onClick: () -> Unit) {
    IconButton(onClick = onClick, modifier = Modifier.size(AnyAIMetrics.sendButtonSize.dp)) {
        Icon(
            Icons.Default.Add,
            "添加附件",
            tint = MaterialTheme.colorScheme.onSurfaceVariant,
            modifier = Modifier.size(AnyAIMetrics.smallIconSize.dp),
        )
    }
}

private fun shouldShowStatus(state: ChatComposerState): Boolean {
    val hasErrorStatus = state.status.contains("失败") || state.status.contains("重连")
    return (state.busy || hasErrorStatus) && state.status.isNotBlank()
}

@Composable
private fun StatusLabel(status: String) {
    Text(
        status,
        Modifier.padding(start = 10.dp),
        style = MaterialTheme.typography.labelSmall,
        color = MaterialTheme.colorScheme.onSurfaceVariant,
        maxLines = 1,
        overflow = TextOverflow.Ellipsis,
    )
}

@Composable
private fun SendButton(state: ChatComposerState, actions: ChatComposerActions) {
    FilledIconButton(
        onClick = if (state.busy) actions.onStop else actions.onSend,
        enabled = state.busy || state.text.isNotBlank() || state.attachments.isNotEmpty(),
        colors = IconButtonDefaults.filledIconButtonColors(
            containerColor = MaterialTheme.colorScheme.onSurface,
            contentColor = MaterialTheme.colorScheme.background,
            disabledContainerColor = AnyAIColors.disabledContainer,
            disabledContentColor = AnyAIColors.disabledContent,
        ),
        modifier = Modifier.size(AnyAIMetrics.sendButtonSize.dp),
    ) {
        Icon(
            if (state.busy) Icons.Default.Stop else Icons.Default.ArrowUpward,
            if (state.busy) "停止" else "发送",
            modifier = Modifier.size(AnyAIMetrics.smallIconSize.dp),
        )
    }
}

@Composable
private fun ComposerActions(state: ChatComposerState, actions: ChatComposerActions) {
    Row(
        Modifier.fillMaxWidth().padding(horizontal = 9.dp, vertical = 2.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        AttachmentButton(actions.onOpenFilePicker)
        if (shouldShowStatus(state)) {
            StatusLabel(state.status)
        }
        Spacer(Modifier.weight(1f))
        SendButton(state, actions)
    }
}

@Composable
internal fun FloatingToolStatus(block: ChatBlock, onStop: () -> Unit) {
    Row(
        Modifier.fillMaxWidth().background(MaterialTheme.colorScheme.surfaceVariant).padding(horizontal = 14.dp, vertical = 8.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        CircularProgressIndicator(Modifier.size(16.dp), strokeWidth = 2.dp)
        Spacer(Modifier.width(9.dp))
        Icon(toolIcon(block.title), null, tint = MaterialTheme.colorScheme.primary, modifier = Modifier.size(18.dp))
        Spacer(Modifier.width(7.dp))
        Text(block.title, maxLines = 1, overflow = TextOverflow.Ellipsis, modifier = Modifier.weight(1f), style = MaterialTheme.typography.bodySmall)
        IconButton(
            onClick = onStop,
            modifier = Modifier.size(AnyAIMetrics.sendButtonSize.dp),
        ) {
            Icon(Icons.Default.Stop, "停止工具", tint = AnyAIColors.error)
        }
    }
}
