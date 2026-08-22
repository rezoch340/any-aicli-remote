package com.grokremote.app.ui.screens

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
import androidx.compose.runtime.rememberUpdatedState
import androidx.compose.runtime.setValue
import androidx.compose.runtime.snapshotFlow
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import com.grokremote.app.model.ChatBlock
import com.grokremote.app.model.ChatBlockKind
import com.grokremote.app.model.ToolRunState
import com.grokremote.app.ui.ChatUiState
import com.grokremote.app.ui.ChatViewModel
import com.grokremote.app.ui.components.AssistantMarkdownFragment
import com.grokremote.app.ui.components.AssistantMessageHeader
import com.grokremote.app.ui.components.ChatBlockItem
import com.grokremote.app.ui.components.ChatComposer
import com.grokremote.app.ui.components.FloatingToolStatus
import kotlinx.coroutines.FlowPreview
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.conflate
import kotlinx.coroutines.flow.distinctUntilChanged
import kotlinx.coroutines.flow.sample
import kotlinx.coroutines.launch

private sealed interface ChatRow {
    val key: String
    val contentType: String

    data class Block(val block: ChatBlock) : ChatRow {
        override val key = "block:${block.id}"
        override val contentType = block.kind.name
    }

    data class AssistantHeader(val messageId: String) : ChatRow {
        override val key = "assistant-header:$messageId"
        override val contentType = "assistant-header"
    }

    data class AssistantFragment(
        val messageId: String,
        val index: Int,
        val text: String,
        val streaming: Boolean,
    ) : ChatRow {
        override val key = "assistant-fragment:$messageId:$index"
        override val contentType = "assistant-fragment"
    }
}

@OptIn(ExperimentalMaterial3Api::class, FlowPreview::class)
@Composable
internal fun ChatScreen(state: ChatUiState, viewModel: ChatViewModel) {
    val session = state.selectedSession ?: return
    val listState = rememberLazyListState()
    val scope = rememberCoroutineScope()
    val bottomThresholdPx = with(LocalDensity.current) { 32.dp.roundToPx() }
    var draft by remember { mutableStateOf("") }
    var follow by remember { mutableStateOf(true) }
    var wasBusy by remember(session.id) { mutableStateOf(false) }
    var effortMenu by remember { mutableStateOf(false) }
    val activeTool = state.blocks.lastOrNull { it.kind == ChatBlockKind.TOOL && it.toolState in setOf(ToolRunState.PENDING, ToolRunState.RUNNING) }
    val rows = remember(state.blocks, state.busy) { buildChatRows(state.blocks, state.busy) }
    val newestFirstRows = remember(rows) { rows.reversed() }
    val streamRevision = remember(state.blocks) { streamRevision(state.blocks) }
    val farFromBottom by remember(listState, bottomThresholdPx) {
        derivedStateOf {
            listState.firstVisibleItemIndex != 0 ||
                listState.firstVisibleItemScrollOffset > bottomThresholdPx
        }
    }
    val latestFollow by rememberUpdatedState(follow)
    val latestBusy by rememberUpdatedState(state.busy)
    val latestRevision by rememberUpdatedState(streamRevision)
    val latestHasRows by rememberUpdatedState(rows.isNotEmpty())

    LaunchedEffect(listState, session.id) {
        snapshotFlow { latestRevision }
            .distinctUntilChanged()
            .conflate()
            .sample(120)
            .collect {
                if (
                    latestBusy && latestFollow && latestHasRows &&
                    !listState.isScrollInProgress &&
                    (listState.firstVisibleItemIndex != 0 || listState.firstVisibleItemScrollOffset != 0)
                ) {
                    listState.scrollToItem(0)
                }
            }
    }

    val newestBlock = state.blocks.lastOrNull()
    LaunchedEffect(session.id, newestBlock?.id) {
        if (newestBlock?.kind == ChatBlockKind.USER) {
            follow = true
            delay(16)
            if (latestHasRows) listState.scrollToItem(0)
        }
    }

    LaunchedEffect(state.busy) {
        val streamEnded = wasBusy && !state.busy
        wasBusy = state.busy
        if (streamEnded) {
            delay(220)
            if (latestFollow && latestHasRows && !listState.isScrollInProgress) {
                listState.scrollToItem(0)
            }
        }
    }

    LaunchedEffect(listState) {
        listState.interactionSource.interactions.collect { interaction ->
            when (interaction) {
                is DragInteraction.Start -> follow = false
                is DragInteraction.Stop, is DragInteraction.Cancel -> {
                    if (
                        listState.firstVisibleItemIndex == 0 &&
                        listState.firstVisibleItemScrollOffset <= bottomThresholdPx
                    ) {
                        follow = true
                    }
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
                reverseLayout = true,
                modifier = Modifier.fillMaxSize(),
            ) {
                items(
                    items = newestFirstRows,
                    key = ChatRow::key,
                    contentType = ChatRow::contentType,
                ) { row ->
                    when (row) {
                        is ChatRow.Block -> ChatBlockItem(row.block, viewModel)
                        is ChatRow.AssistantHeader -> AssistantMessageHeader()
                        is ChatRow.AssistantFragment -> AssistantMarkdownFragment(row.text, row.streaming)
                    }
                }
            }
            if (farFromBottom) {
                FilledIconButton(
                    onClick = {
                        follow = true
                        if (rows.isNotEmpty()) scope.launch { listState.scrollToItem(0) }
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

private fun buildChatRows(blocks: List<ChatBlock>, busy: Boolean): List<ChatRow> {
    val liveAssistantId = if (busy) blocks.lastOrNull { it.kind == ChatBlockKind.ASSISTANT }?.id else null
    return buildList {
        blocks.forEach { block ->
            if (block.kind != ChatBlockKind.ASSISTANT) {
                add(ChatRow.Block(block))
                return@forEach
            }

            add(ChatRow.AssistantHeader(block.id))
            val fragments = splitMarkdownFragments(block.text)
            fragments.forEachIndexed { index, fragment ->
                add(
                    ChatRow.AssistantFragment(
                        messageId = block.id,
                        index = index,
                        text = fragment,
                        streaming = block.id == liveAssistantId && index == fragments.lastIndex,
                    )
                )
            }
        }
    }
}

private fun splitMarkdownFragments(markdown: String): List<String> {
    if (markdown.isBlank()) return emptyList()
    val fragments = mutableListOf<String>()
    val current = StringBuilder()
    var fenceMarker: String? = null

    fun flush() {
        val value = current.toString().trimEnd('\n', '\r')
        if (value.isNotBlank()) fragments += value
        current.clear()
    }

    markdown.split('\n').forEach { line ->
        val trimmed = line.trimStart()
        val marker = when {
            trimmed.startsWith("```") -> "```"
            trimmed.startsWith("~~~") -> "~~~"
            else -> null
        }

        if (fenceMarker == null && marker != null) {
            flush()
            fenceMarker = marker
            current.append(line).append('\n')
        } else if (fenceMarker != null) {
            current.append(line).append('\n')
            if (marker == fenceMarker) {
                fenceMarker = null
                flush()
            }
        } else if (line.isBlank()) {
            flush()
        } else {
            current.append(line).append('\n')
        }
    }
    flush()
    return fragments
}

private fun streamRevision(blocks: List<ChatBlock>): Long {
    var revision = blocks.size.toLong()
    blocks.forEach { block ->
        revision = revision * 31 + block.id.hashCode()
        revision = revision * 31 + block.text.length
        revision = revision * 31 + block.detail.length
        revision = revision * 31 + block.toolState.ordinal
    }
    return revision
}
