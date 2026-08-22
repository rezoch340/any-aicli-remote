package icu.lpitiless.grokremote

import android.content.Intent
import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.compose.BackHandler
import androidx.activity.enableEdgeToEdge
import androidx.activity.compose.setContent
import androidx.activity.viewModels
import androidx.compose.animation.AnimatedVisibility
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.combinedClickable
import androidx.compose.foundation.ExperimentalFoundationApi
import androidx.compose.foundation.interaction.DragInteraction
import androidx.compose.foundation.text.BasicTextField
import androidx.compose.foundation.text.selection.SelectionContainer
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
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.imePadding
import androidx.compose.foundation.layout.navigationBarsPadding
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
import androidx.compose.material3.IconButtonDefaults
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
import androidx.compose.material3.Typography
import androidx.compose.material3.darkColorScheme
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.derivedStateOf
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.snapshotFlow
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.SolidColor
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.platform.LocalClipboardManager
import androidx.compose.ui.text.AnnotatedString
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontStyle
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
import com.mikepenz.markdown.compose.components.markdownComponents
import com.mikepenz.markdown.m3.markdownColor
import com.mikepenz.markdown.m3.markdownTypography
import java.text.DateFormat
import java.util.Date
import kotlinx.coroutines.flow.distinctUntilChanged
import kotlinx.coroutines.launch

