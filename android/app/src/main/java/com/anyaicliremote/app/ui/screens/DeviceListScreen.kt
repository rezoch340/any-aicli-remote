package com.anyaicliremote.app.ui.screens

import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
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
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.semantics.role
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import com.anyaicliremote.app.model.ConnectionStatus
import com.anyaicliremote.app.model.DeviceHealthStatus
import com.anyaicliremote.app.model.SavedDevice
import com.anyaicliremote.app.ui.ChatUiState
import com.anyaicliremote.app.ui.ChatViewModel

@OptIn(ExperimentalMaterial3Api::class)
@Composable
internal fun DeviceListScreen(state: ChatUiState, viewModel: ChatViewModel, onScanPairingCode: () -> Unit = {}) {
    var pendingDeletion by remember { mutableStateOf<SavedDevice?>(null) }
    val managementEnabled = state.connection != ConnectionStatus.CONNECTING
    val healthMonitorKey = state.devices.map { it.id to it.baseUrl }

    DisposableEffect(healthMonitorKey) {
        viewModel.startDeviceHealthMonitoring()
        onDispose(viewModel::stopDeviceHealthMonitoring)
    }

    Scaffold(
        topBar = {
            TopAppBar(
                title = {
                    Column {
                        Text("设备", fontWeight = FontWeight.SemiBold)
                        Text(
                            "选择一台设备开始连接",
                            style = MaterialTheme.typography.labelSmall,
                            color = MaterialTheme.colorScheme.onSurfaceVariant,
                        )
                    }
                },
                actions = {
                    IconButton(onClick = viewModel::refreshDeviceHealth, enabled = managementEnabled) {
                        Icon(Icons.Default.Refresh, "刷新设备状态")
                    }
                },
            )
        },
        floatingActionButton = {
            if (managementEnabled) {
                FloatingActionButton(onClick = viewModel::beginAddDevice) {
                    Icon(Icons.Default.Add, "添加设备")
                }
            }
        },
    ) { padding ->
        LazyColumn(
            modifier = Modifier.fillMaxSize().padding(padding),
            contentPadding = PaddingValues(horizontal = 16.dp, vertical = 12.dp),
            verticalArrangement = Arrangement.spacedBy(12.dp),
        ) {
            item {
                PairingHint(onScanPairingCode)
            }
            state.error?.let { error ->
                item {
                    Text(
                        text = error,
                        color = Color(0xFFFFB74D),
                        style = MaterialTheme.typography.bodySmall,
                        modifier = Modifier.fillMaxWidth()
                            .background(Color(0x22FF9800), RoundedCornerShape(12.dp))
                            .padding(12.dp),
                    )
                }
            }
            if (state.devices.isEmpty()) {
                item {
                    EmptyDeviceList(onAdd = viewModel::beginAddDevice)
                }
            } else {
                items(state.devices, key = SavedDevice::id) { device ->
                    DeviceCard(
                        device = device,
                        healthStatus = state.deviceHealth[device.id] ?: DeviceHealthStatus.CHECKING,
                        connecting = state.connection == ConnectionStatus.CONNECTING && state.activeDeviceId == device.id,
                        enabled = managementEnabled,
                        onConnect = { viewModel.connectDevice(device.id) },
                        onEdit = { viewModel.beginEditDevice(device.id) },
                        onDelete = { pendingDeletion = device },
                    )
                }
            }
            item { Spacer(Modifier.height(76.dp)) }
        }
    }

    pendingDeletion?.let { device ->
        AlertDialog(
            onDismissRequest = { pendingDeletion = null },
            title = { Text("删除设备？") },
            text = { Text("将从此设备移除“${device.name}”及其配对信息。") },
            confirmButton = {
                TextButton(onClick = {
                    pendingDeletion = null
                    viewModel.deleteDevice(device.id)
                }) { Text("删除", color = MaterialTheme.colorScheme.error) }
            },
            dismissButton = {
                TextButton(onClick = { pendingDeletion = null }) { Text("取消") }
            },
        )
    }
}

