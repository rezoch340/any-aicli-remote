package com.anyaicliremote.app.ui.components

import androidx.compose.animation.AnimatedVisibility
import androidx.compose.foundation.ExperimentalFoundationApi
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.combinedClickable
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ExperimentalLayoutApi
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.AutoAwesome
import androidx.compose.material.icons.filled.Cancel
import androidx.compose.material.icons.filled.CheckCircle
import androidx.compose.material.icons.filled.Description
import androidx.compose.material.icons.filled.Error
import androidx.compose.material.icons.filled.Language
import androidx.compose.material.icons.filled.Lightbulb
import androidx.compose.material.icons.filled.Settings
import androidx.compose.material.icons.filled.Terminal
import androidx.compose.material.icons.filled.Warning
import androidx.compose.material3.Button
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.ModalBottomSheet
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberUpdatedState
import androidx.compose.runtime.setValue
import androidx.compose.runtime.snapshotFlow
import androidx.compose.runtime.withFrameNanos
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.platform.LocalClipboardManager
import androidx.compose.ui.text.AnnotatedString
import androidx.compose.ui.text.SpanStyle
import androidx.compose.ui.text.TextLinkStyles
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontStyle
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.mikepenz.markdown.compose.components.markdownComponents
import com.mikepenz.markdown.compose.elements.MarkdownTable
import com.mikepenz.markdown.m3.Markdown
import com.mikepenz.markdown.m3.markdownColor
import com.mikepenz.markdown.m3.markdownTypography
import com.mikepenz.markdown.model.markdownAnimations
import com.mikepenz.markdown.model.rememberMarkdownState
import com.anyaicliremote.app.model.ChatBlock
import com.anyaicliremote.app.model.ChatBlockKind
import com.anyaicliremote.app.model.ToolRunState
import com.anyaicliremote.app.ui.ChatViewModel
import kotlinx.coroutines.flow.conflate
import kotlinx.coroutines.flow.distinctUntilChanged

@Composable
internal fun ChatBlockItem(
    block: ChatBlock,
    viewModel: ChatViewModel,
    isStreaming: Boolean = false,
) {
    when (block.kind) {
        ChatBlockKind.USER -> UserMessage(block)
        ChatBlockKind.ASSISTANT -> {
            AssistantMessageHeader()
            AssistantMarkdownFragment(block.text, isStreaming = isStreaming)
        }
        ChatBlockKind.THINKING -> ThinkingMessage(block)
        ChatBlockKind.TOOL -> ToolMessage(block)
        ChatBlockKind.PERMISSION -> PermissionMessage(block, viewModel)
        ChatBlockKind.PLAN -> PlanMessage(block)
        ChatBlockKind.SYSTEM -> Text(block.text, style = MaterialTheme.typography.labelSmall, color = MaterialTheme.colorScheme.onSurfaceVariant, modifier = Modifier.fillMaxWidth().padding(12.dp))
    }
}

@OptIn(ExperimentalFoundationApi::class)
@Composable
private fun UserMessage(block: ChatBlock) {
    val clipboard = LocalClipboardManager.current
    var menu by remember { mutableStateOf(false) }
    Row(Modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 5.dp), horizontalArrangement = Arrangement.End) {
        Box {
            Column(
                modifier = Modifier.widthIn(max = 304.dp).clip(RoundedCornerShape(18.dp))
                    .background(Color(0xFF1D1E22))
                    .combinedClickable(onClick = { }, onLongClick = { menu = true })
                    .padding(horizontal = 14.dp, vertical = 10.dp),
            ) {
                if (block.text.isNotEmpty()) {
                    Text(
                        block.text,
                        style = MaterialTheme.typography.bodyLarge,
                    )
                }
                block.attachments.forEach { file ->
                    FileCard(
                        file = file,
                        modifier = Modifier.padding(top = 6.dp),
                    )
                }
            }
            DropdownMenu(expanded = menu, onDismissRequest = { menu = false }) {
                DropdownMenuItem(text = { Text("复制") }, onClick = { clipboard.setText(AnnotatedString(block.text)); menu = false })
            }
        }
    }
}

