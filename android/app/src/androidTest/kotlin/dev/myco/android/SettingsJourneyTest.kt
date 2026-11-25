package dev.myco.android

import androidx.activity.compose.setContent
import androidx.compose.runtime.*
import androidx.compose.ui.test.*
import androidx.compose.ui.test.junit4.createAndroidComposeRule
import dev.myco.model.*
import dev.myco.storage.DeviceStore
import dev.myco.ui.MycoTheme
import dev.myco.workspace.SettingsScreen
import kotlinx.coroutines.*
import kotlinx.coroutines.flow.first
import org.junit.Assert.*
import org.junit.Rule
import org.junit.Test

class SettingsJourneyTest {
    @get:Rule val app = createAndroidComposeRule<MainActivity>()

    @Test
    fun themeAndLanguageApplyImmediatelyAndSurviveActivityRecreation() {
        val store = DeviceStore(app.activity.applicationContext)
        val original = runBlocking { store.preferences.first() }
        try {
            runBlocking { store.updatePreferences { AppPreferences() } }
            app.waitUntil(10_000) {
                app.onAllNodesWithText("Appearance and language").fetchSemanticsNodes().isNotEmpty()
            }
            app.onNodeWithText("Appearance and language").performScrollTo().performClick()
            app.onNodeWithTag("settings-theme-dark").performScrollTo().performClick()
            app.onNodeWithTag("settings-language-zh").performScrollTo().performClick()
            app.waitUntil(5_000) { app.onAllNodesWithText("设置").fetchSemanticsNodes().isNotEmpty() }
            app.onNodeWithTag("settings-theme-dark").assertIsSelected()
            app.activityRule.scenario.recreate()
            app.waitUntil(10_000) {
                app.onAllNodesWithText("设置").fetchSemanticsNodes().isNotEmpty()
            }
            app.onNodeWithTag("settings-theme-dark").assertIsSelected()
            app.onNodeWithTag("settings-language-zh").assertIsSelected()
            app.onNodeWithTag("settings-theme-light").performScrollTo().performClick()
            app.onNodeWithTag("settings-language-en").performScrollTo().performClick()
            app.waitUntil(5_000) {
                app.onAllNodesWithText("Settings").fetchSemanticsNodes().isNotEmpty()
            }
            assertEquals(
                AppPreferences("light", "en"),
                runBlocking { DeviceStore(app.activity).preferences.first() },
            )
            // Independent edits must merge, even when queued before either write has finished.
            runBlocking {
                coroutineScope {
                    launch { store.updatePreferences { it.copy(appearance = "dark") } }
                    launch { store.updatePreferences { it.copy(language = "zh") } }
                }
            }
            assertEquals(AppPreferences("dark", "zh"), runBlocking { store.preferences.first() })
        } finally {
            runBlocking { store.updatePreferences { original } }
        }
    }

    @Test
    fun renamingChangesOnlyTheLocalAliasAndKeepsTheSameCredentialAndSelection() {
        val store = DeviceStore(app.activity.applicationContext)
        val id = "alias-fixture"
        val workspace = Workspace(id, "Test workspace", "https://fixture.invalid/w/team")
        val oldSelected = runBlocking { store.selected() }
        val current = mutableStateOf(workspace)
        try {
            runBlocking {
                store.save(Credentials(workspace, "fixture-only"))
                store.select(id)
            }
            app.runOnUiThread {
                app.activity.setContent {
                    MycoTheme {
                        SettingsScreen(
                            current.value,
                            AppPreferences(),
                            {},
                            {},
                            {},
                            {},
                            { current.value = store.rename(id, it) },
                        )
                    }
                }
            }
            app.onNodeWithTag("workspace-rename").performScrollTo().performClick()
            app.onNodeWithTag("workspace-name").performTextReplacement("  我的工作区  ")
            app.onNodeWithText("Save").performClick()
            app.waitUntil(5_000) { current.value.name == "我的工作区" }
            val saved = runBlocking { DeviceStore(app.activity).credentials(id) }!!
            assertEquals("我的工作区", saved.workspace.name)
            assertEquals(workspace.url, saved.workspace.url)
            assertEquals(id, saved.workspace.id)
            assertEquals("fixture-only", saved.token)
            assertEquals(id, runBlocking { store.selected() })
        } finally {
            runBlocking {
                store.remove(id)
                store.select(oldSelected)
            }
        }
    }
}
