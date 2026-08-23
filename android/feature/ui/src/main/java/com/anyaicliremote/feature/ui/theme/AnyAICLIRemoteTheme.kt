package com.anyaicliremote.feature.ui.theme

import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Typography
import androidx.compose.material3.darkColorScheme
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.unit.sp

internal object AnyAIMetrics {
    const val compactSpacing = 5
    const val standardSpacing = 12
    const val cardCornerRadius = 18
    const val indicatorSize = 7
    const val deviceIconSize = 46
    const val composerHorizontalPadding = 10
    const val composerVerticalPadding = 8
    const val composerCornerRadius = 20
    const val composerShadowElevation = 5
    const val attachmentWidth = 260
    const val messageMaxLines = 7
    const val messageMaxHeight = 148
    const val sendButtonSize = 34
    const val smallIconSize = 18
    const val assistantIconSize = 16
    const val markdownCodeSize = 12.5
    const val markdownCodeLineHeight = 18
    const val markdownInlineCodeSize = 13.5
    const val markdownHeadingOneSize = 21
    const val markdownHeadingOneLineHeight = 27
    const val markdownHeadingTwoSize = 18
    const val markdownHeadingTwoLineHeight = 24
    const val markdownHeadingThreeSize = 16
    const val markdownHeadingThreeLineHeight = 22
}

internal object AnyAIColors {
    private const val warningArgb = 0xFFFFB74D
    private const val onlineArgb = 0xFF4ADE80
    private const val errorContainerArgb = 0x22FF9800
    private const val composerSurfaceArgb = 0xFF191A1E
    private const val disabledContainerArgb = 0xFF303137
    private const val disabledContentArgb = 0xFF777981
    private const val errorArgb = 0xFFFF6B6B
    private const val assistantAccentArgb = 0xFFB8B49F
    private const val markdownTextArgb = 0xFFD6D7DA
    private const val linkArgb = 0xFFB8C7EA
    private const val codeBackgroundArgb = 0xFF141519
    private const val inlineCodeBackgroundArgb = 0xFF202126
    private const val tableBackgroundArgb = 0xFF121317
    private const val planBackgroundArgb = 0x221F7AE0

    val warning = Color(warningArgb)
    val online = Color(onlineArgb)
    val errorContainer = Color(errorContainerArgb)
    val composerSurface = Color(composerSurfaceArgb)
    val disabledContainer = Color(disabledContainerArgb)
    val disabledContent = Color(disabledContentArgb)
    val error = Color(errorArgb)
    val assistantAccent = Color(assistantAccentArgb)
    val markdownText = Color(markdownTextArgb)
    val link = Color(linkArgb)
    val codeBackground = Color(codeBackgroundArgb)
    val inlineCodeBackground = Color(inlineCodeBackgroundArgb)
    val tableBackground = Color(tableBackgroundArgb)
    val planBackground = Color(planBackgroundArgb)
    val primary = parseColor("#F1F1F2")
    val primaryText = parseColor("#101114")
    val primaryContainer = parseColor("#24252A")
    val primaryContainerText = parseColor("#EDEDEF")
    val secondaryText = parseColor("#C7C8CC")
    val background = parseColor("#090A0C")
    val surface = parseColor("#0E0F12")
    val surfaceVariant = parseColor("#1B1C20")
    val onSurfaceVariant = parseColor("#9B9DA4")
    val outline = parseColor("#303238")
}

@Composable
fun AnyAICLIRemoteTheme(content: @Composable () -> Unit) {
    val colors = darkColorScheme(
        primary = AnyAIColors.markdownText,
        onPrimary = AnyAIColors.composerSurface,
        primaryContainer = AnyAIColors.primaryContainer,
        onPrimaryContainer = AnyAIColors.markdownText,
        secondary = AnyAIColors.secondaryText,
        onSecondary = AnyAIColors.composerSurface,
        secondaryContainer = AnyAIColors.primaryContainer,
        onSecondaryContainer = AnyAIColors.markdownText,
        tertiary = AnyAIColors.secondaryText,
        onTertiary = AnyAIColors.composerSurface,
        tertiaryContainer = AnyAIColors.primaryContainer,
        onTertiaryContainer = AnyAIColors.markdownText,
        background = AnyAIColors.background,
        onBackground = AnyAIColors.markdownText,
        surface = AnyAIColors.surface,
        onSurface = AnyAIColors.markdownText,
        surfaceVariant = AnyAIColors.surfaceVariant,
        onSurfaceVariant = AnyAIColors.onSurfaceVariant,
        outline = AnyAIColors.outline,
    )
    val type = Typography(
        bodyLarge = MaterialTheme.typography.bodyLarge.copy(fontSize = 15.5.sp, lineHeight = 23.sp),
        bodyMedium = MaterialTheme.typography.bodyMedium.copy(fontSize = 14.5.sp, lineHeight = 21.sp),
        bodySmall = MaterialTheme.typography.bodySmall.copy(fontSize = 12.5.sp, lineHeight = 18.sp),
        titleMedium = MaterialTheme.typography.titleMedium.copy(fontSize = 16.sp, lineHeight = 21.sp),
        titleSmall = MaterialTheme.typography.titleSmall.copy(fontSize = 14.5.sp, lineHeight = 19.sp),
        labelSmall = MaterialTheme.typography.labelSmall.copy(fontSize = 11.sp, lineHeight = 14.sp),
    )
    MaterialTheme(colorScheme = colors, typography = type) {
        Surface(
            modifier = Modifier.fillMaxSize(),
            color = MaterialTheme.colorScheme.background,
            content = content,
        )
    }
}

private fun parseColor(value: String): Color = Color(android.graphics.Color.parseColor(value))
