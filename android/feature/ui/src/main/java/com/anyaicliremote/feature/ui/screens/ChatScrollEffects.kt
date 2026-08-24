package com.anyaicliremote.feature.ui.screens

import androidx.compose.foundation.interaction.DragInteraction
import androidx.compose.foundation.layout.BoxScope
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.lazy.LazyListState
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.ArrowDownward
import androidx.compose.material3.FilledIconButton
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButtonDefaults
import androidx.compose.material3.MaterialTheme
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberUpdatedState
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp
import com.anyaicliremote.core.model.ChatBlock
import com.anyaicliremote.core.model.ChatBlockKind
import com.anyaicliremote.feature.ui.ChatUiState

@Composable
internal fun ChatEffects(effectState: ChatEffectsState) {
    FollowNewMessages(
        effectState = effectState,
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
    effectState: ChatEffectsState,
) {
    val newestBlock = effectState.state.blocks.lastOrNull()
    val latestRows by rememberUpdatedState(effectState.hasRows)
    val latestFollow by rememberUpdatedState(effectState.follow)
    LaunchedEffect(
        effectState.sessionId,
        newestBlock?.id,
        newestBlock?.kind,
        newestBlock?.text,
        newestBlock?.detail,
        newestBlock?.title,
        newestBlock?.toolState,
    ) {
        if (newestBlock?.kind == ChatBlockKind.USER) {
            effectState.setFollow(true)
            if (latestRows) effectState.listState.scrollToItem(0)
        } else if (shouldFollowLiveBlock(newestBlock, latestRows, latestFollow, effectState.listState)) {
            effectState.listState.scrollToItem(0)
        }
    }
}

private fun shouldFollowLiveBlock(
    newestBlock: ChatBlock?,
    hasRows: Boolean,
    follow: Boolean,
    listState: LazyListState,
): Boolean =
    newestBlock?.kind in liveBlockKinds && hasRows && follow && !listState.isScrollInProgress

private val liveBlockKinds = setOf(
    ChatBlockKind.ASSISTANT,
    ChatBlockKind.THINKING,
    ChatBlockKind.TOOL,
)

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


@Composable
internal fun BoxScope.ScrollToBottomButton(onClick: () -> Unit) {
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


internal fun isFarFromBottom(listState: LazyListState, threshold: Int): Boolean =
    listState.firstVisibleItemIndex != 0 || listState.firstVisibleItemScrollOffset > threshold
