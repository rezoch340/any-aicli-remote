package com.anyaicliremote.feature.ui

import com.anyaicliremote.core.model.ModelSwitch
import com.anyaicliremote.core.model.RetryPhase
import com.anyaicliremote.core.model.RetryStatus
import com.anyaicliremote.core.model.SessionStatusUpdate

/** Renders a neutral session status update into a short human notice. */
internal object SessionStatusFormatter {
    fun notice(status: SessionStatusUpdate): String {
        status.retry?.let { return retryNotice(it) }
        status.modelSwitch?.let { return modelSwitchNotice(it) }
        return ""
    }

    private fun retryNotice(retry: RetryStatus): String = when (retry.phase) {
        RetryPhase.RETRYING -> {
            val progress = if (retry.maxRetries > 0) "${retry.attempt}/${retry.maxRetries}" else "${retry.attempt}"
            "正在重试 $progress${reasonSuffix(retry.reason)}"
        }
        RetryPhase.EXHAUSTED -> if (retry.rateLimit) "已达速率上限，重试耗尽" else "重试已耗尽${reasonSuffix(retry.reason)}"
        RetryPhase.FAILED -> "请求失败${reasonSuffix(retry.reason)}"
    }

    private fun modelSwitchNotice(switch: ModelSwitch): String {
        val target = switch.current.ifEmpty { "其他模型" }
        return "已自动切换到 $target${reasonSuffix(switch.reason)}"
    }

    private fun reasonSuffix(reason: String): String = if (reason.isBlank()) "" else "：${reason.trim()}"
}
