package com.anyaicliremote.feature.ui

import com.anyaicliremote.core.model.ChatBlock
import com.anyaicliremote.core.model.ChildAgentCard
import com.anyaicliremote.core.model.ConnectionStatus
import com.anyaicliremote.core.model.DeviceHealthStatus
import com.anyaicliremote.core.model.ModelState
import com.anyaicliremote.core.model.PendingInteraction
import com.anyaicliremote.core.model.SavedDevice
import com.anyaicliremote.core.model.SessionSummary
import com.anyaicliremote.core.model.WorkspaceFile

data class ChatUiState(
    val destination: AppDestination = AppDestination.DEVICES,
    val connection: ConnectionStatus = ConnectionStatus.DISCONNECTED,
    val devices: List<SavedDevice> = emptyList(),
    val activeDeviceId: String? = null,
    val deviceHealth: Map<String, DeviceHealthStatus> = emptyMap(),
    val editingDeviceId: String? = null,
    val deviceName: String = "",
    val address: String = "",
    val pairingKey: String = "",
    val sessions: List<SessionSummary> = emptyList(),
    val selectedSession: SessionSummary? = null,
    val blocks: List<ChatBlock> = emptyList(),
    val busy: Boolean = false,
    val status: String = "",
    val error: String? = null,
    val model: ModelState = ModelState(),
    val selectedFiles: List<WorkspaceFile> = emptyList(),
    val filePickerVisible: Boolean = false,
    val filePickerPath: String = ".",
    val filePickerParent: String? = null,
    val filePickerDirectories: List<WorkspaceFile> = emptyList(),
    val filePickerFiles: List<WorkspaceFile> = emptyList(),
    val filePickerLoading: Boolean = false,
    val filePickerError: String? = null,
    val childAgents: List<ChildAgentCard> = emptyList(),
    val pendingInteraction: PendingInteraction? = null,
)

enum class AppDestination { DEVICES, PAIRING, SESSIONS, CHAT }
