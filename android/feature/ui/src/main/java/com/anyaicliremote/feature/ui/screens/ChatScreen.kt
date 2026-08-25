package com.anyaicliremote.feature.ui.screens

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.imePadding
import androidx.compose.foundation.layout.navigationBarsPadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.LazyListState
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.lazy.rememberLazyListState
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Scaffold
import androidx.compose.runtime.Composable
import androidx.compose.runtime.derivedStateOf
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.unit.dp
import com.anyaicliremote.core.model.ChatBlock
import com.anyaicliremote.core.model.ChatBlockKind
import com.anyaicliremote.core.model.ToolRunState
import com.anyaicliremote.feature.ui.ChatUiState
import com.anyaicliremote.feature.ui.ChatViewModel
import com.anyaicliremote.feature.ui.components.ChatBlockItem
import com.anyaicliremote.feature.ui.components.InteractionSheetActions
import com.anyaicliremote.feature.ui.components.InteractionSheet
import com.anyaicliremote.feature.ui.components.ChildAgentStrip
import com.anyaicliremote.feature.ui.components.SessionStatusBar
import com.anyaicliremote.feature.ui.components.ChatComposer
import com.anyaicliremote.feature.ui.components.ChatComposerActions
import com.anyaicliremote.feature.ui.components.ChatComposerState
import com.anyaicliremote.feature.ui.components.FloatingToolStatus
import com.anyaicliremote.feature.ui.components.WorkspaceFilePickerDialog
import kotlinx.coroutines.launch

internal data class ChatEffectsState(
    val state: ChatUiState,
    val sessionId: String,
    val listState: LazyListState,
    val threshold: Int,
    val hasRows: Boolean,
    val follow: Boolean,
    val setFollow: (Boolean) -> Unit,
)

internal data class ChatContentState(
    val viewModel: ChatViewModel,
    val listState: LazyListState,
    val rows: List<ChatRow>,
    val farFromBottom: Boolean,
    val childAgents: List<com.anyaicliremote.core.model.ChildAgentCard>,
    val sessionMode: String,
    val sessionNotice: String,
    val onScrollToBottom: () -> Unit,
)

internal data class ChatScaffoldState(
    val state: ChatUiState,
    val sessionTitle: String,
    val content: ChatContentState,
    val draft: String,
)

internal data class ChatScaffoldActions(
    val onDraftChange: (String) -> Unit,
    val onSend: (String) -> Unit,
)

internal sealed interface ChatRow {
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
        childAgents = state.childAgents,
        sessionMode = state.sessionMode,
        sessionNotice = state.sessionNotice,
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
    InteractionSheet(
        interaction = state.pendingInteraction,
        actions = InteractionSheetActions(
            onAnswer = viewModel::answerInteraction,
            onDismiss = viewModel::dismissInteraction,
        ),
    )
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
        androidx.compose.foundation.layout.Column(
            modifier = androidx.compose.ui.Modifier.align(androidx.compose.ui.Alignment.TopCenter),
        ) {
            SessionStatusBar(contentState.sessionMode, contentState.sessionNotice)
            ChildAgentStrip(contentState.childAgents)
        }
        if (contentState.farFromBottom) {
            ScrollToBottomButton(contentState.onScrollToBottom)
        }
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


private fun buildChatRows(blocks: List<ChatBlock>, busy: Boolean): List<ChatRow> {
    val liveAssistantId = if (busy) {
        blocks.lastOrNull { it.kind == ChatBlockKind.ASSISTANT }?.id
    } else {
        null
    }
    return blocks.map { block -> ChatRow.Block(block, block.id == liveAssistantId) }
}