@Composable
internal fun AssistantMessageHeader() {
    Row(
        Modifier.fillMaxWidth().padding(start = 16.dp, end = 16.dp, top = 9.dp, bottom = 2.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Icon(Icons.Default.AutoAwesome, null, tint = Color(0xFFB8B49F), modifier = Modifier.size(16.dp))
        Spacer(Modifier.width(6.dp))
        Text("助手", style = MaterialTheme.typography.titleSmall, fontWeight = FontWeight.SemiBold)
    }
}

@Composable
internal fun AssistantMarkdownFragment(text: String, isStreaming: Boolean) {
    val body = MaterialTheme.typography.bodyLarge.copy(color = MaterialTheme.colorScheme.onSurface)
    val code = MaterialTheme.typography.bodySmall.copy(
        color = Color(0xFFD6D7DA),
        fontFamily = FontFamily.Monospace,
        fontSize = 12.5.sp,
        lineHeight = 18.sp,
    )
    val inlineCode = body.copy(
        color = Color(0xFFD6D7DA),
        fontFamily = FontFamily.Monospace,
        fontSize = 13.5.sp,
    )
    val markdownType = markdownTypography(
        h1 = MaterialTheme.typography.titleLarge.copy(fontSize = 21.sp, lineHeight = 27.sp),
        h2 = MaterialTheme.typography.titleMedium.copy(fontSize = 18.sp, lineHeight = 24.sp),
        h3 = MaterialTheme.typography.titleSmall.copy(fontSize = 16.sp, lineHeight = 22.sp),
        h4 = body.copy(fontWeight = FontWeight.SemiBold),
        h5 = body.copy(fontWeight = FontWeight.SemiBold),
        h6 = body.copy(fontWeight = FontWeight.SemiBold),
        text = body,
        code = code,
        inlineCode = inlineCode,
        quote = body.copy(color = MaterialTheme.colorScheme.onSurfaceVariant, fontStyle = FontStyle.Italic),
        paragraph = body,
        ordered = body,
        bullet = body,
        list = body,
        textLink = TextLinkStyles(style = SpanStyle(color = Color(0xFFB8C7EA))),
        table = MaterialTheme.typography.bodySmall.copy(color = MaterialTheme.colorScheme.onSurface),
    )
    val markdownColors = markdownColor(
        text = MaterialTheme.colorScheme.onSurface,
        codeBackground = Color(0xFF141519),
        inlineCodeBackground = Color(0xFF202126),
        dividerColor = MaterialTheme.colorScheme.outline,
        tableBackground = Color(0xFF121317),
    )
    val markdownComponents = markdownComponents(
        table = { model ->
            Box(Modifier.fillMaxWidth().horizontalScroll(rememberScrollState())) {
                MarkdownTable(
                    content = model.content,
                    node = model.node,
                    style = model.typography.table,
                )
            }
        },
    )

    val latestText by rememberUpdatedState(text)
    var displayedText by remember { mutableStateOf(text) }
    LaunchedEffect(isStreaming) {
        if (!isStreaming) {
            displayedText = latestText
            return@LaunchedEffect
        }
        snapshotFlow { latestText }
            .distinctUntilChanged()
            .conflate()
            .collect { nextText ->
                withFrameNanos { }
                displayedText = nextText
            }
    }
    val renderedText = if (isStreaming) displayedText else text
    val markdownState = rememberMarkdownState(
        content = renderedText,
        retainState = true,
    )

    Column(Modifier.fillMaxWidth().padding(start = 16.dp, end = 16.dp, top = 2.dp, bottom = 7.dp)) {
        Markdown(
            markdownState = markdownState,
            colors = markdownColors,
            typography = markdownType,
            components = markdownComponents,
            modifier = Modifier.fillMaxWidth(),
            animations = markdownAnimations(animateTextSize = { this }),
        )
    }
}

@Composable
private fun ThinkingMessage(block: ChatBlock) {
    var expanded by remember { mutableStateOf(false) }
    Card(
        colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surfaceVariant.copy(alpha = 0.55f)),
        modifier = Modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 4.dp).clickable { expanded = !expanded },
    ) {
        Column(Modifier.padding(12.dp)) {
            Row(verticalAlignment = Alignment.CenterVertically) {
                Icon(Icons.Default.Lightbulb, null, modifier = Modifier.size(18.dp), tint = MaterialTheme.colorScheme.onSurfaceVariant)
                Spacer(Modifier.width(7.dp))
                Text("思考过程", style = MaterialTheme.typography.labelMedium, fontWeight = FontWeight.SemiBold)
            }
            AnimatedVisibility(expanded) {
                Text(block.text, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant, modifier = Modifier.padding(top = 9.dp))
            }
        }
    }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun ToolMessage(block: ChatBlock) {
    var detail by remember { mutableStateOf(false) }
    val color = when (block.toolState) {
        ToolRunState.SUCCESS -> Color(0xFF4ADE80)
        ToolRunState.FAILED -> Color(0xFFFF6B6B)
        ToolRunState.CANCELLED -> Color(0xFFFFB74D)
        else -> MaterialTheme.colorScheme.primary
    }
    Row(
        Modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 3.dp).height(38.dp)
            .clip(RoundedCornerShape(19.dp)).background(MaterialTheme.colorScheme.surfaceVariant.copy(alpha = 0.6f))
            .clickable { detail = true }.padding(horizontal = 12.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Icon(toolIcon(block.title), null, tint = color, modifier = Modifier.size(16.dp))
        Spacer(Modifier.width(8.dp))
        Text(block.title, modifier = Modifier.weight(1f), maxLines = 1, overflow = TextOverflow.Ellipsis, style = MaterialTheme.typography.bodySmall, fontWeight = FontWeight.Medium)
        when (block.toolState) {
            ToolRunState.PENDING, ToolRunState.RUNNING -> CircularProgressIndicator(Modifier.size(15.dp), strokeWidth = 2.dp, color = color)
            ToolRunState.SUCCESS -> Icon(Icons.Default.CheckCircle, null, tint = color, modifier = Modifier.size(17.dp))
            ToolRunState.FAILED -> Icon(Icons.Default.Error, null, tint = color, modifier = Modifier.size(17.dp))
            ToolRunState.CANCELLED -> Icon(Icons.Default.Cancel, null, tint = color, modifier = Modifier.size(17.dp))
        }
    }
    if (detail) {
        ModalBottomSheet(onDismissRequest = { detail = false }) {
            Column(Modifier.fillMaxWidth().padding(20.dp)) {
                Text(block.title, style = MaterialTheme.typography.titleMedium, fontWeight = FontWeight.Bold)
                Spacer(Modifier.height(12.dp))
                Text(block.detail.ifEmpty { "暂无工具输出" }, fontFamily = FontFamily.Monospace, style = MaterialTheme.typography.bodySmall)
                Spacer(Modifier.height(28.dp))
            }
        }
    }
}

