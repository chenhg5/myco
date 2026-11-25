package dev.myco.android

import android.view.View
import android.view.ViewGroup
import android.view.accessibility.AccessibilityNodeInfo
import android.webkit.WebView
import androidx.activity.compose.setContent
import androidx.compose.ui.test.junit4.createAndroidComposeRule
import androidx.test.platform.app.InstrumentationRegistry
import dev.myco.network.HubClient
import dev.myco.pages.PageScreen
import dev.myco.ui.MycoTheme
import java.util.concurrent.CountDownLatch
import java.util.concurrent.TimeUnit
import kotlinx.serialization.json.*
import okhttp3.mockwebserver.*
import org.junit.Assert.*
import org.junit.Rule
import org.junit.Test

/** Uses DocumentsUI itself and the real Downloads provider; no fake picker result. */
class NativeSaveJourneyTest {
    @get:Rule val app = createAndroidComposeRule<MainActivity>()

    @Test
    fun aPageCanSaveAFileThroughTheRealSystemDocumentPicker() {
        val name = "myco_native_${java.util.UUID.randomUUID().toString().replace("-", "")}.txt"
        val automation = InstrumentationRegistry.getInstrumentation().uiAutomation
        MockWebServer().use { server ->
            server.dispatcher =
                object : Dispatcher() {
                    override fun dispatch(request: RecordedRequest) =
                        MockResponse()
                            .setBody(
                                when {
                                    request.path == "/v1/console/me" ->
                                        """{"member":{"name":"fixture"}}"""
                                    request.path.orEmpty().endsWith("/state") ->
                                        """{"state":{},"version":1}"""
                                    else ->
                                        """<!doctype html><script>myco.ready.then(()=>window.ready=true);window.saveFile=()=>myco.download('$name','saved by myco').then(v=>window.saved=v).catch(e=>window.failure=e.message);</script>"""
                                }
                            )
                }
            val hub = HubClient(server.url("/").toString(), "fixture-only")
            app.runOnUiThread {
                app.activity.setContent {
                    MycoTheme {
                        PageScreen(
                            buildJsonObject {
                                put("id", "pg_native_save")
                                put("name", "Save test")
                                put("version", 1)
                            },
                            hub,
                            {},
                        )
                    }
                }
            }
            try {
                app.waitUntil(15_000) { javascript("window.ready") == "true" }
                javascript("saveFile()")
                app.waitUntil(15_000) { saveButton(automation.rootInActiveWindow) != null }
                assertTrue(
                    saveButton(automation.rootInActiveWindow)!!.performAction(
                        AccessibilityNodeInfo.ACTION_CLICK
                    )
                )
                app.waitUntil(15_000) { javascript("window.saved") == "true" }
                val descriptor = automation.executeShellCommand("cat /sdcard/Download/$name")
                val data =
                    android.os.ParcelFileDescriptor.AutoCloseInputStream(descriptor).use {
                        it.readBytes().decodeToString()
                    }
                assertEquals("saved by myco", data)
            } finally {
                if (
                    automation.rootInActiveWindow
                        ?.packageName
                        ?.toString()
                        ?.contains("documentsui") == true
                )
                    automation.performGlobalAction(
                        android.accessibilityservice.AccessibilityService.GLOBAL_ACTION_BACK
                    )
                automation.executeShellCommand("rm -f /sdcard/Download/$name").close()
            }
        }
    }

    private fun saveButton(node: AccessibilityNodeInfo?): AccessibilityNodeInfo? {
        if (node == null) return null
        if (
            node.isEnabled &&
                node.isClickable &&
                (node.viewIdResourceName?.endsWith(":id/action_menu_save") == true ||
                    node.text?.toString()?.equals("Save", true) == true)
        )
            return node
        for (i in 0 until node.childCount) saveButton(node.getChild(i))?.let {
            return it
        }
        return null
    }

    private fun javascript(script: String): String {
        val latch = CountDownLatch(1)
        var result = ""
        app.activity.runOnUiThread {
            find(app.activity.window.decorView)?.evaluateJavascript(script) {
                result = it
                latch.countDown()
            } ?: latch.countDown()
        }
        check(latch.await(5, TimeUnit.SECONDS))
        return result
    }

    private fun find(view: View): WebView? {
        if (view is WebView) return view
        if (view is ViewGroup)
            for (i in 0 until view.childCount) find(view.getChildAt(i))?.let {
                return it
            }
        return null
    }
}
