package com.anyaicliremote.feature.ui.screens

import androidx.compose.foundation.background
import androidx.compose.foundation.interaction.DragInteraction
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.BoxScope
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.imePadding
import androidx.compose.foundation.layout.navigationBarsPadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.LazyListState
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
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import com.anyaicliremote.core.model.ChatBlock
import com.anyaicliremote.core.model.ChatBlockKind
import com.anyaicliremote.core.model.ToolRunState
import com.anyaicliremote.feature.ui.ChatUiState
import com.anyaicliremote.feature.ui.ChatViewModel
import com.anyaicliremote.feature.ui.components.ChatBlockItem
import com.anyaicliremote.feature.ui.components.ChatComposer
import com.anyaicliremote.feature.ui.components.ChatComposerActions
import com.anyaicliremote.feature.ui.components.ChatComposerState
import com.anyaicliremote.feature.ui.components.FloatingToolStatus
import com.anyaicliremote.feature.ui.components.WorkspaceFilePickerDialog
import kotlinx.coroutines.launch

private const val MODEL_INDICATOR_SIZE = 5
private const val MODEL_INDICATOR_COLOR = 0xFF69C48A

private data class ChatEffectsState(
    val state: ChatUiState,
    val sessionId: String,
    val listState: LazyListState,
    val threshold: Int,
    val hasRows: Boolean,
    val follow: Boolean,
    val setFollow: (Boolean) -> Unit,
)

private data class ChatContentState(
    val viewModel: ChatViewModel,
    val listState: LazyListState,
    val rows: List<ChatRow>,
    val farFromBottom: Boolean,
    val onScrollToBottom: () -> Unit,
)

private data class ChatScaffoldState(
    val state: ChatUiState,
    val sessionTitle: String,
    val content: ChatContentState,
    val draft: String,
)

private data class ChatScaffoldActions(
    val onDraftChange: (String) -> Unit,
    val onSend: (String) -> Unit,
)

private sealed interface ChatRow {
    val key: String
    val contentType: String

    data class Block(val block: ChatBlock, val streaming: Boolean) : ChatRow {
        override val key = "block:${block.id}"
        override val contentType = block.kind.name
    }
}

@Composable
fun ChatScreen(state: ChatUiState, viewModel: ChatViewModel) {
    val session = state.selectedSession ?: return
    val listState = rememberLazyListState()
    val scope = rememberCoroutineScope()
    val threshold = with(LocalDensity.current) { 32.dp.roundToPx() }
    var draft by remember { mutableStateOf("") }
    var follow by remember { mutableStateOf(true) }
    val rows = remember(state.blocks, state.busy) {
        buildChatRows(state.blocks, state.busy)
    }
    val farFromBottom by remember(listState, threshold) {
        derivedStateOf { isFarFromBottom(listState, threshold) }
    }
    val contentState = ChatContentState(
        viewModel = viewModel,
        listState = listState,
        rows = rows,
        farFromBottom = farFromBottom,
        onScrollToBottom = {
            follow = true
            if (rows.isNotEmpty()) scope.launch { listState.scrollToItem(0) }
        },
    )
    ChatEffects(
        ChatEffectsState(
            state = state,
            sessionId = session.id,
            listState = listState,
            threshold = threshold,
            hasRows = rows.isNotEmpty(),
            follow = follow,
            setFollow = { follow = it },
        ),
    )
    ChatScaffold(
        scaffoldState = ChatScaffoldState(
            state = state,
            sessionTitle = session.title,
            content = contentState,
            draft = draft,
        ),
        viewModel = viewModel,
        actions = ChatScaffoldActions(
            onDraftChange = { draft = it },
            onSend = { outgoing ->
                draft = ""
                follow = true
                viewModel.send(outgoing)
            },
        ),
    )
    ChatFilePicker(state, viewModel)
}

@Composable
private fun ChatEffects(effectState: ChatEffectsState) {
    FollowNewMessages(
        newestBlock = effectState.state.blocks.lastOrNull(),
        sessionId = effectState.sessionId,
        listState = effectState.listState,
        hasRows = effectState.hasRows,
        setFollow = effectState.setFollow,
    )
    FollowStreamCompletion(
        busy = effectState.state.busy,
        sessionId = effectState.sessionId,
        listState = effectState.listState,
        hasRows = effectState.hasRows,
        follow = effectState.follow,
    )
    TrackManualScrolling(
        listState = effectState.listState,
        threshold = effectState.threshold,
        setFollow = effectState.setFollow,
    )
}

