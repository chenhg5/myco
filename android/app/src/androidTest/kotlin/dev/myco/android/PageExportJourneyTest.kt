package dev.myco.android

import android.app.Activity
import android.content.Intent
import android.view.View
import android.view.ViewGroup
import android.webkit.WebView
import androidx.activity.compose.LocalActivityResultRegistryOwner
import androidx.activity.compose.setContent
import androidx.activity.result.ActivityResultRegistry
import androidx.activity.result.ActivityResultRegistryOwner
import androidx.activity.result.contract.ActivityResultContract
import androidx.compose.runtime.*
import androidx.compose.ui.test.junit4.createAndroidComposeRule
import androidx.core.app.ActivityOptionsCompat
import androidx.core.content.FileProvider
import androidx.lifecycle.Lifecycle
import dev.myco.model.AppPreferences
import dev.myco.network.HubClient
import dev.myco.pages.PageScreen
import dev.myco.ui.MycoTheme
import java.io.File
import java.util.concurrent.CountDownLatch
import java.util.concurrent.TimeUnit
import kotlinx.serialization.json.*
import okhttp3.mockwebserver.*
import org.junit.Assert.*
import org.junit.Rule
import org.junit.Test

class PageExportJourneyTest {
    @get:Rule val app = createAndroidComposeRule<MainActivity>()