class MainActivity : ComponentActivity() {
    private val viewModel: ChatViewModel by viewModels()

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        enableEdgeToEdge()
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
        primary = Color(0xFFF1F1F2),
        onPrimary = Color(0xFF101114),
        primaryContainer = Color(0xFF24252A),
        onPrimaryContainer = Color(0xFFEDEDEF),
        secondary = Color(0xFFC7C8CC),
        onSecondary = Color(0xFF101114),
        secondaryContainer = Color(0xFF24252A),
        onSecondaryContainer = Color(0xFFEDEDEF),
        tertiary = Color(0xFFC7C8CC),
        onTertiary = Color(0xFF101114),
        tertiaryContainer = Color(0xFF24252A),
        onTertiaryContainer = Color(0xFFEDEDEF),
        background = Color(0xFF090A0C),
        onBackground = Color(0xFFEDEDEF),
        surface = Color(0xFF0E0F12),
        onSurface = Color(0xFFEDEDEF),
        surfaceVariant = Color(0xFF1B1C20),
        onSurfaceVariant = Color(0xFF9B9DA4),
        outline = Color(0xFF303238),
    )
    val type = Typography(
        bodyLarge = MaterialTheme.typography.bodyLarge.copy(fontSize = 15.5.sp, lineHeight = 23.sp),
        bodyMedium = MaterialTheme.typography.bodyMedium.copy(fontSize = 14.5.sp, lineHeight = 21.sp),
        bodySmall = MaterialTheme.typography.bodySmall.copy(fontSize = 12.5.sp, lineHeight = 18.sp),
        titleMedium = MaterialTheme.typography.titleMedium.copy(fontSize = 16.sp, lineHeight = 21.sp),
        titleSmall = MaterialTheme.typography.titleSmall.copy(fontSize = 14.5.sp, lineHeight = 19.sp),
        labelSmall = MaterialTheme.typography.labelSmall.copy(fontSize = 11.sp, lineHeight = 14.sp),
    )
    MaterialTheme(colorScheme = colors, typography = type) {
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
    val lastBlock = state.blocks.lastOrNull()
    val scrollRevision = remember(state.blocks.size, lastBlock) {
        "${state.blocks.size}:${lastBlock?.id}:${lastBlock?.text?.length}:${lastBlock?.detail?.length}:${lastBlock?.toolState}"
    }

    LaunchedEffect(scrollRevision, follow) {
        if (!follow || state.blocks.isEmpty()) return@LaunchedEffect
        listState.scrollToItem(state.blocks.size)
        snapshotFlow {
            val layout = listState.layoutInfo
            val last = layout.visibleItemsInfo.lastOrNull()
            listOf(
                layout.totalItemsCount,
                layout.viewportEndOffset,
                last?.index ?: -1,
                last?.offset ?: 0,
                last?.size ?: 0,
            )
        }.distinctUntilChanged().collect {
            if (follow && listState.canScrollForward) {
                listState.scrollToItem(state.blocks.size)
            }
        }
    }

    LaunchedEffect(listState) {
        listState.interactionSource.interactions.collect { interaction ->
            when (interaction) {
                is DragInteraction.Start -> follow = false
                is DragInteraction.Stop, is DragInteraction.Cancel -> {
                    if (!listState.canScrollForward) follow = true
                }
            }
        }
    }

    Scaffold(
        topBar = {
            TopAppBar(
                navigationIcon = { IconButton(onClick = viewModel::closeSession) { Icon(Icons.AutoMirrored.Filled.ArrowBack, "返回") } },
                title = {
                    Column {
                        Text(
                            session.title,
                            maxLines = 1,
                            overflow = TextOverflow.Ellipsis,
                            style = MaterialTheme.typography.titleSmall,
                            fontWeight = FontWeight.SemiBold,
                        )
                        Row(verticalAlignment = Alignment.CenterVertically) {
                            Box(Modifier.size(5.dp).clip(CircleShape).background(Color(0xFF69C48A)))
                            Spacer(Modifier.width(6.dp))
                            Text(
                                state.model.currentModelId,
                                fontFamily = FontFamily.Monospace,
                                style = MaterialTheme.typography.labelSmall,
                                color = MaterialTheme.colorScheme.onSurfaceVariant,
                            )
                        }
                    }
                },
                actions = {
                    Box {
                        TextButton(onClick = { effortMenu = true }) {
                            Text(
                                state.model.effort.uppercase(),
                                style = MaterialTheme.typography.labelSmall,
                                fontWeight = FontWeight.SemiBold,
                                color = MaterialTheme.colorScheme.onSurfaceVariant,
                            )
                        }
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
                colors = TopAppBarDefaults.topAppBarColors(containerColor = MaterialTheme.colorScheme.background),
            )
        },
        bottomBar = {
            Column(Modifier.imePadding().navigationBarsPadding()) {
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
                item(key = "chat-bottom") { Spacer(Modifier.fillMaxWidth().height(1.dp)) }
            }
            if (farFromBottom) {
                FilledIconButton(
                    onClick = {
                        follow = true
                        if (state.blocks.isNotEmpty()) scope.launch { listState.scrollToItem(state.blocks.size) }
                    },
                    colors = IconButtonDefaults.filledIconButtonColors(
                        containerColor = MaterialTheme.colorScheme.surfaceVariant,
                        contentColor = MaterialTheme.colorScheme.onSurface,
                    ),
                    modifier = Modifier.align(Alignment.BottomEnd).padding(14.dp).size(38.dp),
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
                style = MaterialTheme.typography.bodyLarge,
                modifier = Modifier.widthIn(max = 304.dp).clip(RoundedCornerShape(18.dp))
                    .background(Color(0xFF1D1E22))
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
        link = body,
        table = MaterialTheme.typography.bodySmall.copy(color = MaterialTheme.colorScheme.onSurface),
    )
    val markdownColors = markdownColor(
        text = MaterialTheme.colorScheme.onSurface,
        codeText = Color(0xFFD6D7DA),
        inlineCodeText = Color(0xFFD6D7DA),
        linkText = Color(0xFFB8C7EA),
        codeBackground = Color(0xFF141519),
        inlineCodeBackground = Color(0xFF202126),
        dividerColor = MaterialTheme.colorScheme.outline,
        tableText = MaterialTheme.colorScheme.onSurface,
        tableBackground = Color(0xFF121317),
    )
    val markdownComponents = markdownComponents(
        table = { model ->
            val start = model.node.startOffset.coerceIn(0, model.content.length)
            val end = model.node.endOffset.coerceIn(start, model.content.length)
            CompactMarkdownTable(model.content.substring(start, end))
        },
    )

    Column(Modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 9.dp)) {
        Row(verticalAlignment = Alignment.CenterVertically) {
            Icon(Icons.Default.AutoAwesome, null, tint = Color(0xFFB8B49F), modifier = Modifier.size(16.dp))
            Spacer(Modifier.width(6.dp))
            Text("Grok", style = MaterialTheme.typography.titleSmall, fontWeight = FontWeight.SemiBold)
        }
        Spacer(Modifier.height(7.dp))
        Markdown(
            content = block.text,
            colors = markdownColors,
            typography = markdownType,
            components = markdownComponents,
            modifier = Modifier.fillMaxWidth(),
        )
    }
}

private data class CompactTableCell(val text: String, val code: Boolean = false)
private data class CompactTableData(
    val header: List<CompactTableCell>?,
    val rows: List<List<CompactTableCell>>,
    val columnCount: Int,
)

@Composable
private fun CompactMarkdownTable(markdown: String) {
    val table = remember(markdown) { parseCompactTable(markdown) }
    if (table.columnCount == 0) return
    val compactFirstColumn = table.columnCount == 2 &&
        (listOfNotNull(table.header) + table.rows).all { row -> row.firstOrNull()?.text.orEmpty().length <= 4 }
    val shape = RoundedCornerShape(12.dp)

    if (table.columnCount > 2) {
        CompactRecordTable(table, shape)
        return
    }

    SelectionContainer {
        Column(
            Modifier
                .fillMaxWidth()
                .padding(vertical = 5.dp)
                .border(1.dp, MaterialTheme.colorScheme.outline.copy(alpha = 0.7f), shape)
                .clip(shape)
                .background(Color(0xFF121317)),
        ) {
            table.header?.let { header ->
                CompactTableRow(
                    cells = header,
                    columnCount = table.columnCount,
                    header = true,
                    compactFirstColumn = compactFirstColumn,
                )
                if (table.rows.isNotEmpty()) {
                    HorizontalDivider(color = MaterialTheme.colorScheme.outline.copy(alpha = 0.7f))
                }
            }
            table.rows.forEachIndexed { index, row ->
                CompactTableRow(
                    cells = row,
                    columnCount = table.columnCount,
                    header = false,
                    compactFirstColumn = compactFirstColumn,
                )
                if (index != table.rows.lastIndex) {
                    HorizontalDivider(color = MaterialTheme.colorScheme.outline.copy(alpha = 0.4f))
                }
            }
        }
    }
}

@Composable
private fun CompactRecordTable(table: CompactTableData, shape: RoundedCornerShape) {
    SelectionContainer {
        Column(
            Modifier
                .fillMaxWidth()
                .padding(vertical = 5.dp)
                .border(1.dp, MaterialTheme.colorScheme.outline.copy(alpha = 0.7f), shape)
                .clip(shape)
                .background(Color(0xFF121317)),
        ) {
            table.rows.forEachIndexed { rowIndex, row ->
                Column(Modifier.fillMaxWidth().padding(horizontal = 13.dp, vertical = 12.dp)) {
                    Surface(
                        color = Color(0xFF24252A),
                        shape = RoundedCornerShape(7.dp),
                    ) {
                        Text(
                            row.firstOrNull()?.text?.ifBlank { "#${rowIndex + 1}" } ?: "#${rowIndex + 1}",
                            modifier = Modifier.padding(horizontal = 9.dp, vertical = 4.dp),
                            style = MaterialTheme.typography.labelSmall,
                            fontWeight = FontWeight.Bold,
                            color = MaterialTheme.colorScheme.onSurface,
                        )
                    }
                    Spacer(Modifier.height(11.dp))

                    for (column in 1 until table.columnCount) {
                        val cell = row.getOrNull(column) ?: continue
                        if (cell.text.isBlank()) continue
                        val label = table.header?.getOrNull(column)?.text.orEmpty()
                        if (label.isNotBlank()) {
                            Text(
                                label,
                                style = MaterialTheme.typography.labelSmall,
                                color = MaterialTheme.colorScheme.onSurfaceVariant,
                            )
                            Spacer(Modifier.height(3.dp))
                        }
                        Text(
                            cell.text,
                            style = MaterialTheme.typography.bodySmall.copy(
                                fontFamily = if (cell.code || label.contains("id", ignoreCase = true) || label.contains("种子")) {
                                    FontFamily.Monospace
                                } else {
                                    FontFamily.Default
                                },
                                lineHeight = 18.sp,
                            ),
                            color = MaterialTheme.colorScheme.onSurface,
                            softWrap = true,
                        )
                        if (column != table.columnCount - 1) Spacer(Modifier.height(10.dp))
                    }
                }
                if (rowIndex != table.rows.lastIndex) {
                    HorizontalDivider(color = MaterialTheme.colorScheme.outline.copy(alpha = 0.45f))
                }
            }
        }
    }
}

@Composable
private fun CompactTableRow(
    cells: List<CompactTableCell>,
    columnCount: Int,
    header: Boolean,
    compactFirstColumn: Boolean,
) {
    val normalized = List(columnCount) { cells.getOrNull(it) ?: CompactTableCell("") }
    if (header && compactFirstColumn && normalized.first().text.isBlank()) {
        Box(
            Modifier.fillMaxWidth().background(Color(0xFF17181C)).padding(horizontal = 12.dp, vertical = 11.dp),
            contentAlignment = Alignment.Center,
        ) {
            Text(
                normalized[1].text,
                style = MaterialTheme.typography.bodySmall,
                fontWeight = FontWeight.SemiBold,
                color = MaterialTheme.colorScheme.onSurface,
            )
        }
        return
    }

    Row(
        Modifier
            .fillMaxWidth()
            .background(if (header) Color(0xFF17181C) else Color.Transparent),
        verticalAlignment = Alignment.Top,
    ) {
        normalized.forEachIndexed { index, cell ->
            val cellModifier = if (compactFirstColumn && index == 0) {
                Modifier.width(48.dp)
            } else {
                Modifier.weight(if (compactFirstColumn) 1f else compactTableColumnWeight(normalized, index))
            }
            Text(
                text = cell.text,
                modifier = cellModifier.padding(horizontal = 12.dp, vertical = 10.dp),
                style = MaterialTheme.typography.bodySmall.copy(
                    fontFamily = if (cell.code) FontFamily.Monospace else FontFamily.Default,
                    lineHeight = 18.sp,
                ),
                fontWeight = if (header) FontWeight.SemiBold else FontWeight.Normal,
                color = if (cell.code) Color(0xFFD6D7DA) else MaterialTheme.colorScheme.onSurface,
                softWrap = true,
            )
        }
    }
}

private fun compactTableColumnWeight(row: List<CompactTableCell>, index: Int): Float {
    val length = row.getOrNull(index)?.text.orEmpty().length.coerceIn(4, 24)
    return length.toFloat()
}

private fun parseCompactTable(markdown: String): CompactTableData {
    val parsedRows = markdown.lineSequence()
        .map(String::trim)
        .filter(String::isNotEmpty)
        .map(::splitMarkdownTableRow)
        .filter { it.isNotEmpty() }
        .toList()
    if (parsedRows.isEmpty()) return CompactTableData(null, emptyList(), 0)

    val separator = parsedRows.indexOfFirst { row ->
        row.isNotEmpty() && row.all { it.text.matches(Regex(":?-{3,}:?")) }
    }
    val header = if (separator > 0) parsedRows[separator - 1] else null
    val rows = if (separator >= 0) parsedRows.drop(separator + 1) else parsedRows
    val columnCount = (listOfNotNull(header) + rows).maxOfOrNull(List<CompactTableCell>::size) ?: 0
    return CompactTableData(header, rows, columnCount)
}

private fun splitMarkdownTableRow(line: String): List<CompactTableCell> {
    var source = line.trim()
    if (source.startsWith('|')) source = source.drop(1)
    if (source.endsWith('|') && !source.endsWith("\\|")) source = source.dropLast(1)

    val rawCells = mutableListOf<String>()
    val cell = StringBuilder()
    var escaped = false
    var inCode = false
    source.forEach { char ->
        when {
            escaped -> {
                cell.append(char)
                escaped = false
            }
            char == '\\' -> escaped = true
            char == '`' -> {
                inCode = !inCode
                cell.append(char)
            }
            char == '|' && !inCode -> {
                rawCells += cell.toString()
                cell.clear()
            }
            else -> cell.append(char)
        }
    }
    if (escaped) cell.append('\\')
    rawCells += cell.toString()

    return rawCells.map { raw ->
        val trimmed = raw.trim()
        val code = trimmed.length >= 2 && trimmed.startsWith('`') && trimmed.endsWith('`')
        CompactTableCell(
            text = if (code) trimmed.drop(1).dropLast(1).trim() else trimmed,
            code = code,
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
