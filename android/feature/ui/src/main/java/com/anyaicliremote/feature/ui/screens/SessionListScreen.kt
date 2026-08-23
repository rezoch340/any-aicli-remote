package com.anyaicliremote.feature.ui.screens

import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.material.icons.filled.Add
import androidx.compose.material.icons.filled.AutoAwesome
import androidx.compose.material.icons.filled.Bolt
import androidx.compose.material.icons.filled.CloudOff
import androidx.compose.material.icons.filled.Code
import androidx.compose.material.icons.filled.MoreVert
import androidx.compose.material.icons.filled.Refresh
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.FloatingActionButton
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.ListItem
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.TopAppBar
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.TextRange
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.input.TextFieldValue
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.anyaicliremote.core.model.SessionSummary
import com.anyaicliremote.feature.ui.ChatUiState
import com.anyaicliremote.feature.ui.ChatViewModel
import com.anyaicliremote.feature.ui.theme.AnyAIColors
import java.text.DateFormat
import java.util.Date

private const val SESSION_ICON_SIZE = 44
private const val LIVE_LABEL_SIZE = 10
private const val DIVIDER_ALPHA = 0.35f
private const val SESSION_ID_PREVIEW_LENGTH = 8
private const val LIVE_BACKGROUND_ALPHA = 0.16f

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun SessionListScreen(state: ChatUiState, viewModel: ChatViewModel) {
    var showNewSession by remember { mutableStateOf(false) }
    var workingDirectory by remember { mutableStateOf(TextFieldValue("~", TextRange(1))) }
    var menuExpanded by remember { mutableStateOf(false) }
    Scaffold(
        topBar = {
            TopAppBar(
                title = {
                    Text(
                        state.devices.firstOrNull { it.id == state.activeDeviceId }?.name ?: "会话",
                        fontWeight = FontWeight.SemiBold,
                    )
                },
                navigationIcon = {
                    IconButton(onClick = viewModel::disconnect) {
                        Icon(Icons.AutoMirrored.Filled.ArrowBack, "返回设备列表")
                    }
                },
                actions = {
                    IconButton(onClick = viewModel::refreshSessions) { Icon(Icons.Default.Refresh, "刷新") }
                    Box {
                        IconButton(onClick = { menuExpanded = true }) { Icon(Icons.Default.MoreVert, "菜单") }
                        DropdownMenu(expanded = menuExpanded, onDismissRequest = { menuExpanded = false }) {
                            DropdownMenuItem(
                                text = { Text("断开连接") },
                                leadingIcon = { Icon(Icons.Default.CloudOff, null) },
                                onClick = {
                                    menuExpanded = false
                                    viewModel.disconnect()
                                },
                            )
                        }
                    }
                },
            )
        },
        floatingActionButton = {
            FloatingActionButton(
                onClick = {
                    workingDirectory = workingDirectory.copy(
                        selection = TextRange(workingDirectory.text.length),
                    )
                    showNewSession = true
                },
            ) { Icon(Icons.Default.Add, "新建会话") }
        },
    ) { padding ->
        Box(Modifier.fillMaxSize().padding(padding)) {
            if (state.sessions.isEmpty()) {
                Column(Modifier.align(Alignment.Center), horizontalAlignment = Alignment.CenterHorizontally) {
                    Icon(Icons.Default.AutoAwesome, null, modifier = Modifier.size(52.dp), tint = MaterialTheme.colorScheme.onSurfaceVariant)
                    Spacer(Modifier.height(12.dp))
                    Text("还没有会话", style = MaterialTheme.typography.titleMedium)
                    Text("点击右下角创建", color = MaterialTheme.colorScheme.onSurfaceVariant)
                }
            } else {
                LazyColumn(contentPadding = PaddingValues(top = 8.dp, bottom = 92.dp)) {
                    items(state.sessions, key = { "${it.providerId}:${it.id}" }) { session ->
                        SessionRow(session) { viewModel.openSession(session) }
                    }
                }
            }
        }
    }
    if (showNewSession) {
        NewSessionDialog(
            workingDirectory = workingDirectory,
            onWorkingDirectoryChange = { workingDirectory = it },
            onDismiss = { showNewSession = false },
            onCreate = {
                showNewSession = false
                viewModel.createSession(workingDirectory.text)
            },
        )
    }
}


@Composable
private fun NewSessionDialog(
    workingDirectory: TextFieldValue,
    onWorkingDirectoryChange: (TextFieldValue) -> Unit,
    onDismiss: () -> Unit,
    onCreate: () -> Unit,
) {
    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text("新建会话") },
        text = {
            OutlinedTextField(
                value = workingDirectory,
                onValueChange = onWorkingDirectoryChange,
                label = { Text("工作目录") },
                minLines = 2,
                maxLines = 3,
                modifier = Modifier.fillMaxWidth(),
            )
        },
        confirmButton = {
            TextButton(onClick = onCreate, enabled = workingDirectory.text.isNotBlank()) { Text("创建") }
        },
        dismissButton = { TextButton(onClick = onDismiss) { Text("取消") } },
    )
}
@Composable
private fun SessionRow(session: SessionSummary, onClick: () -> Unit) {
    ListItem(
        headlineContent = { Text(session.title, maxLines = 1, overflow = TextOverflow.Ellipsis, fontWeight = FontWeight.SemiBold) },
        supportingContent = {
            Row(verticalAlignment = Alignment.CenterVertically) {
                if (session.resident) {
                    Text("LIVE", color = AnyAIColors.online, fontSize = LIVE_LABEL_SIZE.sp, fontWeight = FontWeight.Bold)
                    Spacer(Modifier.width(6.dp))
                }
                Text(
                    session.projectDirectory.ifEmpty { session.id.take(SESSION_ID_PREVIEW_LENGTH) + "…" },
                    maxLines = 1,
                    overflow = TextOverflow.Ellipsis,
                )
            }
        },
        leadingContent = {
            Box(
                Modifier.size(SESSION_ICON_SIZE.dp).clip(RoundedCornerShape(12.dp))
                    .background(if (session.resident) AnyAIColors.online.copy(alpha = LIVE_BACKGROUND_ALPHA) else MaterialTheme.colorScheme.surfaceVariant),
                contentAlignment = Alignment.Center,
            ) {
                Icon(
                    if (session.resident) Icons.Default.Bolt else Icons.Default.Code,
                    null,
                    tint = if (session.resident) AnyAIColors.online else MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
        },
        trailingContent = {
            if (session.updatedAt > 0) {
                Text(
                    DateFormat.getDateInstance(DateFormat.SHORT).format(Date(session.updatedAt)),
                    style = MaterialTheme.typography.labelSmall,
                )
            }
        },
        modifier = Modifier.clickable(onClick = onClick),
    )
    HorizontalDivider(color = MaterialTheme.colorScheme.outline.copy(alpha = DIVIDER_ALPHA))
}
