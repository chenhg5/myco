package dev.myco.android

import android.graphics.Bitmap
import androidx.activity.compose.setContent
import androidx.compose.runtime.*
import androidx.compose.ui.graphics.asAndroidBitmap
import androidx.compose.ui.test.*
import androidx.compose.ui.test.junit4.createAndroidComposeRule
import androidx.core.view.WindowCompat
import dev.myco.model.AppPreferences
import dev.myco.model.Workspace
import dev.myco.ui.MycoTheme
import dev.myco.workspace.SettingsScreen
import java.io.File
import org.junit.Rule
import org.junit.Test

/** Controlled, bilingual fixtures for release material; no connected workspace is shown. */
class SettingsScreenshots {
    @get:Rule val app = createAndroidComposeRule<MainActivity>()

    @Test
    fun capturePreferencesInBothLanguagesAndAppearances() {
        val preferences = mutableStateOf(AppPreferences("light", "zh"))
        app.runOnUiThread {
            app.activity.setContent {
                MycoTheme(preferences = preferences.value) {
                    SettingsScreen(
                        Workspace(
                            "media-kit",
                            if (preferences.value.language == "zh") "产品设计" else "Product design",
                            "https://workspace.example/w/design",
                        ),
                        preferences.value,
                        { change -> preferences.value = change(preferences.value) },
                        {},
                        {},
                        {},
                        {},
                    )
                }
            }
        }
        val folder = File(app.activity.getExternalFilesDir(null), "media-kit").apply { mkdirs() }
        for (lang in listOf("zh", "en")) for (theme in listOf("light", "dark")) {
            app.runOnUiThread {
                preferences.value = AppPreferences(theme, lang)
                WindowCompat.getInsetsController(app.activity.window, app.activity.window.decorView)
                    .apply {
                        isAppearanceLightStatusBars = theme != "dark"
                        isAppearanceLightNavigationBars = theme != "dark"
                    }
            }
            app.waitForIdle()
            val bitmap = app.onRoot().captureToImage().asAndroidBitmap()
            File(folder, "android-settings-$lang-$theme.png").outputStream().use {
                bitmap.compress(Bitmap.CompressFormat.PNG, 100, it)
            }
            val preview =
                Bitmap.createScaledBitmap(bitmap, bitmap.width / 3, bitmap.height / 3, true)
            File(folder, "android-settings-$lang-$theme-preview.webp").outputStream().use {
                @Suppress("DEPRECATION") preview.compress(Bitmap.CompressFormat.WEBP, 70, it)
            }
            preview.recycle()
        }
    }
}
