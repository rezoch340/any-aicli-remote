package com.anyaicliremote.feature.ui.screens

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.material.icons.filled.CheckCircle
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.TopAppBar
import androidx.compose.material3.TopAppBarDefaults
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import com.anyaicliremote.feature.ui.ChatUiState
import com.anyaicliremote.feature.ui.ChatViewModel

private const val MODEL_INDICATOR_SIZE = 5
private const val MODEL_INDICATOR_COLOR = 0xFF69C48A

@OptIn(ExperimentalMaterial3Api::class)
@Composable
internal fun ChatTopBar(
    state: ChatUiState,
    viewModel: ChatViewModel,
    sessionTitle: String,
) {
    var effortMenu by remember { mutableStateOf(false) }
    TopAppBar(
        navigationIcon = {
            IconButton(onClick = viewModel::closeSession) {
                Icon(Icons.AutoMirrored.Filled.ArrowBack, "返回")
            }
        },
        title = { ChatTitle(sessionTitle, state.model.currentModelId) },
        actions = {
            EffortMenu(state, viewModel, effortMenu) { effortMenu = it }
        },
        colors = TopAppBarDefaults.topAppBarColors(
            containerColor = MaterialTheme.colorScheme.background,
        ),
    )
}

@Composable
private fun ChatTitle(title: String, modelId: String) {
    Column {
        Text(
            text = title,
            maxLines = 1,
            overflow = TextOverflow.Ellipsis,
            style = MaterialTheme.typography.titleSmall,
            fontWeight = FontWeight.SemiBold,
        )
        Row(verticalAlignment = Alignment.CenterVertically) {
            Box(
                Modifier
                    .size(MODEL_INDICATOR_SIZE.dp)
                    .clip(CircleShape)
                    .background(Color(MODEL_INDICATOR_COLOR)),
            )
            Spacer(Modifier.width(6.dp))
            Text(
                text = modelId.ifBlank { "自动" },
                fontFamily = FontFamily.Monospace,
                style = MaterialTheme.typography.labelSmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
    }
}

@Composable
private fun EffortMenu(
    state: ChatUiState,
    viewModel: ChatViewModel,
    expanded: Boolean,
    setExpanded: (Boolean) -> Unit,
) {
    Box {
        TextButton(onClick = { setExpanded(true) }) {
            Text(
                text = state.model.effort.uppercase(),
                style = MaterialTheme.typography.labelSmall,
                fontWeight = FontWeight.SemiBold,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
        DropdownMenu(
            expanded = expanded,
            onDismissRequest = { setExpanded(false) },
        ) {
            state.model.effortLevels.forEach { effort ->
                DropdownMenuItem(
                    text = { Text(effort) },
                    trailingIcon = {
                        if (effort == state.model.effort) {
                            Icon(
                                Icons.Default.CheckCircle,
                                null,
                                tint = MaterialTheme.colorScheme.primary,
                            )
                        }
                    },
                    onClick = {
                        setExpanded(false)
                        viewModel.setEffort(effort)
                    },
                )
            }
        }
    }
}


