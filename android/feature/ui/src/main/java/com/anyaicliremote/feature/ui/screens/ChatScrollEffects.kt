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


