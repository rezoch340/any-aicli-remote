package icu.lpitiless.grokremote.ui

import androidx.activity.compose.BackHandler
import androidx.compose.runtime.Composable
import icu.lpitiless.grokremote.model.ConnectionStatus
import icu.lpitiless.grokremote.ui.screens.ChatScreen
import icu.lpitiless.grokremote.ui.screens.PairingScreen
import icu.lpitiless.grokremote.ui.screens.SessionListScreen

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
