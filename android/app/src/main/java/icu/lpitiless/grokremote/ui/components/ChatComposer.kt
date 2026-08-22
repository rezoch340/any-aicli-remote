package icu.lpitiless.grokremote.ui.components

import androidx.compose.foundation.background
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
import icu.lpitiless.grokremote.model.ChatBlock

@Composable
internal fun ChatComposer(
    text: String,
    busy: Boolean,
    status: String,
    onTextChange: (String) -> Unit,
    onSend: () -> Unit,
    onStop: () -> Unit,
) {
    Surface(color = MaterialTheme.colorScheme.background) {
        Column(Modifier.fillMaxWidth().padding(horizontal = 10.dp, vertical = 8.dp)) {
            Surface(
                color = Color(0xFF191A1E),
                shape = RoundedCornerShape(20.dp),
                shadowElevation = 5.dp,
            ) {
                Column(Modifier.fillMaxWidth().padding(top = 10.dp, bottom = 8.dp)) {
                    BasicTextField(
                        value = text,
                        onValueChange = onTextChange,
                        minLines = 1,
                        maxLines = 7,
                        textStyle = MaterialTheme.typography.bodyLarge.copy(color = MaterialTheme.colorScheme.onSurface),
                        cursorBrush = SolidColor(MaterialTheme.colorScheme.onSurface),
                        modifier = Modifier.fillMaxWidth().heightIn(min = 34.dp, max = 148.dp),
                        decorationBox = { innerTextField ->
                            Box(
                                Modifier.fillMaxWidth().padding(horizontal = 14.dp, vertical = 5.dp),
                                contentAlignment = Alignment.CenterStart,
                            ) {
                                if (text.isBlank()) {
                                    Text(
                                        "给 Grok 发送消息",
                                        style = MaterialTheme.typography.bodyLarge,
                                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                                    )
                                }
                                innerTextField()
                            }
                        },
                    )

                    Row(
                        modifier = Modifier.fillMaxWidth().padding(horizontal = 9.dp, vertical = 2.dp),
                        verticalAlignment = Alignment.CenterVertically,
                    ) {
                        Surface(
                            modifier = Modifier.size(34.dp),
                            shape = CircleShape,
                            color = Color(0xFF24252A),
                        ) {
                            Box(contentAlignment = Alignment.Center) {
                                Icon(
                                    Icons.Default.Add,
                                    contentDescription = "添加附件",
                                    tint = MaterialTheme.colorScheme.onSurfaceVariant,
                                    modifier = Modifier.size(19.dp),
                                )
                            }
                        }

                        val showStatus = busy || status.contains("失败") || status.contains("重连")
                        if (showStatus && status.isNotBlank()) {
                            Text(
                                status,
                                modifier = Modifier.padding(start = 10.dp),
                                style = MaterialTheme.typography.labelSmall,
                                color = MaterialTheme.colorScheme.onSurfaceVariant,
                                maxLines = 1,
                                overflow = TextOverflow.Ellipsis,
                            )
                        }
                        Spacer(Modifier.weight(1f))
                        FilledIconButton(
                            onClick = if (busy) onStop else onSend,
                            enabled = busy || text.isNotBlank(),
                            colors = IconButtonDefaults.filledIconButtonColors(
                                containerColor = MaterialTheme.colorScheme.onSurface,
                                contentColor = MaterialTheme.colorScheme.background,
                                disabledContainerColor = Color(0xFF303137),
                                disabledContentColor = Color(0xFF777981),
                            ),
                            modifier = Modifier.size(34.dp),
                        ) {
                            Icon(
                                if (busy) Icons.Default.Stop else Icons.Default.ArrowUpward,
                                if (busy) "停止" else "发送",
                                modifier = Modifier.size(18.dp),
                            )
                        }
                    }
                }
            }
        }
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
        IconButton(onClick = onStop, modifier = Modifier.size(34.dp)) { Icon(Icons.Default.Stop, "停止工具", tint = Color(0xFFFF6B6B)) }
    }
}
