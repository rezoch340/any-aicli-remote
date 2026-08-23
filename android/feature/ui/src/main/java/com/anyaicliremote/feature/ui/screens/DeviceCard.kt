package com.anyaicliremote.feature.ui.screens

import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.RowScope
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Computer
import androidx.compose.material.icons.filled.Delete
import androidx.compose.material.icons.filled.Edit
import androidx.compose.material.icons.filled.MoreVert
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
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
import com.anyaicliremote.core.model.ConnectionStatus
import com.anyaicliremote.core.model.DeviceHealthStatus
import com.anyaicliremote.core.model.SavedDevice
import com.anyaicliremote.feature.ui.ChatUiState
import com.anyaicliremote.feature.ui.theme.AnyAIColors
import com.anyaicliremote.feature.ui.theme.AnyAIMetrics

internal fun isConnecting(state: ChatUiState, device: SavedDevice): Boolean =
    state.connection == ConnectionStatus.CONNECTING && state.activeDeviceId == device.id


@Composable
internal fun DeviceCard(
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

