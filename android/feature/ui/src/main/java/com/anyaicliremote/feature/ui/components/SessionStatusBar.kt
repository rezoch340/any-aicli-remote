package com.anyaicliremote.feature.ui.components

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp

/**
 * Thin transient bar: an interaction-mode badge (plan/normal/yolo) and a passing
 * status notice (retry / auto model switch). Both are display-only, driven by
 * neutral session state; empty values render nothing.
 */
@Composable
fun SessionStatusBar(mode: String, notice: String, modifier: Modifier = Modifier) {
    val badge = modeBadge(mode)
    if (badge.isEmpty() && notice.isEmpty()) return
    Row(
        modifier = modifier
            .fillMaxWidth()
            .padding(horizontal = 12.dp, vertical = 4.dp),
        horizontalArrangement = Arrangement.spacedBy(8.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        if (badge.isNotEmpty()) {
            Text(
                text = badge,
                style = MaterialTheme.typography.labelSmall,
                fontWeight = FontWeight.SemiBold,
                color = MaterialTheme.colorScheme.onPrimaryContainer,
                modifier = Modifier
                    .clip(RoundedCornerShape(6.dp))
                    .background(MaterialTheme.colorScheme.primaryContainer)
                    .padding(horizontal = 8.dp, vertical = 2.dp),
            )
        }
        if (notice.isNotEmpty()) {
            Text(
                text = notice,
                style = MaterialTheme.typography.labelSmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
    }
}

// Only surface a badge for modes worth flagging; plain "normal" needs no chrome.
private fun modeBadge(mode: String): String = when (mode.lowercase()) {
    "plan" -> "计划模式"
    "yolo" -> "YOLO 模式"
    "" , "normal", "default", "agent" -> ""
    else -> mode
}
