package icu.lpitiless.grokremote.ui.screens

import androidx.compose.foundation.background
import androidx.compose.foundation.interaction.DragInteraction
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.imePadding
import androidx.compose.foundation.layout.navigationBarsPadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.lazy.rememberLazyListState
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.material.icons.filled.ArrowDownward
import androidx.compose.material.icons.filled.CheckCircle
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.FilledIconButton
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.IconButtonDefaults
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.TopAppBar
import androidx.compose.material3.TopAppBarDefaults
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.derivedStateOf
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.runtime.snapshotFlow
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import icu.lpitiless.grokremote.model.ChatBlockKind
import icu.lpitiless.grokremote.model.ToolRunState
import icu.lpitiless.grokremote.ui.ChatUiState
import icu.lpitiless.grokremote.ui.ChatViewModel
import icu.lpitiless.grokremote.ui.components.ChatBlockItem
import icu.lpitiless.grokremote.ui.components.ChatComposer
import icu.lpitiless.grokremote.ui.components.FloatingToolStatus
import kotlinx.coroutines.flow.distinctUntilChanged
import kotlinx.coroutines.launch

@OptIn(ExperimentalMaterial3Api::class)
@Composable
internal fun ChatScreen(state: ChatUiState, viewModel: ChatViewModel) {
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
                ChatComposer(
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