    @Test
    fun preferencesReachTheFirstScriptAndSaveResultsSurviveTheSystemPickerPause() {
        MockWebServer().use { server ->
            val html =
                """
                <!doctype html><meta name="viewport" content="width=device-width,initial-scale=1"><h1>Export fixture</h1><script>
                window.documentID=Math.random(); window.early=myco.preferences;
                window.earlyDOM=[document.documentElement.lang,document.documentElement.dataset.mycoTheme];
                window.changes=0; myco.on('preferences',()=>window.changes++);
                myco.ready.then(()=>window.ready=true);
                window.exportFile=function(name,content) {
                  window.outcome='pending'; window.problem=null;
                  myco.download(name,content||new Uint8Array([9,65,66,67,9]).subarray(1,4),'text/plain')
                    .then(saved=>window.outcome=saved).catch(e=>{window.problem=e.code;window.outcome='failed';});
                };
                </script>
                """
                    .trimIndent()
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
                                    else -> html
                                }
                            )
                }
            val registry = DocumentRegistry()
            val registryOwner =
                object : ActivityResultRegistryOwner {
                    override val activityResultRegistry = registry
                }
            val preferences = mutableStateOf(AppPreferences("dark", "zh"))
            val show = mutableStateOf(true)
            val hub = HubClient(server.url("/").toString(), "fixture-only")
            app.runOnUiThread {
                app.activity.setContent {
                    MycoTheme(preferences = preferences.value) {
                        CompositionLocalProvider(
                            LocalActivityResultRegistryOwner provides registryOwner
                        ) {
                            if (show.value)
                                PageScreen(
                                    buildJsonObject {
                                        put("id", "pg_export")
                                        put("name", "Export fixture")
                                        put("version", 1)
                                    },
                                    hub,
                                    {},
                                )
                        }
                    }
                }
            }
            app.waitUntil(15_000) { javascript("window.ready") == "true" }
            assertEquals("[\"zh\",\"dark\"]", javascript("[early.language,early.theme]"))
            assertEquals("[\"zh\",\"dark\"]", javascript("window.earlyDOM"))
            val document = javascript("window.documentID")
            val web = web()
            app.runOnUiThread { preferences.value = AppPreferences("light", "en") }
            app.waitUntil(5_000) {
                javascript("[myco.preferences.language,myco.preferences.theme]") ==
                    "[\"en\",\"light\"]"
            }
            assertEquals(
                "[\"en\",\"light\"]",
                javascript(
                    "[document.documentElement.lang,document.documentElement.dataset.mycoTheme]"
                ),
            )
            assertEquals(document, javascript("window.documentID"))
            assertSame(web, web())
            assertTrue(javascript("window.changes").toInt() > 0)

            val folder = File(app.activity.cacheDir, "shared/export-fixture").apply { mkdirs() }
            val output = File(folder, "report.txt")
            val uri = FileProvider.getUriForFile(app.activity, "dev.myco.android.files", output)
            javascript("exportFile('../报告.txt')")
            app.waitUntil(5_000) { registry.count == 1 }
            assertEquals(Intent.ACTION_CREATE_DOCUMENT, registry.intent!!.action)
            assertEquals("报告.txt", registry.intent!!.getStringExtra(Intent.EXTRA_TITLE))
            assertEquals("text/plain", registry.intent!!.type)
            app.activityRule.scenario.moveToState(Lifecycle.State.CREATED)
            app.runOnUiThread { registry.answer(Activity.RESULT_OK, Intent().setData(uri)) }
            app.activityRule.scenario.moveToState(Lifecycle.State.RESUMED)
            app.waitUntil(10_000) { javascript("window.outcome") == "true" }
            assertArrayEquals("ABC".toByteArray(), output.readBytes())

            javascript("exportFile('cancel.txt')")
            app.waitUntil(5_000) { registry.count == 2 }
            javascript(
                "window.busy=null; myco.download('second.txt','x').catch(e=>window.busy=e.code)"
            )
            app.waitUntil(5_000) { javascript("window.busy") == "\"busy\"" }
            assertEquals(2, registry.count)
            app.runOnUiThread { registry.answer(Activity.RESULT_CANCELED, null) }
            app.waitUntil(5_000) { javascript("window.outcome") == "false" }

            javascript("exportFile('invalid.apk')")
            app.waitUntil(5_000) { javascript("window.problem") == "\"invalid_argument\"" }
            assertEquals(2, registry.count)
            javascript("exportFile('large.pdf',new Blob([new Uint8Array(20*1024*1024+1)]))")
            app.waitUntil(5_000) { javascript("window.problem") == "\"too_large\"" }
            assertEquals(2, registry.count)

            javascript(
                "window.invalid=null; myco.download('empty.txt',undefined).catch(e=>window.invalid=e.code)"
            )
            app.waitUntil(5_000) { javascript("window.invalid") == "\"invalid_argument\"" }
            assertEquals(2, registry.count)
            val untouched = File(folder, "untouched.txt").apply { writeText("unchanged") }
            javascript("exportFile('destination.txt')")
            app.waitUntil(5_000) { registry.count == 3 }
            app.runOnUiThread {
                registry.answer(
                    Activity.RESULT_OK,
                    Intent().setData(android.net.Uri.fromFile(untouched)),
                )
            }
            app.waitUntil(5_000) { javascript("window.problem") == "\"save_failed\"" }
            assertEquals("unchanged", untouched.readText())
            val late = File(folder, "after-close.txt")
            late.delete()
            javascript("exportFile('after-close.txt')")
            app.waitUntil(5_000) { registry.count == 4 }
            app.runOnUiThread { show.value = false }
            app.waitForIdle()
            val lateUri = FileProvider.getUriForFile(app.activity, "dev.myco.android.files", late)
            app.runOnUiThread { registry.answer(Activity.RESULT_OK, Intent().setData(lateUri)) }
            Thread.sleep(200)
            assertFalse(late.exists())
        }
    }

    private class DocumentRegistry : ActivityResultRegistry() {
        @Volatile var count = 0
        @Volatile var intent: Intent? = null
        private var code = 0

        override fun <I, O> onLaunch(
            requestCode: Int,
            contract: ActivityResultContract<I, O>,
            input: I,
            options: ActivityOptionsCompat?,
        ) {
            code = requestCode
            intent =
                contract.createIntent(
                    androidx.test.platform.app.InstrumentationRegistry.getInstrumentation()
                        .targetContext,
                    input,
                )
            count++
        }

        fun answer(result: Int, data: Intent?) {
            dispatchResult(code, result, data)
        }
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

    private fun web(): WebView? {
        var result: WebView? = null
        app.runOnUiThread { result = find(app.activity.window.decorView) }
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
