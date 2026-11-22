package dev.myco.android

import android.app.Activity
import android.app.Instrumentation
import android.content.Intent
import android.text.Spanned
import android.text.style.ClickableSpan
import android.view.MotionEvent
import android.view.View
import android.view.ViewGroup
import android.webkit.WebView
import android.widget.TextView
import androidx.activity.compose.setContent
import androidx.compose.ui.test.*
import androidx.compose.ui.test.junit4.createAndroidComposeRule
import androidx.test.platform.app.InstrumentationRegistry
import dev.myco.model.Workspace
import dev.myco.network.HubClient
import dev.myco.ui.MycoTheme
import dev.myco.workspace.WorkspaceScreen
import java.util.concurrent.CopyOnWriteArrayList
import java.util.concurrent.CountDownLatch
import java.util.concurrent.TimeUnit
import kotlinx.serialization.json.*
import okhttp3.mockwebserver.*
import org.junit.Assert.*
import org.junit.Rule
import org.junit.Test

/** A real Markdown tap enters the existing host; further locations keep its document alive. */
class PageLinksJourneyTest {
    @get:Rule val app = createAndroidComposeRule<MainActivity>()

    @Test
    fun chatLinksUseTheExistingHostWithoutSendingFragmentsToTheHubOrReloading() {
        MockWebServer().use { server ->
            val calls = CopyOnWriteArrayList<RecordedRequest>()
            val base = server.url("/w/team").toString()
            val first =
                "watch?source=record&movie=film&line=%E4%B8%AD%E6%96%87&episode=%E7%AC%AC%E4%B8%80%E9%9B%86"
            val second =
                "watch?source=record&movie=film&line=%E4%B8%AD%E6%96%87&episode=%E7%AC%AC%E4%BA%8C%E9%9B%86"
            val pageUrl = "$base/pages/pg_movie"
            val ownUrl = "$pageUrl#watch?episode=own"
            val outside = "https://other.invalid/w/team/pages/pg_movie#watch"
            val ordinary = "https://example.invalid/help"
            val invalid = "$base/pages/pg_bad-id#watch"
            val message =
                "[Episode one]($pageUrl#$first) · [Episode two]($pageUrl#$second) · [Home]($pageUrl) · [Outside]($outside) · [Help]($ordinary) · [Invalid]($invalid) · Bare $pageUrl#$first"
            server.dispatcher =
                object : Dispatcher() {
                    override fun dispatch(request: RecordedRequest): MockResponse {
                        calls += request
                        val path = request.requestUrl!!.encodedPath.removePrefix("/w/team")
                        val body =
                            when {
                                path == "/v1/console/me" -> """{"member":{"name":"owner"}}"""
                                path == "/v1/overview" ->
                                    """{"agents":[{"agent":{"id":"agent","name":"agent","display_name":"Movie helper","online":true}}]}"""
                                path == "/v1/projects" -> """{"projects":[]}"""
                                path == "/v1/console/recent" -> """{"items":[]}"""
                                path == "/v1/console/hidden" -> """{"ids":[]}"""
                                path == "/v1/console/exchanges" -> buildJsonObject {
                                        put(
                                            "messages",
                                            buildJsonArray {
                                                add(
                                                    buildJsonObject {
                                                        put("id", "message")
                                                        put("from", "agent")
                                                        put("to", "@console:owner")
                                                        put("type", "note")
                                                        put("status", "done")
                                                        put("body", message)
                                                    }
                                                )
                                                add(
                                                    buildJsonObject {
                                                        put("id", "own")
                                                        put("from", "@console:owner")
                                                        put("to", "agent")
                                                        put("type", "task")
                                                        put("status", "done")
                                                        put("body", ownUrl)
                                                    }
                                                )
                                            },
                                        )
                                    }
                                        .toString()
                                path == "/v1/pages/pg_movie" -> buildJsonObject {
                                        put("id", "pg_movie")
                                        put("name", "Movie app")
                                        put("version", 1)
                                        put("publisher", "agent")
                                        put("url", pageUrl)
                                    }
                                        .toString()
                                path.endsWith("/state") -> """{"state":{},"version":1}"""
                                path == "/v1/pages/pg_movie/v/1" ->
                                    """
                                    <!doctype html><div id="status">Loading</div><script>
                                    window.documentToken = Math.random().toString(); window.changes = 0;
                                    window.addEventListener('hashchange', function() { window.changes++; });
                                    myco.ready.then(function(ctx) { window.context = ctx; document.getElementById('status').textContent = 'Ready'; });
                                    </script>
                                    """
                                        .trimIndent()
                                else -> "{}"
                            }
                        return MockResponse()
                            .setBody(body)
                            .setHeader("Content-Type", "application/json")
                    }
                }
            val hub = HubClient(base, "fixture-only")
            app.activity.runOnUiThread {
                app.activity.setContent {
                    MycoTheme {
                        WorkspaceScreen(
                            Workspace("workspace", "Workspace", base),
                            hub,
                            DemoStore(),
                            emptyList(),
                            {},
                            {},
                            {},
                            { _, _ -> },
                            {},
                        )
                    }
                }
            }
            app.waitUntil(15_000) {
                app.onAllNodesWithText("Movie helper").fetchSemanticsNodes().isNotEmpty()
            }
            app.onNodeWithText("Movie helper").performClick()
            app.waitUntil(10_000) { findMarkdown() != null }
            val markdown = findMarkdown()!!
            app.onNode(hasScrollAction()).performTouchInput { swipeDown() }
            app.waitForIdle()
            clickLink(markdown, "Episode one", touch = true)
            app.waitUntil(15_000) {
                javascript("document.getElementById('status')?.textContent") == "\"Ready\""
            }
            assertEquals("\"#$first\"", javascript("location.hash"))
            assertEquals(JsonPrimitive(pageUrl).toString(), javascript("window.context.page.url"))
            val document = javascript("window.documentToken")
            val web = findWeb()
            val beforeChange = javascript("window.changes").toInt()
            clickLink(markdown, "Episode two")
            app.waitUntil(5_000) {
                javascript("location.hash") == "\"#$second\"" &&
                    (javascript("window.changes").toIntOrNull() ?: 0) > beforeChange
            }
            val changes = javascript("window.changes").toInt()
            clickLink(markdown, "Episode two")
            app.waitUntil(5_000) { javascript("window.changes").toIntOrNull() == changes + 1 }
            clickLink(markdown, "Home")
            app.waitUntil(5_000) { javascript("location.hash") == "\"\"" }
            clickLink(markdown, "$pageUrl#$first")
            app.waitUntil(5_000) { javascript("location.hash") == "\"#$first\"" }
            assertSame(web, findWeb())
            assertEquals(document, javascript("window.documentToken"))
            assertEquals(
                1,
                calls.count { it.requestUrl?.encodedPath == "/w/team/v1/pages/pg_movie/v/1" },
            )
            assertFalse(
                calls.any {
                    it.path.orEmpty().contains("#") ||
                        it.path.orEmpty().contains("episode=") ||
                        it.path.orEmpty().contains("source=")
                }
            )
            assertTrue(
                calls
                    .filter { it.path.orEmpty().contains("/v1/pages/") }
                    .all { it.getHeader("Authorization") == "Bearer fixture-only" }
            )
            val external = CopyOnWriteArrayList<String>()
            val instrumentation = InstrumentationRegistry.getInstrumentation()
            val monitor =
                object : Instrumentation.ActivityMonitor() {
                    override fun onStartActivity(intent: Intent): Instrumentation.ActivityResult? {
                        if (intent.action != Intent.ACTION_VIEW) return null
                        external += intent.dataString.orEmpty()
                        return Instrumentation.ActivityResult(Activity.RESULT_CANCELED, null)
                    }
                }
            instrumentation.addMonitor(monitor)
            try {
                clickLink(markdown, "Outside")
                clickLink(markdown, "Help")
                clickLink(markdown, "Invalid")
                assertEquals(listOf(outside, ordinary, invalid), external.toList())
            } finally {
                instrumentation.removeMonitor(monitor)
            }
            app.onNodeWithContentDescription("Close").performClick()
            app.onAllNodesWithText(ownUrl).onFirst().performScrollTo().performClick()
            app.waitUntil(15_000) {
                javascript("document.getElementById('status')?.textContent") == "\"Ready\""
            }
            assertEquals("\"#watch?episode=own\"", javascript("location.hash"))
        }
    }