@Composable
private fun FollowNewMessages(
    newestBlock: ChatBlock?,
    sessionId: String,
    listState: LazyListState,
    hasRows: Boolean,
    setFollow: (Boolean) -> Unit,
) {
    val latestRows by rememberUpdatedState(hasRows)
    LaunchedEffect(sessionId, newestBlock?.id) {
        if (newestBlock?.kind == ChatBlockKind.USER) {
            setFollow(true)
            if (latestRows) listState.scrollToItem(0)
        }
    }
}

@Composable
private fun FollowStreamCompletion(
    busy: Boolean,
    sessionId: String,
    listState: LazyListState,
    hasRows: Boolean,
    follow: Boolean,
) {
    var wasBusy by remember(sessionId) { mutableStateOf(false) }
    val latestFollow by rememberUpdatedState(follow)
    val latestRows by rememberUpdatedState(hasRows)
    LaunchedEffect(busy) {
        val shouldScroll = wasBusy && !busy && latestFollow && latestRows &&
            !listState.isScrollInProgress
        wasBusy = busy
        if (shouldScroll) listState.scrollToItem(0)
    }
}

@Composable
private fun TrackManualScrolling(
    listState: LazyListState,
    threshold: Int,
    setFollow: (Boolean) -> Unit,
) {
    LaunchedEffect(listState) {
        listState.interactionSource.interactions.collect { interaction ->
            when (interaction) {
                is DragInteraction.Start -> setFollow(false)
                is DragInteraction.Stop,
                is DragInteraction.Cancel -> {
                    if (!isFarFromBottom(listState, threshold)) setFollow(true)
                }
            }
        }
    }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun ChatScaffold(
    scaffoldState: ChatScaffoldState,
    viewModel: ChatViewModel,
    actions: ChatScaffoldActions,
) {
    Scaffold(
        topBar = {
            ChatTopBar(scaffoldState.state, viewModel, scaffoldState.sessionTitle)
        },
        bottomBar = {
            ChatBottomBar(
                state = scaffoldState.state,
                viewModel = viewModel,
                draft = scaffoldState.draft,
                onDraftChange = actions.onDraftChange,
                onSend = actions.onSend,
            )
        },
    ) { padding ->
        ChatContent(scaffoldState.content, Modifier.padding(padding))
    }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun ChatTopBar(
    state: ChatUiState,
    viewModel: ChatViewModel,
    sessionTitle: String,
) {
    var effortMenu by remember { mutableStateOf(false) }
    TopAppBar(
        navigationIcon = {
            IconButton(onClick = viewModel::closeSession) {
                Icon(Icons.AutoMirrored.Filled.ArrowBack, "返回")
            }
        },
        title = { ChatTitle(sessionTitle, state.model.currentModelId) },
        actions = {
            EffortMenu(state, viewModel, effortMenu) { effortMenu = it }
        },
        colors = TopAppBarDefaults.topAppBarColors(
            containerColor = MaterialTheme.colorScheme.background,
        ),
    )
}

@Composable
private fun ChatTitle(title: String, modelId: String) {
    Column {
        Text(
            text = title,
            maxLines = 1,
            overflow = TextOverflow.Ellipsis,
            style = MaterialTheme.typography.titleSmall,
            fontWeight = FontWeight.SemiBold,
        )
        Row(verticalAlignment = Alignment.CenterVertically) {
            Box(
                Modifier
                    .size(MODEL_INDICATOR_SIZE.dp)
                    .clip(CircleShape)
                    .background(Color(MODEL_INDICATOR_COLOR)),
            )
            Spacer(Modifier.width(6.dp))
            Text(
                text = modelId.ifBlank { "自动" },
                fontFamily = FontFamily.Monospace,
                style = MaterialTheme.typography.labelSmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
    }
}

@Composable
private fun EffortMenu(
    state: ChatUiState,
    viewModel: ChatViewModel,
    expanded: Boolean,
    setExpanded: (Boolean) -> Unit,
) {
    Box {
        TextButton(onClick = { setExpanded(true) }) {
            Text(
                text = state.model.effort.uppercase(),
                style = MaterialTheme.typography.labelSmall,
                fontWeight = FontWeight.SemiBold,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
        DropdownMenu(
            expanded = expanded,
            onDismissRequest = { setExpanded(false) },
        ) {
            state.model.effortLevels.forEach { effort ->
                DropdownMenuItem(
                    text = { Text(effort) },
                    trailingIcon = {
                        if (effort == state.model.effort) {
                            Icon(
                                Icons.Default.CheckCircle,
                                null,
                                tint = MaterialTheme.colorScheme.primary,
                            )
                        }
                    },
                    onClick = {
                        setExpanded(false)
                        viewModel.setEffort(effort)
                    },
                )
            }
        }
    }
}

@Composable
private fun ChatBottomBar(
    state: ChatUiState,
    viewModel: ChatViewModel,
    draft: String,
    onDraftChange: (String) -> Unit,
    onSend: (String) -> Unit,
) {
    Column(Modifier.imePadding().navigationBarsPadding()) {
        activeTool(state)?.let { FloatingToolStatus(it, viewModel::cancel) }
        ChatComposer(
            state = ChatComposerState(
                draft,
                state.busy,
                state.status,
                state.selectedFiles,
            ),
            actions = ChatComposerActions(
                onDraftChange,
                { onSend(draft) },
                viewModel::cancel,
                viewModel::openFilePicker,
                viewModel::removeFileAttachment,
            ),
        )
    }
}

@Composable
private fun ChatContent(contentState: ChatContentState, modifier: Modifier) {
    Box(modifier.fillMaxSize()) {
        LazyColumn(
            state = contentState.listState,
            contentPadding = PaddingValues(vertical = 12.dp),
            verticalArrangement = Arrangement.spacedBy(3.dp),
            reverseLayout = true,
            modifier = Modifier.fillMaxSize(),
        ) {
            items(
                items = contentState.rows.reversed(),
                key = ChatRow::key,
                contentType = ChatRow::contentType,
            ) { row ->
                when (row) {
                    is ChatRow.Block -> {
                        ChatBlockItem(row.block, contentState.viewModel, row.streaming)
                    }
                }
            }
        }
        if (contentState.farFromBottom) {
            ScrollToBottomButton(contentState.onScrollToBottom)
        }
    }
}

@Composable
private fun BoxScope.ScrollToBottomButton(onClick: () -> Unit) {
    FilledIconButton(
        onClick = onClick,
        colors = IconButtonDefaults.filledIconButtonColors(
            containerColor = MaterialTheme.colorScheme.surfaceVariant,
            contentColor = MaterialTheme.colorScheme.onSurface,
        ),
        modifier = Modifier
            .align(Alignment.BottomEnd)
            .padding(14.dp)
            .size(38.dp),
    ) {
        Icon(Icons.Default.ArrowDownward, "滚动到底部")
    }
}

@Composable
private fun ChatFilePicker(state: ChatUiState, viewModel: ChatViewModel) {
    if (state.filePickerVisible) {
        WorkspaceFilePickerDialog(
            state,
            viewModel::closeFilePicker,
            viewModel::browseWorkspace,
            viewModel::toggleFileAttachment,
        )
    }
}

private fun activeTool(state: ChatUiState): ChatBlock? = state.blocks.lastOrNull { block ->
    block.kind == ChatBlockKind.TOOL &&
        block.toolState in setOf(ToolRunState.PENDING, ToolRunState.RUNNING)
}

private fun isFarFromBottom(listState: LazyListState, threshold: Int): Boolean =
    listState.firstVisibleItemIndex != 0 || listState.firstVisibleItemScrollOffset > threshold

private fun buildChatRows(blocks: List<ChatBlock>, busy: Boolean): List<ChatRow> {
    val liveAssistantId = if (busy) {
        blocks.lastOrNull { it.kind == ChatBlockKind.ASSISTANT }?.id
    } else {
        null
    }
    return blocks.map { block -> ChatRow.Block(block, block.id == liveAssistantId) }
}
