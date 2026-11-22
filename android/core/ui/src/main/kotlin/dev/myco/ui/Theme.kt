package dev.myco.ui

import androidx.compose.foundation.Canvas
import androidx.compose.foundation.background
import androidx.compose.foundation.isSystemInDarkTheme
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.material3.*
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.geometry.Size
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.LocalConfiguration
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.unit.dp

@Composable
fun words(zh: String, en: String): String =
    if (LocalConfiguration.current.locales[0].language == "zh") zh else en

@Composable
fun MycoTheme(dark: Boolean = isSystemInDarkTheme(), content: @Composable () -> Unit) {
    val colors =
        if (dark)
            darkColorScheme(
                primary = Color(0xffeeece5),
                onPrimary = Color(0xff171816),
                background = Color(0xff141513),
                surface = Color(0xff1c1d1a),
                surfaceContainer = Color(0xff242520),
                secondaryContainer = Color(0xff2b2d26),
                onSecondaryContainer = Color(0xffeae8df),
                outlineVariant = Color(0xff34352f),
            )
        else
            lightColorScheme(
                primary = Color(0xff252821),
                onPrimary = Color(0xfffaf9f4),
                background = Color(0xfff7f7f2),
                surface = Color(0xfffdfdf9),
                surfaceContainer = Color(0xffefefe8),
                secondaryContainer = Color(0xffeaece3),
                onSecondaryContainer = Color(0xff252821),
                outlineVariant = Color(0xffe4e5dd),
            )
    MaterialTheme(colorScheme = colors, content = content)
}

/** Deterministic pixel mushroom; no network fetch is needed to draw a member. */
@Composable
fun Mushroom(seed: String, modifier: Modifier = Modifier, working: Boolean = false) {
    val hash = seed.hashCode().toUInt().toLong()
    val ink =
        listOf(
            Color(0xff719076),
            Color(0xffa5816c),
            Color(0xff8885a0),
            Color(0xff78949b),
            Color(0xffa18b54),
        )[(hash % 5).toInt()]
    Canvas(
        modifier
            .size(44.dp)
            .background(MaterialTheme.colorScheme.surfaceContainer, CircleShape)
            .semantics { contentDescription = seed }
    ) {
        val u = size.minDimension / 14f
        fun rect(x: Int, y: Int, w: Int, h: Int, color: Color) =
            drawRect(color, Offset(x * u, y * u), Size(w * u, h * u))
        rect(5, 3, 4, 1, ink)
        rect(3, 4, 8, 1, ink)
        rect(2, 5, 10, 3, ink)
        rect(3, 8, 8, 1, ink)
        rect(5, 9, 4, 3, Color(0xffc7bca4))
        rect(5, 9, 1, 1, Color(0xff393c32))
        rect(8, 9, 1, 1, Color(0xff393c32))
        rect(4, 5, 2, 1, Color(0xfff0e9d8))
        rect(8, 6, 2, 1, Color(0xfff0e9d8))
        if (working) drawCircle(Color(0xff6c987b), 2 * u, Offset(12 * u, 12 * u))
    }
}

@Composable
fun Empty(text: String, detail: String = "", modifier: Modifier = Modifier) {
    Column(
        modifier.fillMaxSize().padding(32.dp),
        verticalArrangement = Arrangement.Center,
        horizontalAlignment = androidx.compose.ui.Alignment.CenterHorizontally,
    ) {
        Mushroom("myco", Modifier.size(64.dp))
        Spacer(Modifier.height(20.dp))
        Text(text, style = MaterialTheme.typography.titleMedium)
        if (detail.isNotEmpty()) {
            Spacer(Modifier.height(8.dp))
            Text(
                detail,
                style = MaterialTheme.typography.bodyMedium,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
    }
}

@Composable
fun Problem(text: String, retry: (() -> Unit)? = null) {
    Surface(color = MaterialTheme.colorScheme.errorContainer, modifier = Modifier.fillMaxWidth()) {
        Row(
            Modifier.padding(horizontal = 16.dp, vertical = 8.dp),
            verticalAlignment = androidx.compose.ui.Alignment.CenterVertically,
        ) {
            Text(text, Modifier.weight(1f), style = MaterialTheme.typography.bodySmall)
            if (retry != null) TextButton(onClick = retry) { Text(words("重试", "Retry")) }
        }
    }
}
