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
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp
import com.anyaicliremote.core.model.ChatBlockKind

@Composable
internal fun ChatEffects(effectState: ChatEffectsState) {
    TrackTranscriptDrag(effectState)
    PinTranscriptToBottom(effectState)
}

@Composable
private fun TrackTranscriptDrag(effectState: ChatEffectsState) {
    val listState = effectState.listState
    LaunchedEffect(listState) {
        listState.interactionSource.interactions.collect { interaction ->
            applyTranscriptDrag(interaction, listState, effectState.threshold, effectState.setFollow)
        }
    }
}

private fun applyTranscriptDrag(
    interaction: androidx.compose.foundation.interaction.Interaction,
    listState: LazyListState,
    threshold: Int,
    setFollow: (Boolean) -> Unit,
) {
    when (interaction) {
        is DragInteraction.Start -> setFollow(false)
        is DragInteraction.Stop, is DragInteraction.Cancel -> {
            if (!isFarFromBottom(listState, threshold)) setFollow(true)
        }
    }
}

@Composable
private fun PinTranscriptToBottom(effectState: ChatEffectsState) {
    val newestBlock = effectState.state.blocks.lastOrNull()
    LaunchedEffect(
        effectState.sessionId,
        newestBlock?.id,
        newestBlock?.kind,
        newestBlock?.text?.length,
        newestBlock?.detail?.length,
        effectState.state.blocks.size,
        effectState.state.busy,
        effectState.follow,
    ) {
        if (newestBlock?.kind == ChatBlockKind.USER) effectState.setFollow(true)
        if (effectState.follow && effectState.hasRows && !effectState.listState.isScrollInProgress) {
            effectState.listState.scrollToItem(0)
        }
    }

    var wasBusy by remember(effectState.sessionId) { mutableStateOf(false) }
    LaunchedEffect(effectState.state.busy) {
        val streamCompleted = wasBusy && !effectState.state.busy
        wasBusy = effectState.state.busy
        if (streamCompleted && effectState.follow && effectState.hasRows) {
            effectState.listState.scrollToItem(0)
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
