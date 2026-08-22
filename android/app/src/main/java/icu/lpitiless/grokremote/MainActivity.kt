package icu.lpitiless.grokremote

import android.content.Intent
import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.activity.enableEdgeToEdge
import androidx.activity.viewModels
import androidx.compose.runtime.getValue
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import icu.lpitiless.grokremote.ui.ChatViewModel
import icu.lpitiless.grokremote.ui.GrokRemoteRoot
import icu.lpitiless.grokremote.ui.theme.GrokRemoteTheme

class MainActivity : ComponentActivity() {
    private val viewModel: ChatViewModel by viewModels()

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        enableEdgeToEdge()
        handlePairingIntent(intent)
        setContent {
            GrokRemoteTheme {
                val state by viewModel.state.collectAsStateWithLifecycle()
                GrokRemoteRoot(state, viewModel)
            }
        }
    }

    override fun onNewIntent(intent: Intent) {
        super.onNewIntent(intent)
        setIntent(intent)
        handlePairingIntent(intent)
    }

    private fun handlePairingIntent(intent: Intent?) {
        val data = intent?.data ?: return
        if (data.scheme != "grokremote" || data.host != "pair") return
        val address = data.getQueryParameter("url") ?: return
        val key = data.getQueryParameter("key").orEmpty()
        val cwd = data.getQueryParameter("cwd") ?: viewModel.state.value.defaultCwd
        viewModel.updatePairing(address = address, key = key, cwd = cwd)
        viewModel.connect(address, key, cwd)
    }
}
