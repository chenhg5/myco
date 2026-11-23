package dev.myco.android

import android.view.MotionEvent
import android.view.View
import android.view.ViewGroup
import android.webkit.ConsoleMessage
import android.webkit.WebChromeClient
import android.webkit.WebView
import androidx.activity.compose.setContent
import androidx.compose.runtime.*
import androidx.compose.ui.test.junit4.createAndroidComposeRule
import androidx.test.platform.app.InstrumentationRegistry
import dev.myco.network.HubClient
import dev.myco.pages.PageScreen
import dev.myco.ui.MycoTheme
import java.io.IOException
import java.net.*
import java.nio.ByteBuffer
import java.nio.ByteOrder
import java.util.concurrent.*
import kotlinx.serialization.json.*
import okhttp3.mockwebserver.*
import okio.Buffer
import org.junit.Assert.*
import org.junit.Rule
import org.junit.Test

/**
 * Real document-start bridge, CSP, Range fetch and Chromium audio decoder through the App proxy.
 */
class PageMediaJourneyTest {
    @get:Rule val app = createAndroidComposeRule<MainActivity>()

    @Test
    fun publicMediaStreamsInTheWebViewAndStopsWhenItsPageCloses() {
        MockWebServer().use { proxy ->
            MockWebServer().use { hubServer ->
                val previous = ProxySelector.getDefault()
                val calls = CopyOnWriteArrayList<RecordedRequest>()
                val wav = wave()
                proxy.dispatcher =
                    object : Dispatcher() {
                        override fun dispatch(request: RecordedRequest): MockResponse {
                            calls += request
                            if (request.mediaPath() == "/redirect")
                                return MockResponse()
                                    .setResponseCode(302)
                                    .setHeader("Location", "http://127.0.0.1/private")
                            if (request.mediaPath() == "/html")
                                return MockResponse()
                                    .setHeader("Content-Type", "text/html")
                                    .setBody("<script>bad()</script>")
                            if (request.mediaPath() == "/large")
                                return MockResponse()
                                    .setResponseCode(206)
                                    .setHeader("Content-Type", "application/octet-stream")
                                    .setHeader(
                                        "Content-Range",
                                        "bytes 2202518806-2202518809/3000000000",
                                    )
                                    .setBody("data")
                            val start =
                                request
                                    .getHeader("Range")
                                    ?.substringAfter("bytes=")
                                    ?.substringBefore('-')
                                    ?.toIntOrNull() ?: 0
                            val end =
                                request
                                    .getHeader("Range")
                                    ?.substringAfter('-')
                                    ?.toIntOrNull()
                                    ?.coerceAtMost(wav.lastIndex) ?: wav.lastIndex
                            if (start > wav.lastIndex)
                                return MockResponse()
                                    .setResponseCode(416)
                                    .setHeader("Content-Range", "bytes */${wav.size}")
                            return MockResponse()
                                .setResponseCode(
                                    if (request.getHeader("Range") != null) 206 else 200
                                )
                                .setHeader("Content-Type", "audio/wav")
                                .setHeader("Accept-Ranges", "bytes")
                                .setHeader("Content-Range", "bytes $start-$end/${wav.size}")
                                .setHeader("ETag", "wave1")
                                .setHeader("Set-Cookie", "upstream=secret")
                                .setBody(Buffer().write(wav, start, end - start + 1))
                                .throttleBody(8192, 100, TimeUnit.MILLISECONDS)
                        }
                    }
                val html =
                    """
                      <!doctype html><meta name="viewport" content="width=device-width,initial-scale=1">
                      <button id="play" style="width:200px;height:80px" onclick="player.play().then(()=>window.playing=true).catch(e=>window.playError=e.message)">Play</button>
                      <audio id="player" preload="none" controls></audio>
                      <script>
                      window.ready=false; window.results=null;
                      window.prepare=async function(){ await myco.ready;
                        window.supported=myco.features.includes('net-media');
                        var u=myco.media.url('http://93.184.216.34/audio.wav');
                        window.sameOrigin=new URL(u).origin===location.origin;
                        document.cookie='pagecookie=private';
                        player.src=u;
                        window.phase='fetch'; var range=await fetch(u,{headers:{Range:'bytes=20-29','If-Range':'wave1','X-Secret':'never'}});
                        window.phase='body'; window.results={status:range.status,range:range.headers.get('content-range'),length:(await range.arrayBuffer()).byteLength};
                        var big=await fetch(myco.media.url('http://93.184.216.34/large'),{headers:{Range:'bytes=2202518806-2202518809'}});
                    var bigBody=await big.text();
                    var invalidRange=await fetch(u,{headers:{Range:'bytes=999999-'}}); await invalidRange.arrayBuffer();
                    window.largeResult=[big.status,bigBody,invalidRange.status];
                    var statuses=[];
                        for(var target of ['http://127.0.0.1/private','http://93.184.216.34/redirect','http://93.184.216.34/html']) {
                          var r=await fetch(myco.media.url(target)); statuses.push(r.status); await r.text();
                        }
                        window.statuses=statuses;
                        window.ready=true;
                      };
                      </script>
                    """
                        .trimIndent()
                hubServer.dispatcher =
                    object : Dispatcher() {
                        override fun dispatch(request: RecordedRequest) =
                            MockResponse()
                                .setHeader("Content-Type", "text/html")
                                .setBody(
                                    when {
                                        request.path == "/v1/console/me" ->
                                            """{"member":{"name":"owner"}}"""
                                        request.path.orEmpty().endsWith("/state") ->
                                            """{"state":{},"version":1}"""
                                        else -> html
                                    }
                                )
                    }
                ProxySelector.setDefault(
                    object : ProxySelector() {
                        override fun select(uri: URI) =
                            if (uri.host == "93.184.216.34")
                                listOf(
                                    Proxy(
                                        Proxy.Type.HTTP,
                                        InetSocketAddress("127.0.0.1", proxy.port),
                                    )
                                )
                            else listOf(Proxy.NO_PROXY)

                        override fun connectFailed(
                            uri: URI,
                            address: SocketAddress,
                            e: IOException,
                        ) {}
                    }
                )
                val show = mutableStateOf(true)
                try {
                    val hub = HubClient(hubServer.url("/").toString(), "fixture-only")
                    app.activity.runOnUiThread {
                        app.activity.setContent {
                            MycoTheme {
                                if (show.value)
                                    PageScreen(
                                        buildJsonObject {
                                            put("id", "pg_media")
                                            put("name", "Media test")
                                            put("version", 1)
                                        },
                                        hub,
                                        {},
                                    )
                            }
                        }
                    }
                    app.waitUntil(10_000) { javascript("typeof window.prepare") == "\"function\"" }
                    app.runOnUiThread {
                        findWeb(app.activity.window.decorView)!!.webChromeClient =
                            object : WebChromeClient() {
                                override fun onConsoleMessage(message: ConsoleMessage): Boolean {
                                    android.util.Log.i("MediaWebTest", message.message())
                                    return true
                                }
                            }
                    }
                    javascript("prepare().catch(e=>window.error=e.message)")
                    try {
                        app.waitUntil(20_000) {
                            val error = javascript("window.error")
                            check(error == "null" || error.isEmpty()) { "Media script: $error" }
                            javascript("window.ready") == "true"
                        }
                    } catch (e: Throwable) {
                        throw AssertionError(
                            "Media page: " +
                                javascript(
                                    "({ready:window.ready,error:window.error,results:window.results,phase:window.phase,statuses:window.statuses,supported:window.supported})"
                                ) +
                                " proxy calls=" +
                                calls.map { it.requestLine + " " + it.headers },
                            e,
                        )
                    }
                    assertEquals("true", javascript("window.supported && window.sameOrigin"))
                    val rangeResult =
                        Json.parseToJsonElement(javascript("window.results")).jsonObject
                    assertEquals(206, rangeResult["status"]!!.jsonPrimitive.int)
                    assertEquals(
                        "bytes 20-29/${wav.size}",
                        rangeResult["range"]!!.jsonPrimitive.content,
                    )
                    assertEquals(10, rangeResult["length"]!!.jsonPrimitive.int)
                    assertEquals("[206,\"data\",416]", javascript("window.largeResult"))
                    assertEquals("[403,403,415]", javascript("window.statuses"))
                    // The source cannot reach the public URL directly under the unchanged CSP.
                    javascript(
                        "window.direct='pending'; fetch('http://93.184.216.34/direct').then(()=>window.direct='allowed').catch(()=>window.direct='blocked')"
                    )
                    app.waitUntil(5_000) { javascript("window.direct") == "\"blocked\"" }
                    assertFalse(
                        calls.any {
                            it.mediaPath() == "/direct" ||
                                URI(it.requestLine.split(' ')[1]).host == "127.0.0.1"
                        }
                    )
                    tapPlay()
                    app.waitUntil(20_000) {
                        javascript(
                            "window.playing && player.currentTime > 0 && player.readyState >= 2"
                        ) == "true"
                    }
                    javascript("player.currentTime=25")
                    app.waitUntil(15_000) {
                        calls.any {
                            it.mediaPath() == "/audio.wav" &&
                                (it.getHeader("Range")
                                    ?.substringAfter("bytes=")
                                    ?.substringBefore('-')
                                    ?.toIntOrNull() ?: 0) > 1000
                        }
                    }
                    app.runOnUiThread { show.value = false }
                    app.waitForIdle()
                    val afterClose = calls.size
                    Thread.sleep(500)
                    assertEquals(afterClose, calls.size)
                    calls.forEach { request ->
                        listOf("Authorization", "Cookie", "Origin", "Referer", "X-Secret").forEach {
                            assertNull(it, request.getHeader(it))
                        }
                    }
                } finally {
                    app.runOnUiThread { show.value = false }
                    app.waitForIdle()
                    ProxySelector.setDefault(previous)
                }
            }
        }
    }

