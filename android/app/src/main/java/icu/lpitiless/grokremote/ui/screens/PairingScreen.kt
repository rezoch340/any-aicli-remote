package icu.lpitiless.grokremote.ui.screens

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Bolt
import androidx.compose.material.icons.filled.Warning
import androidx.compose.material3.Button
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.input.PasswordVisualTransformation
import androidx.compose.ui.unit.dp
import icu.lpitiless.grokremote.model.ConnectionStatus
import icu.lpitiless.grokremote.ui.ChatUiState
import icu.lpitiless.grokremote.ui.ChatViewModel

@Composable
internal fun PairingScreen(state: ChatUiState, viewModel: ChatViewModel) {
    Column(
        modifier = Modifier.fillMaxSize().verticalScroll(rememberScrollState()).padding(24.dp),
        verticalArrangement = Arrangement.Center,
    ) {
        Icon(Icons.Default.Bolt, null, tint = MaterialTheme.colorScheme.primary, modifier = Modifier.size(58.dp))
        Spacer(Modifier.height(12.dp))
        Text("连接 Grok Remote", style = MaterialTheme.typography.headlineLarge, fontWeight = FontWeight.Bold)
        Text("粘贴 connect.url，或分别输入服务地址与配对 Key。", color = MaterialTheme.colorScheme.onSurfaceVariant)
        Spacer(Modifier.height(28.dp))
        OutlinedTextField(
            value = state.address,
            onValueChange = { viewModel.updatePairing(address = it) },
            label = { Text("服务地址") },
            placeholder = { Text("http://192.168.1.100:2421") },
            singleLine = true,
            modifier = Modifier.fillMaxWidth(),
        )
        Spacer(Modifier.height(12.dp))
        OutlinedTextField(
            value = state.pairingKey,
            onValueChange = { viewModel.updatePairing(key = it) },
            label = { Text("配对 Key") },
            visualTransformation = PasswordVisualTransformation(),
            singleLine = true,
            modifier = Modifier.fillMaxWidth(),
        )
        Spacer(Modifier.height(12.dp))
        OutlinedTextField(
            value = state.defaultCwd,
            onValueChange = { viewModel.updatePairing(cwd = it) },
            label = { Text("默认工作目录") },
            singleLine = true,
            modifier = Modifier.fillMaxWidth(),
        )
        Spacer(Modifier.height(20.dp))
        Button(
            onClick = { viewModel.connect() },
            enabled = state.connection != ConnectionStatus.CONNECTING,
            modifier = Modifier.fillMaxWidth().height(52.dp),
        ) {
            if (state.connection == ConnectionStatus.CONNECTING) {
                CircularProgressIndicator(modifier = Modifier.size(20.dp), strokeWidth = 2.dp)
                Spacer(Modifier.width(10.dp))
            }
            Text(if (state.connection == ConnectionStatus.CONNECTING) "连接中" else "连接")
        }
        state.error?.let {
            Spacer(Modifier.height(14.dp))
            Row(verticalAlignment = Alignment.CenterVertically) {
                Icon(Icons.Default.Warning, null, tint = Color(0xFFFFB74D))
                Spacer(Modifier.width(8.dp))
                Text(it, color = Color(0xFFFFB74D), style = MaterialTheme.typography.bodySmall)
            }
        }
    }
}
