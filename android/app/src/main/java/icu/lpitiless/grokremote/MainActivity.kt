package icu.lpitiless.grokremote

import android.content.Intent
import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.compose.BackHandler
import androidx.activity.compose.setContent
import androidx.activity.viewModels
import androidx.compose.animation.AnimatedVisibility
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.combinedClickable
import androidx.compose.foundation.ExperimentalFoundationApi
import androidx.compose.foundation.layout.ExperimentalLayoutApi
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.imePadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.lazy.rememberLazyListState
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.material.icons.filled.Add
import androidx.compose.material.icons.filled.ArrowDownward
import androidx.compose.material.icons.filled.ArrowUpward
import androidx.compose.material.icons.filled.AutoAwesome
import androidx.compose.material.icons.filled.Bolt
import androidx.compose.material.icons.filled.Lightbulb
import androidx.compose.material.icons.filled.Cancel
import androidx.compose.material.icons.filled.CheckCircle
import androidx.compose.material.icons.filled.Close
import androidx.compose.material.icons.filled.CloudOff
import androidx.compose.material.icons.filled.Code
import androidx.compose.material.icons.filled.Description
import androidx.compose.material.icons.filled.Error
import androidx.compose.material.icons.filled.Folder
import androidx.compose.material.icons.filled.Language
import androidx.compose.material.icons.filled.MoreVert
import androidx.compose.material.icons.filled.Refresh
import androidx.compose.material.icons.filled.Settings
import androidx.compose.material.icons.filled.Stop
import androidx.compose.material.icons.filled.Terminal
import androidx.compose.material.icons.filled.Warning
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.Divider
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.FilledIconButton
import androidx.compose.material3.FloatingActionButton
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.ListItem
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.ModalBottomSheet
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.TopAppBar
import androidx.compose.material3.TopAppBarDefaults
import androidx.compose.material3.darkColorScheme
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.derivedStateOf
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.platform.LocalClipboardManager
import androidx.compose.ui.text.AnnotatedString
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.input.PasswordVisualTransformation
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import icu.lpitiless.grokremote.model.ChatBlock
import icu.lpitiless.grokremote.model.ChatBlockKind
import icu.lpitiless.grokremote.model.ConnectionStatus
import icu.lpitiless.grokremote.model.SessionSummary
import icu.lpitiless.grokremote.model.ToolRunState
import icu.lpitiless.grokremote.ui.ChatUiState
import icu.lpitiless.grokremote.ui.ChatViewModel
import com.mikepenz.markdown.m3.Markdown
import java.text.DateFormat
import java.util.Date
import kotlinx.coroutines.launch

class MainActivity : ComponentActivity() {
    private val viewModel: ChatViewModel by viewModels()

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        handlePairingIntent(intent)
        setContent {
            GrokRemoteTheme {
                val state by viewModel.state.collectAsStateWithLifecycle()
                GrokRemoteRoot(state, viewModel)
            }
        }
    }

    override fun onNewIntent(intent: Intent) {
        super.onNewIntent(intent)
        setIntent(intent)
        handlePairingIntent(intent)
    }

    private fun handlePairingIntent(intent: Intent?) {
        val data = intent?.data ?: return
        if (data.scheme != "grokremote" || data.host != "pair") return
        val address = data.getQueryParameter("url") ?: return
        val key = data.getQueryParameter("key").orEmpty()
        val cwd = data.getQueryParameter("cwd") ?: viewModel.state.value.defaultCwd
        viewModel.updatePairing(address = address, key = key, cwd = cwd)
        viewModel.connect(address, key, cwd)
    }
}

@Composable
private fun GrokRemoteTheme(content: @Composable () -> Unit) {
    val colors = darkColorScheme(
        primary = Color(0xFF67E8F9),
        onPrimary = Color(0xFF071014),
        background = Color(0xFF090A0C),
        surface = Color(0xFF111317),
        surfaceVariant = Color(0xFF1A1D22),
        outline = Color(0xFF353A43),
    )
    MaterialTheme(colorScheme = colors) {
        Surface(
            modifier = Modifier.fillMaxSize(),
            color = MaterialTheme.colorScheme.background,
            content = content,
        )
    }
}

@Composable
private fun GrokRemoteRoot(state: ChatUiState, viewModel: ChatViewModel) {
    when {
        state.connection !in setOf(ConnectionStatus.CONNECTED, ConnectionStatus.RECONNECTING) ->
            PairingScreen(state, viewModel)
        state.selectedSession != null -> {
            BackHandler { viewModel.closeSession() }
            ChatScreen(state, viewModel)
        }
        else -> SessionListScreen(state, viewModel)
    }
}

