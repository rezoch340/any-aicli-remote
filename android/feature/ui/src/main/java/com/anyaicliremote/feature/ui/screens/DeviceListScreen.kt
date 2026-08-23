package com.anyaicliremote.feature.ui.screens

import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Add
import androidx.compose.material.icons.filled.Computer
import androidx.compose.material.icons.filled.QrCodeScanner
import androidx.compose.material.icons.filled.Refresh
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.FloatingActionButton
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.TopAppBar
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.role
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import com.anyaicliremote.core.model.ConnectionStatus
import com.anyaicliremote.core.model.DeviceHealthStatus
import com.anyaicliremote.core.model.SavedDevice
import com.anyaicliremote.feature.ui.ChatUiState
import com.anyaicliremote.feature.ui.ChatViewModel
import com.anyaicliremote.feature.ui.theme.AnyAIColors

internal data class DeviceCardActions(
    val onConnect: () -> Unit,
    val onEdit: () -> Unit,
    val onDelete: () -> Unit,
)

internal data class DeviceListContentState(
    val state: ChatUiState,
    val managementEnabled: Boolean,
)

internal data class DeviceListContentActions(
    val onAddDevice: () -> Unit,
    val onScanPairingCode: () -> Unit,
    val onConnectDevice: (SavedDevice) -> Unit,
    val onEditDevice: (SavedDevice) -> Unit,
    val onDeleteDevice: (SavedDevice) -> Unit,
)


@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun DeviceListScreen(
    state: ChatUiState,
    viewModel: ChatViewModel,
    onScanPairingCode: () -> Unit = {},
) {
    var pendingDeletion by remember { mutableStateOf<SavedDevice?>(null) }
    val managementEnabled = state.connection != ConnectionStatus.CONNECTING
    DeviceHealthMonitor(state, viewModel)
    DeviceListScaffold(
        contentState = DeviceListContentState(state, managementEnabled),
        viewModel = viewModel,
        onScanPairingCode = onScanPairingCode,
        onDelete = { pendingDeletion = it },
    )
    DeleteDeviceDialog(pendingDeletion, viewModel) { pendingDeletion = null }
}