@Composable
private fun PairingHint(onScanPairingCode: () -> Unit) {
    Row(
        modifier = Modifier.fillMaxWidth().clickable(onClick = onScanPairingCode).semantics {
                role = Role.Button
                contentDescription = "扫码添加，点按打开相机扫描"
            }
            .background(MaterialTheme.colorScheme.surfaceVariant, RoundedCornerShape(16.dp))
            .padding(14.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Icon(Icons.Default.QrCodeScanner, null, tint = MaterialTheme.colorScheme.onSurfaceVariant)
        Spacer(Modifier.width(12.dp))
        Column {
            Text("扫码添加", fontWeight = FontWeight.Medium)
            Text(
                "点按打开相机扫描 Mac 启动器中的二维码，设备会自动保存到这里。",
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
    onConnect: () -> Unit,
    onEdit: () -> Unit,
    onDelete: () -> Unit,
) {
    var menuExpanded by remember { mutableStateOf(false) }
    val statusLabel: String
    val statusColor: Color
    when {
        connecting -> {
            statusLabel = "连接中"
            statusColor = Color(0xFFFFB74D)
        }
        healthStatus == DeviceHealthStatus.ONLINE -> {
            statusLabel = "在线"
            statusColor = Color(0xFF4ADE80)
        }
        healthStatus == DeviceHealthStatus.CHECKING -> {
            statusLabel = "检查中"
            statusColor = Color(0xFFFFB74D)
        }
        else -> {
            statusLabel = "离线"
            statusColor = MaterialTheme.colorScheme.onSurfaceVariant
        }
    }

    Card(
        modifier = Modifier.fillMaxWidth().clickable(enabled = enabled, onClick = onConnect),
        shape = RoundedCornerShape(18.dp),
        colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surface),
    ) {
        Row(
            modifier = Modifier.fillMaxWidth().padding(start = 16.dp, top = 16.dp, bottom = 16.dp, end = 6.dp),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            Box(
                modifier = Modifier.size(46.dp).clip(RoundedCornerShape(13.dp))
                    .background(MaterialTheme.colorScheme.surfaceVariant),
                contentAlignment = Alignment.Center,
            ) {
                if (connecting) {
                    CircularProgressIndicator(Modifier.size(22.dp), strokeWidth = 2.dp)
                } else {
                    Icon(Icons.Default.Computer, null, tint = MaterialTheme.colorScheme.onSurfaceVariant)
                }
            }
            Spacer(Modifier.width(14.dp))
            Column(Modifier.weight(1f)) {
                Text(device.name, fontWeight = FontWeight.SemiBold, maxLines = 1, overflow = TextOverflow.Ellipsis)
                Text(
                    device.baseUrl,
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                    maxLines = 1,
                    overflow = TextOverflow.Ellipsis,
                )
                Spacer(Modifier.height(5.dp))
                Row(verticalAlignment = Alignment.CenterVertically) {
                    Box(
                        Modifier.size(7.dp).clip(CircleShape)
                            .background(statusColor),
                    )
                    Spacer(Modifier.width(6.dp))
                    Text(
                        statusLabel,
                        style = MaterialTheme.typography.labelSmall,
                        color = statusColor,
                    )
                }
            }
            Box {
                IconButton(onClick = { menuExpanded = true }, enabled = enabled) {
                    Icon(Icons.Default.MoreVert, "设备菜单")
                }
                DropdownMenu(expanded = menuExpanded, onDismissRequest = { menuExpanded = false }) {
                    DropdownMenuItem(
                        text = { Text("编辑") },
                        leadingIcon = { Icon(Icons.Default.Edit, null) },
                        enabled = enabled,
                        onClick = { menuExpanded = false; onEdit() },
                    )
                    DropdownMenuItem(
                        text = { Text("删除") },
                        leadingIcon = { Icon(Icons.Default.Delete, null) },
                        enabled = enabled,
                        onClick = { menuExpanded = false; onDelete() },
                    )
                }
            }
        }
    }
}