@OptIn(ExperimentalLayoutApi::class)
@Composable
private fun PermissionMessage(block: ChatBlock, viewModel: ChatViewModel) {
    Card(
        colors = CardDefaults.cardColors(containerColor = Color(0x22FF9800)),
        modifier = Modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 6.dp),
    ) {
        Column(Modifier.padding(14.dp)) {
            Row(verticalAlignment = Alignment.CenterVertically) {
                Icon(Icons.Default.Warning, null, tint = Color(0xFFFFB74D))
                Spacer(Modifier.width(8.dp))
                Text("需要确认", fontWeight = FontWeight.Bold)
            }
            Spacer(Modifier.height(8.dp))
            Text(block.text)
            Spacer(Modifier.height(12.dp))
            FlowRow(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                block.options.forEach { option -> Button(onClick = { viewModel.answerPermission(block, option.id) }) { Text(option.label) } }
                OutlinedButton(onClick = { viewModel.answerPermission(block, null) }) { Text("取消") }
            }
        }
    }
}

@Composable
private fun PlanMessage(block: ChatBlock) {
    Card(
        colors = CardDefaults.cardColors(containerColor = Color(0x221F7AE0)),
        modifier = Modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 4.dp),
    ) {
        Row(Modifier.padding(12.dp), verticalAlignment = Alignment.Top) {
            Icon(Icons.Default.Description, null, tint = MaterialTheme.colorScheme.primary)
            Spacer(Modifier.width(8.dp))
            Text(block.text, style = MaterialTheme.typography.bodySmall)
        }
    }
}

internal fun toolIcon(title: String): ImageVector {
    val value = title.lowercase()
    return when {
        value.contains("terminal") || value.contains("shell") || value.contains("command") -> Icons.Default.Terminal
        value.contains("browser") || value.contains("web") -> Icons.Default.Language
        value.contains("file") || value.contains("read") || value.contains("write") || value.contains("edit") -> Icons.Default.Description
        else -> Icons.Default.Settings
    }
}