@Composable
private fun DeviceHealthMonitor(state: ChatUiState, viewModel: ChatViewModel) {
    val healthMonitorKey = state.devices.map { it.id to it.baseUrl }
    DisposableEffect(healthMonitorKey) {
        viewModel.startDeviceHealthMonitoring()
        onDispose(viewModel::stopDeviceHealthMonitoring)
    }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun DeviceListScaffold(
    contentState: DeviceListContentState,
    viewModel: ChatViewModel,
    onScanPairingCode: () -> Unit,
    onDelete: (SavedDevice) -> Unit,
) {
    Scaffold(
        topBar = {
            DeviceListHeader(viewModel, contentState.managementEnabled)
        },
        floatingActionButton = {
            AddDeviceButton(
                contentState.managementEnabled,
                viewModel::beginAddDevice,
            )
        },
    ) { padding ->
        DeviceListContent(
            contentState = contentState,
            actions = DeviceListContentActions(
                onAddDevice = viewModel::beginAddDevice,
                onScanPairingCode = onScanPairingCode,
                onConnectDevice = { device -> viewModel.connectDevice(device.id) },
                onEditDevice = { device -> viewModel.beginEditDevice(device.id) },
                onDeleteDevice = onDelete,
            ),
            modifier = Modifier.padding(padding),
        )
    }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun DeviceListHeader(viewModel: ChatViewModel, managementEnabled: Boolean) {
    TopAppBar(
        title = {
            Column {
                Text("设备", fontWeight = FontWeight.SemiBold)
                Text(
                    text = "选择一台设备开始连接",
                    style = MaterialTheme.typography.labelSmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
        },
        actions = {
            IconButton(
                onClick = viewModel::refreshDeviceHealth,
                enabled = managementEnabled,
            ) {
                Icon(Icons.Default.Refresh, "刷新设备状态")
            }
        },
    )
}

@Composable
private fun AddDeviceButton(enabled: Boolean, onAdd: () -> Unit) {
    if (enabled) {
        FloatingActionButton(onClick = onAdd) {
            Icon(Icons.Default.Add, "添加设备")
        }
    }
}

@Composable
private fun DeviceListContent(
    contentState: DeviceListContentState,
    actions: DeviceListContentActions,
    modifier: Modifier,
) {
    val state = contentState.state
    LazyColumn(
        modifier = modifier.fillMaxSize(),
        contentPadding = PaddingValues(horizontal = 16.dp, vertical = 12.dp),
        verticalArrangement = Arrangement.spacedBy(12.dp),
    ) {
        item { PairingHint(actions.onScanPairingCode) }
        state.error?.let { error ->
            item { DeviceListError(error) }
        }
        if (state.devices.isEmpty()) {
            item { EmptyDeviceList(actions.onAddDevice) }
        } else {
            items(state.devices, key = SavedDevice::id) { device ->
                DeviceCard(
                    device = device,
                    healthStatus = state.deviceHealth[device.id]
                        ?: DeviceHealthStatus.CHECKING,
                    connecting = isConnecting(state, device),
                    enabled = contentState.managementEnabled,
                    actions = DeviceCardActions(
                        onConnect = { actions.onConnectDevice(device) },
                        onEdit = { actions.onEditDevice(device) },
                        onDelete = { actions.onDeleteDevice(device) },
                    ),
                )
            }
        }
        item { Spacer(Modifier.height(76.dp)) }
    }
}

@Composable
private fun DeviceListError(error: String) {
    Text(
        text = error,
        color = AnyAIColors.warning,
        style = MaterialTheme.typography.bodySmall,
        modifier = Modifier
            .fillMaxWidth()
            .background(AnyAIColors.errorContainer, RoundedCornerShape(12.dp))
            .padding(12.dp),
    )
}

@Composable
private fun DeleteDeviceDialog(
    device: SavedDevice?,
    viewModel: ChatViewModel,
    onDismiss: () -> Unit,
) {
    device?.let { selected ->
        AlertDialog(
            onDismissRequest = onDismiss,
            title = { Text("删除设备？") },
            text = { Text("将从此设备移除“${selected.name}”及其配对信息。") },
            confirmButton = {
                TextButton(
                    onClick = {
                        onDismiss()
                        viewModel.deleteDevice(selected.id)
                    },
                ) {
                    Text("删除", color = MaterialTheme.colorScheme.error)
                }
            },
            dismissButton = { TextButton(onClick = onDismiss) { Text("取消") } },
        )
    }
}


@Composable
private fun PairingHint(onScanPairingCode: () -> Unit) {
    Row(
        modifier = Modifier
            .fillMaxWidth()
            .clickable(onClick = onScanPairingCode)
            .semantics {
                role = Role.Button
                contentDescription = "扫码添加，点按打开相机扫描"
            }
            .background(
                MaterialTheme.colorScheme.surfaceVariant,
                RoundedCornerShape(16.dp),
            )
            .padding(14.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Icon(
            Icons.Default.QrCodeScanner,
            null,
            tint = MaterialTheme.colorScheme.onSurfaceVariant,
        )
        Spacer(Modifier.width(12.dp))
        Column {
            Text("扫码添加", fontWeight = FontWeight.Medium)
            Text(
                text = "点按打开相机扫描 Mac 启动器中的二维码，设备会自动保存到这里。",
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
    }
}

@Composable
private fun EmptyDeviceList(onAdd: () -> Unit) {
    Column(
        modifier = Modifier.fillMaxWidth().padding(vertical = 48.dp),
        horizontalAlignment = Alignment.CenterHorizontally,
    ) {
        Icon(
            Icons.Default.Computer,
            null,
            modifier = Modifier.size(54.dp),
            tint = MaterialTheme.colorScheme.onSurfaceVariant,
        )
        Spacer(Modifier.height(12.dp))
        Text("还没有设备", style = MaterialTheme.typography.titleMedium)
        TextButton(onClick = onAdd) { Text("手动添加") }
    }
}


