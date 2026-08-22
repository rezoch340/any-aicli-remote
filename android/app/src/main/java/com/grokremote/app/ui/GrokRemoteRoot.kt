package com.grokremote.app.ui

import androidx.activity.compose.BackHandler
import androidx.compose.runtime.Composable
import com.grokremote.app.model.ConnectionStatus
import com.grokremote.app.ui.screens.ChatScreen
import com.grokremote.app.ui.screens.PairingScreen
import com.grokremote.app.ui.screens.SessionListScreen

@Composable
internal fun GrokRemoteRoot(state: ChatUiState, viewModel: ChatViewModel) {
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