@Composable
private fun PairingScreen(state: ChatUiState, viewModel: ChatViewModel) {
    Column(
        modifier = Modifier.fillMaxSize().verticalScroll(rememberScrollState()).padding(24.dp),
        verticalArrangement = Arrangement.Center,
    ) {
        Icon(Icons.Default.Bolt, null, tint = MaterialTheme.colorScheme.primary, modifier = Modifier.size(58.dp))
        Spacer(Modifier.height(12.dp))
        Text("连接 Grok Remote", style = MaterialTheme.typography.headlineLarge, fontWeight = FontWeight.Bold)
        Text("粘贴 connect.url，或分别输入服务地址与配对 Key。", color = MaterialTheme.colorScheme.onSurfaceVariant)
        Spacer(Modifier.height(28.dp))
        OutlinedTextField(
            value = state.address,
            onValueChange = { viewModel.updatePairing(address = it) },
            label = { Text("服务地址") },
            placeholder = { Text("http://192.168.1.100:2421") },
            singleLine = true,
            modifier = Modifier.fillMaxWidth(),
        )
        Spacer(Modifier.height(12.dp))
        OutlinedTextField(
            value = state.pairingKey,
            onValueChange = { viewModel.updatePairing(key = it) },
            label = { Text("配对 Key") },
            visualTransformation = PasswordVisualTransformation(),
            singleLine = true,
            modifier = Modifier.fillMaxWidth(),
        )
        Spacer(Modifier.height(12.dp))
        OutlinedTextField(
            value = state.defaultCwd,
            onValueChange = { viewModel.updatePairing(cwd = it) },
            label = { Text("默认工作目录") },
            singleLine = true,
            modifier = Modifier.fillMaxWidth(),
        )
        Spacer(Modifier.height(20.dp))
        Button(
            onClick = { viewModel.connect() },
            enabled = state.connection != ConnectionStatus.CONNECTING,
            modifier = Modifier.fillMaxWidth().height(52.dp),
        ) {
            if (state.connection == ConnectionStatus.CONNECTING) {
                CircularProgressIndicator(modifier = Modifier.size(20.dp), strokeWidth = 2.dp)
                Spacer(Modifier.width(10.dp))
            }
            Text(if (state.connection == ConnectionStatus.CONNECTING) "连接中" else "连接")
        }
        state.error?.let {
            Spacer(Modifier.height(14.dp))
            Row(verticalAlignment = Alignment.CenterVertically) {
                Icon(Icons.Default.Warning, null, tint = Color(0xFFFFB74D))
                Spacer(Modifier.width(8.dp))
                Text(it, color = Color(0xFFFFB74D), style = MaterialTheme.typography.bodySmall)
            }
        }
    }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun SessionListScreen(state: ChatUiState, viewModel: ChatViewModel) {
    var showNew by remember { mutableStateOf(false) }
    var cwd by remember(state.defaultCwd) { mutableStateOf(state.defaultCwd) }
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
            FloatingActionButton(onClick = { showNew = true }) { Icon(Icons.Default.Add, "新建会话") }
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
            text = { OutlinedTextField(cwd, { cwd = it }, label = { Text("工作目录") }, singleLine = true) },
            confirmButton = {
                TextButton(onClick = { showNew = false; viewModel.createSession(cwd) }, enabled = cwd.isNotBlank()) { Text("创建") }
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

@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun ChatScreen(state: ChatUiState, viewModel: ChatViewModel) {
    val session = state.selectedSession ?: return
    val listState = rememberLazyListState()
    val scope = rememberCoroutineScope()
    var draft by remember { mutableStateOf("") }
    var follow by remember { mutableStateOf(true) }
    var effortMenu by remember { mutableStateOf(false) }
    val activeTool = state.blocks.lastOrNull { it.kind == ChatBlockKind.TOOL && it.toolState in setOf(ToolRunState.PENDING, ToolRunState.RUNNING) }
    val farFromBottom by remember { derivedStateOf { listState.canScrollForward } }

    LaunchedEffect(state.blocks) {
        if (follow && state.blocks.isNotEmpty()) listState.animateScrollToItem(state.blocks.lastIndex)
    }

    Scaffold(
        topBar = {
            TopAppBar(
                navigationIcon = { IconButton(onClick = viewModel::closeSession) { Icon(Icons.AutoMirrored.Filled.ArrowBack, "返回") } },
                title = {
                    Column {
                        Text(session.title, maxLines = 1, overflow = TextOverflow.Ellipsis, style = MaterialTheme.typography.titleMedium)
                        Text(state.model.currentModelId, fontFamily = FontFamily.Monospace, style = MaterialTheme.typography.labelSmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
                    }
                },
                actions = {
                    Box {
                        TextButton(onClick = { effortMenu = true }) { Text(state.model.effort.uppercase(), fontWeight = FontWeight.Bold) }
                        DropdownMenu(expanded = effortMenu, onDismissRequest = { effortMenu = false }) {
                            state.model.effortLevels.forEach { effort ->
                                DropdownMenuItem(
                                    text = { Text(effort) },
                                    trailingIcon = { if (effort == state.model.effort) Icon(Icons.Default.CheckCircle, null, tint = MaterialTheme.colorScheme.primary) },
                                    onClick = { effortMenu = false; viewModel.setEffort(effort) },
                                )
                            }
                        }
                    }
                },
            )
        },
        bottomBar = {
            Column(Modifier.imePadding()) {
                activeTool?.let { FloatingToolStatus(it, viewModel::cancel) }
                Composer(
                    text = draft,
                    busy = state.busy,
                    status = state.status,
                    onTextChange = { draft = it },
                    onSend = {
                        val outgoing = draft
                        draft = ""
                        follow = true
                        viewModel.send(outgoing)
                    },
                    onStop = viewModel::cancel,
                )
            }
        },
    ) { padding ->
        Box(Modifier.fillMaxSize().padding(padding)) {
            LazyColumn(
                state = listState,
                contentPadding = PaddingValues(vertical = 12.dp),
                verticalArrangement = Arrangement.spacedBy(3.dp),
                modifier = Modifier.fillMaxSize(),
            ) {
                items(state.blocks, key = { it.id }) { block -> ChatBlockItem(block, viewModel) }
            }
            if (farFromBottom) {
                FilledIconButton(
                    onClick = {
                        follow = true
                        if (state.blocks.isNotEmpty()) scope.launch { listState.animateScrollToItem(state.blocks.lastIndex) }
                    },
                    modifier = Modifier.align(Alignment.BottomEnd).padding(14.dp).size(42.dp),
                ) { Icon(Icons.Default.ArrowDownward, "滚动到底部") }
            }
        }
    }
}

@Composable
private fun Composer(
    text: String,
    busy: Boolean,
    status: String,
    onTextChange: (String) -> Unit,
    onSend: () -> Unit,
    onStop: () -> Unit,
) {
    Surface(tonalElevation = 4.dp) {
        Column(Modifier.fillMaxWidth().padding(horizontal = 12.dp, vertical = 8.dp)) {
            if (status.isNotEmpty()) Text(status, style = MaterialTheme.typography.labelSmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
            Spacer(Modifier.height(4.dp))
            Row(verticalAlignment = Alignment.Bottom, horizontalArrangement = Arrangement.spacedBy(9.dp)) {
                IconButton(onClick = { }, enabled = false) {
                    Icon(Icons.Default.Add, null, modifier = Modifier.background(MaterialTheme.colorScheme.surfaceVariant, CircleShape).padding(7.dp))
                }
                OutlinedTextField(
                    value = text,
                    onValueChange = onTextChange,
                    placeholder = { Text("给 Grok 发送消息") },
                    minLines = 1,
                    maxLines = 7,
                    modifier = Modifier.weight(1f),
                    shape = RoundedCornerShape(20.dp),
                )
                FilledIconButton(
                    onClick = if (busy) onStop else onSend,
                    enabled = busy || text.isNotBlank(),
                ) { Icon(if (busy) Icons.Default.Stop else Icons.Default.ArrowUpward, if (busy) "停止" else "发送") }
            }
        }
    }
}

@Composable
private fun FloatingToolStatus(block: ChatBlock, onStop: () -> Unit) {
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

@Composable
private fun ChatBlockItem(block: ChatBlock, viewModel: ChatViewModel) {
    when (block.kind) {
        ChatBlockKind.USER -> UserMessage(block)
        ChatBlockKind.ASSISTANT -> AssistantMessage(block)
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
            Text(
                block.text,
                fontSize = 16.5.sp,
                modifier = Modifier.widthIn(max = 330.dp).clip(RoundedCornerShape(18.dp))
                    .background(MaterialTheme.colorScheme.surfaceVariant)
                    .combinedClickable(onClick = { }, onLongClick = { menu = true })
                    .padding(horizontal = 14.dp, vertical = 10.dp),
            )
            DropdownMenu(expanded = menu, onDismissRequest = { menu = false }) {
                DropdownMenuItem(text = { Text("复制") }, onClick = { clipboard.setText(AnnotatedString(block.text)); menu = false })
            }
        }
    }
}

@Composable
private fun AssistantMessage(block: ChatBlock) {
    Column(Modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 8.dp)) {
        Row(verticalAlignment = Alignment.CenterVertically) {
            Icon(Icons.Default.AutoAwesome, null, tint = MaterialTheme.colorScheme.primary, modifier = Modifier.size(18.dp))
            Spacer(Modifier.width(6.dp))
            Text("Grok", fontWeight = FontWeight.SemiBold)
        }
        Spacer(Modifier.height(8.dp))
        Markdown(
            content = block.text,
            modifier = Modifier.fillMaxWidth(),
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

private fun toolIcon(title: String): ImageVector {
    val value = title.lowercase()
    return when {
        value.contains("terminal") || value.contains("shell") || value.contains("command") -> Icons.Default.Terminal
        value.contains("browser") || value.contains("web") -> Icons.Default.Language
        value.contains("file") || value.contains("read") || value.contains("write") || value.contains("edit") -> Icons.Default.Description
        else -> Icons.Default.Settings
    }
}
