package com.anyaicliremote.feature.ui.screens

import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.RowScope
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Add
import androidx.compose.material.icons.filled.Computer
import androidx.compose.material.icons.filled.Delete
import androidx.compose.material.icons.filled.Edit
import androidx.compose.material.icons.filled.MoreVert
import androidx.compose.material.icons.filled.QrCodeScanner
import androidx.compose.material.icons.filled.Refresh
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
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
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.role
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import com.anyaicliremote.core.model.ConnectionStatus
import com.anyaicliremote.core.model.DeviceHealthStatus
import com.anyaicliremote.core.model.SavedDevice
import com.anyaicliremote.feature.ui.ChatUiState
import com.anyaicliremote.feature.ui.ChatViewModel
import com.anyaicliremote.feature.ui.theme.AnyAIColors
import com.anyaicliremote.feature.ui.theme.AnyAIMetrics

private data class DeviceCardActions(
    val onConnect: () -> Unit,
    val onEdit: () -> Unit,
    val onDelete: () -> Unit,
)

private data class DeviceListContentState(
    val state: ChatUiState,
    val managementEnabled: Boolean,
)

private data class DeviceListContentActions(
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

private fun isConnecting(state: ChatUiState, device: SavedDevice): Boolean =
    state.connection == ConnectionStatus.CONNECTING && state.activeDeviceId == device.id

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

@Composable
private fun DeviceCard(
    device: SavedDevice,
    healthStatus: DeviceHealthStatus,
    connecting: Boolean,
    enabled: Boolean,
    actions: DeviceCardActions,
) {
    var menuExpanded by remember { mutableStateOf(false) }
    Card(
        modifier = Modifier
            .fillMaxWidth()
            .clickable(enabled = enabled, onClick = actions.onConnect),
        shape = RoundedCornerShape(AnyAIMetrics.cardCornerRadius.dp),
        colors = CardDefaults.cardColors(
            containerColor = MaterialTheme.colorScheme.surface,
        ),
    ) {
        Row(
            modifier = Modifier.fillMaxWidth().padding(
                start = 16.dp,
                top = 16.dp,
                bottom = 16.dp,
                end = 6.dp,
            ),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            DeviceIdentity(device, connecting, healthStatus)
            DeviceActions(enabled, menuExpanded, { menuExpanded = it }, actions)
        }
    }
}

@Composable
private fun RowScope.DeviceIdentity(
    device: SavedDevice,
    connecting: Boolean,
    healthStatus: DeviceHealthStatus,
) {
    DeviceIcon(connecting)
    Spacer(Modifier.width(14.dp))
    Column(Modifier.weight(1f)) {
        Text(
            device.name,
            fontWeight = FontWeight.SemiBold,
            maxLines = 1,
            overflow = TextOverflow.Ellipsis,
        )
        Text(
            device.baseUrl,
            style = MaterialTheme.typography.bodySmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
            maxLines = 1,
            overflow = TextOverflow.Ellipsis,
        )
        Spacer(Modifier.height(5.dp))
        DeviceStatus(connecting, healthStatus)
    }
}

@Composable
private fun DeviceIcon(connecting: Boolean) {
    Box(
        modifier = Modifier
            .size(AnyAIMetrics.deviceIconSize.dp)
            .clip(RoundedCornerShape(13.dp))
            .background(MaterialTheme.colorScheme.surfaceVariant),
        contentAlignment = Alignment.Center,
    ) {
        if (connecting) {
            CircularProgressIndicator(Modifier.size(22.dp), strokeWidth = 2.dp)
        } else {
            Icon(
                Icons.Default.Computer,
                null,
                tint = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
    }
}

@Composable
private fun DeviceStatus(connecting: Boolean, healthStatus: DeviceHealthStatus) {
    val (label, color) = deviceStatus(connecting, healthStatus)
    Row(verticalAlignment = Alignment.CenterVertically) {
        Box(
            Modifier
                .size(AnyAIMetrics.indicatorSize.dp)
                .clip(CircleShape)
                .background(color),
        )
        Spacer(Modifier.width(6.dp))
        Text(label, style = MaterialTheme.typography.labelSmall, color = color)
    }
}

@Composable
private fun DeviceActions(
    enabled: Boolean,
    expanded: Boolean,
    setExpanded: (Boolean) -> Unit,
    actions: DeviceCardActions,
) {
    Box {
        IconButton(onClick = { setExpanded(true) }, enabled = enabled) {
            Icon(Icons.Default.MoreVert, "设备菜单")
        }
        DropdownMenu(
            expanded = expanded,
            onDismissRequest = { setExpanded(false) },
        ) {
            DropdownMenuItem(
                text = { Text("编辑") },
                leadingIcon = { Icon(Icons.Default.Edit, null) },
                enabled = enabled,
                onClick = {
                    setExpanded(false)
                    actions.onEdit()
                },
            )
            DropdownMenuItem(
                text = { Text("删除") },
                leadingIcon = { Icon(Icons.Default.Delete, null) },
                enabled = enabled,
                onClick = {
                    setExpanded(false)
                    actions.onDelete()
                },
            )
        }
    }
}

@Composable
private fun deviceStatus(
    connecting: Boolean,
    healthStatus: DeviceHealthStatus,
): Pair<String, Color> = when {
    connecting -> "连接中" to AnyAIColors.warning
    healthStatus == DeviceHealthStatus.ONLINE -> "在线" to AnyAIColors.online
    healthStatus == DeviceHealthStatus.CHECKING -> "检查中" to AnyAIColors.warning
    else -> "离线" to MaterialTheme.colorScheme.onSurfaceVariant
}
