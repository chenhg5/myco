package dev.myco.android

import androidx.compose.ui.test.*
import androidx.compose.ui.test.junit4.createAndroidComposeRule
import androidx.lifecycle.Lifecycle
import org.junit.Rule
import org.junit.Test

class ConversationJourneyTest {
    @get:Rule val app = createAndroidComposeRule<MainActivity>()

    @Test
    fun aConversationSendsMultilineTextAndKeepsAnUnsentDraftAcrossBackgrounding() {
        app.waitUntil(10_000) {
            app.onAllNodesWithText("Look around first").fetchSemanticsNodes().isNotEmpty()
        }
        app.onNodeWithText("Look around first").performClick()
        app.waitUntil(10_000) {
            app.onAllNodesWithText("Willow").fetchSemanticsNodes().isNotEmpty()
        }
        app.onNodeWithText("Willow").performClick()
        app.onNode(hasSetTextAction()).performTextInput("First line\nSecond line")
        app.onNodeWithContentDescription("Send").performClick()
        try {
            app.waitUntil(8_000) {
                app.onAllNodesWithText("First line\nSecond line").fetchSemanticsNodes().isNotEmpty()
            }
        } catch (error: Throwable) {
            println(app.onRoot().printToString())
            throw error
        }
        app.onNodeWithText("First line\nSecond line").assertIsDisplayed()
        app.onNode(hasSetTextAction()).performTextInput("Keep this thought")
        app.activityRule.scenario.moveToState(Lifecycle.State.CREATED)
        app.activityRule.scenario.moveToState(Lifecycle.State.RESUMED)
        app.onNode(hasSetTextAction()).assertTextContains("Keep this thought")
    }

    @Test
    fun joinFormShowsAReadableFailureForAnInvalidLink() {
        app.waitUntil(10_000) {
            app.onAllNodesWithText("Join a workspace").fetchSemanticsNodes().isNotEmpty()
        }
        app.onNodeWithText("Join a workspace").performClick()
        app.onNode(hasSetTextAction()).performTextInput("not a link")
        app.onNodeWithText("Join", useUnmergedTree = true).performClick()
        app.waitUntil(5_000) {
            app.onAllNodesWithText("Join", useUnmergedTree = true)
                .fetchSemanticsNodes()
                .isNotEmpty()
        }
        app.onNodeWithText("Use an add-device link from Settings").assertIsDisplayed()
    }
}
