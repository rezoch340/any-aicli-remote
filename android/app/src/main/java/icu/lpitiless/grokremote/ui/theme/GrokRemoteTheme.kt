package icu.lpitiless.grokremote.ui.theme

import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Typography
import androidx.compose.material3.darkColorScheme
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.unit.sp

@Composable
internal fun GrokRemoteTheme(content: @Composable () -> Unit) {
    val colors = darkColorScheme(
        primary = Color(0xFFF1F1F2),
        onPrimary = Color(0xFF101114),
        primaryContainer = Color(0xFF24252A),
        onPrimaryContainer = Color(0xFFEDEDEF),
        secondary = Color(0xFFC7C8CC),
        onSecondary = Color(0xFF101114),
        secondaryContainer = Color(0xFF24252A),
        onSecondaryContainer = Color(0xFFEDEDEF),
        tertiary = Color(0xFFC7C8CC),
        onTertiary = Color(0xFF101114),
        tertiaryContainer = Color(0xFF24252A),
        onTertiaryContainer = Color(0xFFEDEDEF),
        background = Color(0xFF090A0C),
        onBackground = Color(0xFFEDEDEF),
        surface = Color(0xFF0E0F12),
        onSurface = Color(0xFFEDEDEF),
        surfaceVariant = Color(0xFF1B1C20),
        onSurfaceVariant = Color(0xFF9B9DA4),
        outline = Color(0xFF303238),
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
