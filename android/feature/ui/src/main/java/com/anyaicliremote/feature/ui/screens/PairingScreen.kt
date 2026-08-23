package com.anyaicliremote.feature.ui.screens

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
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.material.icons.filled.Computer
import androidx.compose.material.icons.filled.Warning
import androidx.compose.material3.Button
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Text
import androidx.compose.material3.TopAppBar
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.input.PasswordVisualTransformation
import androidx.compose.ui.unit.dp
import com.anyaicliremote.feature.ui.ChatUiState
import com.anyaicliremote.feature.ui.ChatViewModel

private const val FORM_ICON_SIZE = 52
private const val FORM_HORIZONTAL_PADDING = 24
private const val SAVE_BUTTON_HEIGHT = 52
private const val WARNING_COLOR = 0xFFFFB74D

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun PairingScreen(state: ChatUiState, viewModel: ChatViewModel) {
    Scaffold(
        topBar = {
            TopAppBar(
                title = { Text(if (state.editingDeviceId == null) "添加设备" else "编辑设备") },
                navigationIcon = {
                    IconButton(onClick = viewModel::cancelPairing) {
                        Icon(Icons.AutoMirrored.Filled.ArrowBack, "返回设备列表")
                    }
                },
            )
        },
    ) { padding ->
        Column(
            modifier = Modifier.fillMaxSize().padding(padding)
                .verticalScroll(rememberScrollState()).padding(FORM_HORIZONTAL_PADDING.dp),
            verticalArrangement = Arrangement.Center,
        ) {
            Icon(Icons.Default.Computer, null, tint = MaterialTheme.colorScheme.primary, modifier = Modifier.size(FORM_ICON_SIZE.dp))
            Spacer(Modifier.height(12.dp))
            Text("设备信息", style = MaterialTheme.typography.headlineLarge, fontWeight = FontWeight.Bold)
            Text("配对 Key 将加密保存在此设备中。", color = MaterialTheme.colorScheme.onSurfaceVariant)
            Spacer(Modifier.height(28.dp))
            OutlinedTextField(
                value = state.deviceName,
                onValueChange = { viewModel.updatePairing(name = it) },
                label = { Text("设备名称") },
                placeholder = { Text("例如：书房 Mac") },
                singleLine = true,
                modifier = Modifier.fillMaxWidth(),
            )
            Spacer(Modifier.height(12.dp))
            OutlinedTextField(
                value = state.address,
                onValueChange = { viewModel.updatePairing(address = it) },
                label = { Text("服务地址") },
                placeholder = { Text("http://mac.local:端口") },
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
            Spacer(Modifier.height(20.dp))
            Button(
                onClick = viewModel::savePairing,
                modifier = Modifier.fillMaxWidth().height(SAVE_BUTTON_HEIGHT.dp),
            ) {
                Text("保存设备")
            }
            state.error?.let {
                Spacer(Modifier.height(14.dp))
                Row(verticalAlignment = Alignment.CenterVertically) {
                    Icon(Icons.Default.Warning, null, tint = Color(WARNING_COLOR))
                    Spacer(Modifier.width(8.dp))
                    Text(it, color = Color(WARNING_COLOR), style = MaterialTheme.typography.bodySmall)
                }
            }
        }
    }
}
