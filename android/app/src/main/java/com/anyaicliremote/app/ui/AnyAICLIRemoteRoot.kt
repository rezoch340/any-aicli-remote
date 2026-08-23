package com.anyaicliremote.app.ui

import androidx.activity.compose.BackHandler
import androidx.compose.runtime.Composable
import com.anyaicliremote.feature.ui.AppDestination
import com.anyaicliremote.feature.ui.ChatUiState
import com.anyaicliremote.feature.ui.ChatViewModel
import com.anyaicliremote.feature.ui.screens.ChatScreen
import com.anyaicliremote.feature.ui.screens.DeviceListScreen
import com.anyaicliremote.feature.ui.screens.PairingScreen
import com.anyaicliremote.feature.ui.screens.SessionListScreen

@Composable
internal fun AnyAICLIRemoteRoot(
    state: ChatUiState,
    viewModel: ChatViewModel,
    onScanPairingCode: () -> Unit = {},
) {
    when (state.destination) {
        AppDestination.DEVICES -> {
            DeviceListScreen(state, viewModel, onScanPairingCode)
        }
        AppDestination.PAIRING -> {
            BackHandler { viewModel.cancelPairing() }
            PairingScreen(state, viewModel)
        }
        AppDestination.SESSIONS -> {
            BackHandler { viewModel.disconnect() }
            SessionListScreen(state, viewModel)
        }
        AppDestination.CHAT -> if (state.selectedSession != null) {
            BackHandler { viewModel.closeSession() }
            ChatScreen(state, viewModel)
        } else {
            BackHandler { viewModel.disconnect() }
            SessionListScreen(state, viewModel)
        }
    }
}