    private fun findMarkdown(): TextView? {
        var result: TextView? = null
        InstrumentationRegistry.getInstrumentation().runOnMainSync {
            walk(app.activity.window.decorView) {
                if (it is TextView && it.text.toString().contains("Episode one")) result = it
            }
        }
        return result
    }

    private fun findWeb(): WebView? {
        var result: WebView? = null
        InstrumentationRegistry.getInstrumentation().runOnMainSync {
            walk(app.activity.window.decorView) { if (it is WebView) result = it }
        }
        return result
    }

    private fun clickLink(view: TextView, label: String, touch: Boolean = false) {
        var x = 0f
        var y = 0f
        app.runOnUiThread {
            val text = view.text as Spanned
            val start = text.toString().indexOf(label)
            val span =
                text.getSpans(start, start + label.length, ClickableSpan::class.java).single()
            if (!touch) span.onClick(view)
            else {
                val layout = view.layout
                val line = layout.getLineForOffset(start)
                val position = IntArray(2)
                view.getLocationOnScreen(position)
                x = position[0] + layout.getPrimaryHorizontal(start + 1) + view.totalPaddingLeft
                y =
                    position[1] +
                        (layout.getLineTop(line) + layout.getLineBottom(line)) / 2f +
                        view.totalPaddingTop
            }
        }
        if (touch) {
            check(x > 0 && y > 0) { "Link is outside the viewport: $x, $y" }
            val now = android.os.SystemClock.uptimeMillis()
            listOf(MotionEvent.ACTION_DOWN, MotionEvent.ACTION_UP).forEach { action ->
                val event = MotionEvent.obtain(now, now, action, x, y, 0)
                event.source = android.view.InputDevice.SOURCE_TOUCHSCREEN
                InstrumentationRegistry.getInstrumentation().sendPointerSync(event)
                event.recycle()
            }
            app.waitForIdle()
        }
    }

    private fun javascript(script: String): String {
        val latch = CountDownLatch(1)
        var result = ""
        app.activity.runOnUiThread {
            var web: WebView? = null
            walk(app.activity.window.decorView) { if (it is WebView) web = it }
            web?.evaluateJavascript(script) {
                result = it
                latch.countDown()
            } ?: latch.countDown()
        }
        check(latch.await(5, TimeUnit.SECONDS))
        return result
    }

    private fun walk(view: View, visit: (View) -> Unit) {
        visit(view)
        if (view is ViewGroup) for (i in 0 until view.childCount) walk(view.getChildAt(i), visit)
    }
}
