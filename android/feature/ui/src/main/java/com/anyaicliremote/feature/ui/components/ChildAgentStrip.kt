package com.anyaicliremote.feature.ui.components

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyRow
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import com.anyaicliremote.core.model.ChildAgentCard
import com.anyaicliremote.core.model.ChildAgentStatus

/**
 * Horizontal strip of live child-agent cards. Display-only: it shows structured
 * status the daemon supplies and never renders prompts or generated output.
 */
@Composable
fun ChildAgentStrip(cards: List<ChildAgentCard>, modifier: Modifier = Modifier) {
    if (cards.isEmpty()) return
    LazyRow(
        modifier = modifier.fillMaxWidth().padding(horizontal = 12.dp, vertical = 6.dp),
        horizontalArrangement = Arrangement.spacedBy(8.dp),
    ) {
        items(cards, key = { it.providerChildId }) { card -> ChildAgentCardItem(card) }
    }
}

@Composable
private fun ChildAgentCardItem(card: ChildAgentCard) {
    Column(
        Modifier
            .width(208.dp)
            .clip(RoundedCornerShape(12.dp))
            .background(MaterialTheme.colorScheme.surfaceVariant)
            .padding(12.dp),
    ) {
        Row(verticalAlignment = Alignment.CenterVertically) {
            StatusDot(card.status)
            Spacer(Modifier.width(6.dp))
            Text(
                text = card.agentType.ifEmpty { "子 Agent" },
                style = MaterialTheme.typography.labelLarge,
                fontWeight = FontWeight.SemiBold,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis,
            )
            Spacer(Modifier.width(6.dp))
            Text(
                text = statusLabel(card.status),
                style = MaterialTheme.typography.labelSmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
        if (card.description.isNotEmpty()) {
            Text(
                text = card.description,
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
                maxLines = 2,
                overflow = TextOverflow.Ellipsis,
                modifier = Modifier.padding(top = 4.dp),
            )
        }
        Text(
            text = metricsLine(card),
            style = MaterialTheme.typography.labelSmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
            modifier = Modifier.padding(top = 4.dp),
        )
    }
}

@Composable
private fun StatusDot(status: ChildAgentStatus) {
    Column(
        Modifier
            .size(8.dp)
            .clip(CircleShape)
            .background(statusColor(status)),
    ) {}
}

private fun statusLabel(status: ChildAgentStatus): String = when (status) {
    ChildAgentStatus.RUNNING -> "运行中"
    ChildAgentStatus.COMPLETED -> "已完成"
    ChildAgentStatus.FAILED -> "失败"
    ChildAgentStatus.CANCELLED -> "已取消"
    ChildAgentStatus.UNKNOWN -> "未知"
}

private val runningColor = Color(0xFF69C48A)
private val completedColor = Color(0xFF4F9DDE)
private val failedColor = Color(0xFFE06666)
private val idleColor = Color(0xFF9AA0A6)

private fun statusColor(status: ChildAgentStatus): Color = when (status) {
    ChildAgentStatus.RUNNING -> runningColor
    ChildAgentStatus.COMPLETED -> completedColor
    ChildAgentStatus.FAILED -> failedColor
    ChildAgentStatus.CANCELLED -> idleColor
    ChildAgentStatus.UNKNOWN -> idleColor
}

private fun metricsLine(card: ChildAgentCard): String {
    val parts = mutableListOf<String>()
    if (card.turnCount > 0) parts.add("${card.turnCount} 轮")
    if (card.toolCallCount > 0) parts.add("${card.toolCallCount} 次工具")
    if (card.tokensUsed > 0) parts.add("${card.tokensUsed} tokens")
    return parts.joinToString(" · ").ifEmpty { "等待中" }
}
