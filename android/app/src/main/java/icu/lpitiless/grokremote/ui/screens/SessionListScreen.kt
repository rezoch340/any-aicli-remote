package icu.lpitiless.grokremote.ui.screens

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
import androidx.compose.material.icons.filled.Add
import androidx.compose.material.icons.filled.AutoAwesome
import androidx.compose.material.icons.filled.Bolt
import androidx.compose.material.icons.filled.CloudOff
import androidx.compose.material.icons.filled.Code
import androidx.compose.material.icons.filled.MoreVert
import androidx.compose.material.icons.filled.Refresh
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.CircularProgressIndicator
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
import icu.lpitiless.grokremote.model.ConnectionStatus
import icu.lpitiless.grokremote.model.SessionSummary
import icu.lpitiless.grokremote.ui.ChatUiState
import icu.lpitiless.grokremote.ui.ChatViewModel
import java.text.DateFormat
import java.util.Date

@OptIn(ExperimentalMaterial3Api::class)
@Composable
internal fun SessionListScreen(state: ChatUiState, viewModel: ChatViewModel) {
    var showNew by remember { mutableStateOf(false) }
    var cwd by remember(state.defaultCwd) {
        mutableStateOf(TextFieldValue(state.defaultCwd, TextRange(state.defaultCwd.length)))
    }
    var menu by remember { mutableStateOf(false) }
    Scaffold(
        topBar = {
            TopAppBar(
                title = { Text("Grok Remote", fontWeight = FontWeight.SemiBold) },
                actions = {
                    IconButton(onClick = viewModel::refreshSessions) { Icon(Icons.Default.Refresh, "刷新") }
                    Box {
                        IconButton(onClick = { menu = true }) { Icon(Icons.Default.MoreVert, "菜单") }
                        DropdownMenu(expanded = menu, onDismissRequest = { menu = false }) {
                            DropdownMenuItem(
                                text = { Text("断开连接") },
                                leadingIcon = { Icon(Icons.Default.CloudOff, null) },
                                onClick = { menu = false; viewModel.disconnect() },
                            )
                        }
                    }
                },
            )
        },
        floatingActionButton = {
            FloatingActionButton(
                onClick = {
                    cwd = cwd.copy(selection = TextRange(cwd.text.length))
                    showNew = true
                },
            ) { Icon(Icons.Default.Add, "新建会话") }
        },
    ) { padding ->
        Box(Modifier.fillMaxSize().padding(padding)) {
            if (state.connection == ConnectionStatus.RECONNECTING) {
                Row(
                    modifier = Modifier.fillMaxWidth().background(Color(0x33FF9800)).padding(10.dp),
                    verticalAlignment = Alignment.CenterVertically,
                ) {
                    CircularProgressIndicator(Modifier.size(16.dp), strokeWidth = 2.dp)
                    Spacer(Modifier.width(8.dp))
                    Text("连接中断，后台重连中", style = MaterialTheme.typography.bodySmall)
                }
            }
            if (state.sessions.isEmpty()) {
                Column(Modifier.align(Alignment.Center), horizontalAlignment = Alignment.CenterHorizontally) {
                    Icon(Icons.Default.AutoAwesome, null, modifier = Modifier.size(52.dp), tint = MaterialTheme.colorScheme.onSurfaceVariant)
                    Spacer(Modifier.height(12.dp))
                    Text("还没有会话", style = MaterialTheme.typography.titleMedium)
                    Text("点击右下角创建", color = MaterialTheme.colorScheme.onSurfaceVariant)
                }
            } else {
                LazyColumn(contentPadding = PaddingValues(top = if (state.connection == ConnectionStatus.RECONNECTING) 44.dp else 8.dp, bottom = 92.dp)) {
                    items(state.sessions, key = { it.id }) { session ->
                        SessionRow(session) { viewModel.openSession(session) }
                    }
                }
            }
        }
    }
    if (showNew) {
        AlertDialog(
            onDismissRequest = { showNew = false },
            title = { Text("新建会话") },
            text = {
                OutlinedTextField(
                    value = cwd,
                    onValueChange = { cwd = it },
                    label = { Text("工作目录") },
                    minLines = 2,
                    maxLines = 3,
                    modifier = Modifier.fillMaxWidth(),
                )
            },
            confirmButton = {
                TextButton(
                    onClick = { showNew = false; viewModel.createSession(cwd.text) },
                    enabled = cwd.text.isNotBlank(),
                ) { Text("创建") }
            },
            dismissButton = { TextButton(onClick = { showNew = false }) { Text("取消") } },
        )
    }
}

@Composable
private fun SessionRow(session: SessionSummary, onClick: () -> Unit) {
    ListItem(
        headlineContent = { Text(session.title, maxLines = 1, overflow = TextOverflow.Ellipsis, fontWeight = FontWeight.SemiBold) },
        supportingContent = {
            Row(verticalAlignment = Alignment.CenterVertically) {
                if (session.resident) {
                    Text("LIVE", color = Color(0xFF4ADE80), fontSize = 10.sp, fontWeight = FontWeight.Bold)
                    Spacer(Modifier.width(6.dp))
                }
                Text(session.cwd.ifEmpty { session.id.take(8) + "…" }, maxLines = 1, overflow = TextOverflow.Ellipsis)
            }
        },
        leadingContent = {
            Box(
                Modifier.size(44.dp).clip(RoundedCornerShape(12.dp))
                    .background(if (session.resident) Color(0x224ADE80) else MaterialTheme.colorScheme.surfaceVariant),
                contentAlignment = Alignment.Center,
            ) {
                Icon(if (session.resident) Icons.Default.Bolt else Icons.Default.Code, null, tint = if (session.resident) Color(0xFF4ADE80) else MaterialTheme.colorScheme.onSurfaceVariant)
            }
        },
        trailingContent = {
            if (session.updatedAt > 0) Text(DateFormat.getDateInstance(DateFormat.SHORT).format(Date(session.updatedAt)), style = MaterialTheme.typography.labelSmall)
        },
        modifier = Modifier.clickable(onClick = onClick),
    )
    HorizontalDivider(color = MaterialTheme.colorScheme.outline.copy(alpha = 0.35f))
}
