package dev.myco.android

import android.view.View
import android.view.ViewGroup
import android.webkit.WebView
import androidx.activity.compose.setContent
import androidx.compose.runtime.*
import androidx.compose.ui.test.junit4.createAndroidComposeRule
import dev.myco.network.HubClient
import dev.myco.pages.PageScreen
import dev.myco.ui.MycoTheme
import java.util.concurrent.CopyOnWriteArrayList
import java.util.concurrent.CountDownLatch
import java.util.concurrent.TimeUnit
import kotlinx.serialization.json.*
import okhttp3.mockwebserver.*
import org.junit.Assert.*
import org.junit.Rule
import org.junit.Test

/** Exercises the real WebView document-start bridge, not a JavaScript stand-in. */
class PageBridgeJourneyTest {
    @get:Rule val app = createAndroidComposeRule<MainActivity>()

    @Test
    fun pageBridgeCanReadAndFetchWithoutSharingCredentialsOrAnotherPagesCookies() {
        MockWebServer().use { server ->
            val calls = CopyOnWriteArrayList<RecordedRequest>()
            server.dispatcher =
                object : Dispatcher() {
                    override fun dispatch(request: RecordedRequest): MockResponse {
                        calls += request
                        val path = request.path.orEmpty()
                        val body =
                            when {
                                path == "/v1/console/me" ->
                                    """{"member":{"name":"owner","display_name":"Test owner"}}"""
                                path.endsWith("/state") -> """{"state":{},"version":1}"""
                                path == "/catalog" -> "catalog result"
                                path.contains("/v/1") ->
                                    """
                            <!doctype html><div id="status">Loading</div><script>
                            myco.ready.then(async function(ctx) {
                              var result = await myco.net.fetch({url:${JsonPrimitive(server.url("/catalog").toString())}, response:"text"});
                              document.getElementById("status").textContent = ctx.me.name + ": " + result.body;
                            }).catch(function(e) { document.getElementById("status").textContent = "Failure: " + e.message; });
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
            val hub = HubClient(server.url("/").toString(), "fixture-only")
            val pageId = mutableStateOf("first")
            app.activity.runOnUiThread {
                app.activity.setContent {
                    MycoTheme {
                        PageScreen(
                            buildJsonObject {
                                put("id", pageId.value)
                                put("name", "Bridge test")
                                put("version", 1)
                                put("publisher", "android-test")
                            },
                            hub,
                            {},
                        )
                    }
                }
            }
            app.waitUntil(15_000) {
                javascript("document.getElementById('status')?.textContent") ==
                    "\"Test owner: catalog result\""
            }
            val fetch = calls.first { it.path == "/catalog" }
            assertNull(fetch.getHeader("Authorization"))
            assertNull(fetch.getHeader("Cookie"))
            assertEquals(
                "Bearer fixture-only",
                calls.first { it.path == "/v1/console/me" }.getHeader("Authorization"),
            )
            val firstOrigin = javascript("location.origin")
            javascript("document.cookie='private=first'; document.cookie")
            assertTrue(javascript("document.cookie").contains("private=first"))
            app.runOnUiThread { pageId.value = "second" }
            app.waitUntil(15_000) {
                javascript("document.getElementById('status')?.textContent") ==
                    "\"Test owner: catalog result\"" && javascript("location.origin") != firstOrigin
            }
            assertEquals("\"\"", javascript("document.cookie"))
        }
    }

    private fun javascript(script: String): String {
        val done = CountDownLatch(1)
        var answer = ""
        app.activity.runOnUiThread {
            findWeb(app.activity.window.decorView)?.evaluateJavascript(script) {
                answer = it
                done.countDown()
            } ?: done.countDown()
        }
        check(done.await(5, TimeUnit.SECONDS)) { "WebView did not answer" }
        return answer
    }

    private fun findWeb(view: View): WebView? {
        if (view is WebView) return view
        if (view is ViewGroup)
            for (i in 0 until view.childCount) findWeb(view.getChildAt(i))?.let {
                return it
            }
        return null
    }
}