    private fun RecordedRequest.mediaPath(): String = URI(requestLine.split(' ')[1]).path

    private fun wave(): ByteArray {
        val samples = 8000 * 30
        return ByteBuffer.allocate(44 + samples * 2)
            .order(ByteOrder.LITTLE_ENDIAN)
            .apply {
                put("RIFF".toByteArray())
                putInt(36 + samples * 2)
                put("WAVEfmt ".toByteArray())
                putInt(16)
                putShort(1)
                putShort(1)
                putInt(8000)
                putInt(16000)
                putShort(2)
                putShort(16)
                put("data".toByteArray())
                putInt(samples * 2)
                repeat(samples) {
                    putShort((Math.sin(it * 2 * Math.PI * 220 / 8000) * 1000).toInt().toShort())
                }
            }
            .array()
    }

    private fun tapPlay() {
        var x = 0f
        var y = 0f
        InstrumentationRegistry.getInstrumentation().runOnMainSync {
            val web = findWeb(app.activity.window.decorView)!!
            val point = IntArray(2)
            web.getLocationOnScreen(point)
            x = point[0] + 50 * app.activity.resources.displayMetrics.density
            y = point[1] + 40 * app.activity.resources.displayMetrics.density
        }
        val now = android.os.SystemClock.uptimeMillis()
        listOf(MotionEvent.ACTION_DOWN, MotionEvent.ACTION_UP).forEach { action ->
            val event = MotionEvent.obtain(now, now, action, x, y, 0)
            event.source = android.view.InputDevice.SOURCE_TOUCHSCREEN
            InstrumentationRegistry.getInstrumentation().sendPointerSync(event)
            event.recycle()
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
        check(done.await(5, TimeUnit.SECONDS))
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
