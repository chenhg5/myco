package dev.myco.pages

import android.annotation.SuppressLint
import android.net.Uri
import android.webkit.*
import androidx.compose.foundation.layout.*
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.Close
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.viewinterop.AndroidView
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.LifecycleEventObserver
import androidx.lifecycle.compose.LocalLifecycleOwner
import androidx.webkit.WebViewCompat
import androidx.webkit.WebViewFeature
import dev.myco.model.*
import dev.myco.network.HubClient
import dev.myco.network.HubRemoteTransport
import dev.myco.network.MediaFailure
import dev.myco.network.PageMedia
import dev.myco.network.PageNetwork
import dev.myco.network.RemoteSessions
import dev.myco.network.WebViewMediaInput
import dev.myco.ui.*
import java.io.ByteArrayInputStream
import kotlin.coroutines.resume
import kotlinx.coroutines.*
import kotlinx.serialization.json.*

private const val POLICY =
    "default-src 'self' data: blob:; script-src 'self' 'unsafe-inline' 'unsafe-eval' data: blob:; style-src 'self' 'unsafe-inline' data: blob:; img-src 'self' data: blob:; font-src 'self' data:; media-src 'self' data: blob:; connect-src 'self' data: blob:; worker-src 'self' blob:; frame-src 'none'; form-action 'none'; base-uri 'self'; object-src 'none'"

private data class PageConsent(val text: String, val to: String, val answer: (Boolean) -> Unit)

/** An opaque document location; request distinguishes repeated clicks on the same link. */
data class PageNavigation(val fragment: String? = null, val request: Long = 0)

@Composable
fun PageScreen(
    page: JsonElement,
    hub: HubClient,
    back: () -> Unit,
    navigation: PageNavigation = PageNavigation(),
) {
    key(hub.baseUrl, page.field("id")) { PageContent(page, hub, back, navigation) }
}

@SuppressLint("SetJavaScriptEnabled")
@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun PageContent(
    page: JsonElement,
    hub: HubClient,
    back: () -> Unit,
    navigation: PageNavigation,
) {
    val context = LocalContext.current
    val lifecycle = LocalLifecycleOwner.current.lifecycle
    val hostScope = rememberCoroutineScope()
    val id = page.field("id")
    val name = page.field("name")
    val origin =
        remember(hub.baseUrl, id) {
            val hash =
                java.security.MessageDigest.getInstance("SHA-256")
                    .digest((hub.baseUrl + ":" + id).toByteArray())
                    .joinToString("") { "%02x".format(it) }
                    .take(32)
            "https://p-$hash.myco.invalid"
        }
    val pageHost = Uri.parse(origin).host

    var consent by remember { mutableStateOf<PageConsent?>(null) }
    var problem by remember { mutableStateOf<String?>(null) }
    var remoteConsent by remember { mutableStateOf<PageConsent?>(null) }
    var remoteActive by remember { mutableStateOf(false) }
    val cleanupScope = remember { CoroutineScope(SupervisorJob() + Dispatchers.IO) }
    val network = remember { PageNetwork() }
    val media = remember(id) { PageMedia() }
    val remote =
        remember(id) {
            RemoteSessions(
                HubRemoteTransport(hub),
                cleanupScope,
                ask = { computer, control ->
                    suspendCancellableCoroutine { c ->
                        remoteConsent =
                            PageConsent(
                                if (control) "Control this computer’s screen, keyboard, and mouse"
                                else "View this computer’s screen",
                                computer["name"]?.jsonPrimitive?.content ?: "Computer",
                            ) {
                                if (c.isActive) c.resume(it)
                            }
                        c.invokeOnCancellation { hostScope.launch { remoteConsent = null } }
                    }
                },
                changed = { remoteActive = it },
            )
        }
    val lang = words("zh", "en")
    val theme = if (androidx.compose.foundation.isSystemInDarkTheme()) "dark" else "light"
    var active by remember { mutableStateOf(true) }
    val web = remember(id) { WebView(context) }
    var documentReady by remember(id) { mutableStateOf(false) }
    var appliedNavigation by remember(id) { mutableStateOf(navigation) }
    var requests by remember(id) { mutableStateOf(SupervisorJob()) }
    var known by remember(id) { mutableIntStateOf(0) }
    var seq by remember(id) { mutableIntStateOf(0) }
    var watching by remember(id) { mutableStateOf(false) }
    var bridged by remember(id) { mutableStateOf(false) }
    var allowed by remember(id) { mutableStateOf(false) }
    val sent = remember(id) { mutableListOf<Long>() }
    val prefs =
        remember(id, hub.baseUrl) {
            context.getSharedPreferences(
                "page-" +
                    java.security.MessageDigest.getInstance("SHA-256")
                        .digest((hub.baseUrl + id).toByteArray())
                        .joinToString("") { "%02x".format(it) },
                0,
            )
        }
    suspend fun call(
        method: String,
        path: List<String>,
        query: Map<String, String> = emptyMap(),
        body: JsonElement? = null,
    ): JsonElement = hub.request(method, listOf("v1", "pages", id) + path, query, body)
    suspend fun answer(method: String, args: JsonObject): JsonElement {
        fun arg(key: String) = args.field(key)
        val collection = arg("collection")
        return when (method) {
            "ready" -> {
                val state = call("GET", listOf("state")).jsonObject
                known = state["version"]?.jsonPrimitive?.intOrNull ?: 0
                bridged = true
                buildJsonObject {
                    put("page", page)
                    put(
                        "me",
                        buildJsonObject { put("name", hub.whoami().member?.displayName ?: "Me") },
                    )
                    put("state", state["state"] ?: JsonNull)
                    put("version", known)
                    put(
                        "preferences",
                        buildJsonObject {
                            put("language", lang)
                            put("theme", theme)
                        },
                    )
                }
            }
            "update" -> {
                try {
                    val state =
                        call(
                                "PUT",
                                listOf("state"),
                                body =
                                    buildJsonObject {
                                        put("state", args["state"] ?: JsonNull)
                                        put("if_version", args["version"] ?: JsonPrimitive(0))
                                    },
                            )
                            .jsonObject
                    known = state["version"]?.jsonPrimitive?.intOrNull ?: known
                    buildJsonObject {
                        put("ok", true)
                        put("state", state["state"] ?: JsonNull)
                        put("version", known)
                    }
                } catch (e: HubException) {
                    if (e.status != 409) throw e
                    val state = call("GET", listOf("state")).jsonObject
                    known = state["version"]?.jsonPrimitive?.intOrNull ?: known
                    buildJsonObject {
                        put("ok", false)
                        put("state", state["state"] ?: JsonNull)
                        put("version", known)
                    }
                }
            }
            "net.fetch" -> network.fetch(args)
            "remote.call" ->
                remote.call(arg("action"), args["args"] as? JsonObject ?: buildJsonObject {})
            "data.list" ->
                call(
                        "GET",
                        listOf("data"),
                        if (collection.isEmpty()) emptyMap() else mapOf("collection" to collection),
                    )
                    .also { seq = maxOf(seq, it.jsonObject["seq"]?.jsonPrimitive?.intOrNull ?: 0) }
            "data.get" -> call("GET", listOf("data", collection, arg("id")))
            "data.put" -> {
                val record = arg("id")
                call(
                    if (record.isEmpty()) "POST" else "PUT",
                    listOf("data", collection) +
                        if (record.isEmpty()) emptyList() else listOf(record),
                    body =
                        buildJsonObject {
                            put("data", args["data"] ?: buildJsonObject {})
                            args["ifVersion"]
                                ?.takeUnless { it == JsonNull }
                                ?.let { put("if_version", it) }
                        },
                )
            }
            "data.remove" -> {
                call(
                    "DELETE",
                    listOf("data", collection, arg("id")),
                    args["ifVersion"]
                        ?.jsonPrimitive
                        ?.intOrNull
                        ?.takeIf { it > 0 }
                        ?.let { mapOf("if_version" to it.toString()) } ?: emptyMap(),
                )
                JsonPrimitive(true)
            }
            "data.watch" -> {
                if (seq == 0)
                    seq =
                        call("GET", listOf("data")).jsonObject["seq"]?.jsonPrimitive?.intOrNull ?: 0
                watching = true
                JsonPrimitive(true)
            }
            "storage.get" ->
                prefs.getString(arg("key"), null)?.let { HubJson.parseToJsonElement(it) }
                    ?: JsonNull
            "storage.set" -> {
                val value = args["value"] ?: JsonNull
                require(
                    (prefs.all.values.sumOf { it.toString().length } + value.toString().length) *
                        2 <= 1024 * 1024
                ) {
                    "Page storage is full (1 MB)"
                }
                withContext(Dispatchers.IO) {
                    prefs
                        .edit()
                        .apply {
                            if (value == JsonNull) remove(arg("key"))
                            else putString(arg("key"), value.toString())
                        }
                        .commit()
                }
                JsonPrimitive(true)
            }
            "send" -> {
                val text = arg("text").trim()
                require(text.isNotEmpty()) { "The message is empty" }
                val to = page.field("publisher")
                require(to.isNotBlank() && !to.startsWith("@console:")) {
                    "This page has no agent to send to"
                }
                val now = System.currentTimeMillis()
                sent.removeAll { now - it > 60_000 }
                require(sent.size < 5) { "Too many messages: at most five a minute" }
                if (!allowed)
                    allowed = suspendCancellableCoroutine { c ->
                        consent = PageConsent(text, to) { if (c.isActive) c.resume(it) }
                        c.invokeOnCancellation { hostScope.launch { consent = null } }
                    }
                check(allowed) { "You did not allow this page to send messages" }
                sent += now
                val result =
                    hub.send(
                        SendRequest(
                            to,
                            text + "\n\n(from the page \"$name\", state version $known)",
                            project = page.field("project").ifEmpty { null },
                        )
                    )
                buildJsonObject { put("id", result.id) }
            }
            else -> throw IllegalArgumentException("This app does not support $method")
        }
    }
    DisposableEffect(web, lifecycle) {
        if (
            !WebViewFeature.isFeatureSupported(WebViewFeature.WEB_MESSAGE_LISTENER) ||
                !WebViewFeature.isFeatureSupported(WebViewFeature.DOCUMENT_START_SCRIPT)
        ) {
            problem = "Update Android System WebView to open this app"
        } else {
            web.settings.apply {
                javaScriptEnabled = true
                allowFileAccess = false
                allowContentAccess = false
                domStorageEnabled = false
                setSupportMultipleWindows(false)
                mixedContentMode = WebSettings.MIXED_CONTENT_NEVER_ALLOW
                mediaPlaybackRequiresUserGesture = true
            }
            CookieManager.getInstance().setAcceptThirdPartyCookies(web, false)
            WebViewCompat.addWebMessageListener(web, "mycoNative", setOf(origin)) {
                _,
                message,
                sourceOrigin,
                isMainFrame,
                reply ->
                if (!active || !isMainFrame || sourceOrigin.toString().trimEnd('/') != origin)
                    return@addWebMessageListener
                val generation = requests
                hostScope.launch(generation + Dispatchers.Main.immediate) {
                    var callId: JsonElement = JsonNull
                    val result =
                        try {
                            val input = HubJson.parseToJsonElement(message.data ?: "{}").jsonObject
                            callId = input["id"] ?: JsonNull
                            buildJsonObject {
                                put(
                                    "value",
                                    answer(
                                        input.field("method"),
                                        input["args"] as? JsonObject ?: buildJsonObject {},
                                    ),
                                )
                            }
                        } catch (e: CancellationException) {
                            throw e
                        } catch (e: Exception) {
                            buildJsonObject {
                                put(
                                    "error",
                                    if (e is HubException) e.failure.message
                                    else e.message ?: "The request failed",
                                )
                                put("code", if (e is HubException) e.failure.error else "error")
                                if (e is HubException) e.failure.latest?.let { put("latest", it) }
                            }
                        }
                    if (active && generation === requests)
                        reply.postMessage(
                            buildJsonObject {
                                put("id", callId)
                                put("answer", result)
                            }
                                .toString()
                        )
                }
            }
            val script =
                context.assets.open("page-bridge.js").bufferedReader().use { it.readText() }
            WebViewCompat.addDocumentStartJavaScript(web, script, setOf(origin))
            web.webViewClient =
                object : WebViewClient() {
                    override fun onPageFinished(view: WebView, url: String) {
                        if (Uri.parse(url).host == pageHost) documentReady = true
                    }

                    override fun shouldOverrideUrlLoading(
                        view: WebView,
                        request: WebResourceRequest,
                    ) = request.url.scheme != "https" || request.url.host != pageHost

                    override fun shouldInterceptRequest(
                        view: WebView,
                        request: WebResourceRequest,
                    ): WebResourceResponse {
                        fun response(status: Int, type: String, data: ByteArray) =
                            WebResourceResponse(
                                type,
                                "utf-8",
                                status,
                                if (status == 200) "OK" else "Blocked",
                                mapOf(
                                    "Content-Security-Policy" to POLICY,
                                    "X-Content-Type-Options" to "nosniff",
                                    "Cache-Control" to "no-store",
                                ),
                                ByteArrayInputStream(data),
                            )
                        val uri = request.url
                        if (
                            uri.scheme != "https" ||
                                uri.host != pageHost ||
                                (uri.port != -1 && uri.port != 443)
                        )
                            return response(403, "text/plain", byteArrayOf())
                        if (uri.path == "/__myco/stream") {
                            return try {
                                val stream =
                                    media.open(
                                        uri.getQueryParameter("url").orEmpty(),
                                        request.method,
                                        request.requestHeaders,
                                    )
                                val input =
                                    WebViewMediaInput(
                                        stream,
                                        request.requestHeaders.entries
                                            .firstOrNull { it.key.equals("Range", true) }
                                            ?.value,
                                    )
                                WebResourceResponse(
                                    stream.type,
                                    null,
                                    stream.status,
                                    when (stream.status) {
                                        206 -> "Partial Content"
                                        416 -> "Range Not Satisfiable"
                                        else -> "OK"
                                    },
                                    input.headers,
                                    input,
                                )
                            } catch (e: Exception) {
                                response(
                                    if (e is MediaFailure) e.status else 502,
                                    "text/plain",
                                    "Media unavailable".toByteArray(),
                                )
                            }
                        }
                        if (request.method != "GET")
                            return response(405, "text/plain", byteArrayOf())
                        return try {
                            val resource = uri.path.orEmpty().trimStart('/')
                            val path =
                                if (page.flag("live")) listOf("live")
                                else listOf("v", page.field("version").ifEmpty { "1" })
                            val bytes =
                                runBlocking(Dispatchers.IO + requests) {
                                    hub.raw(
                                        "GET",
                                        listOf("v1", "pages", id) +
                                            path +
                                            resource.split('/').filter { it.isNotEmpty() },
                                    )
                                }
                            val type =
                                when (resource.substringAfterLast('.', "html")) {
                                    "html" -> "text/html"
                                    "js",
                                    "mjs" -> "text/javascript"
                                    "css" -> "text/css"
                                    "svg" -> "image/svg+xml"
                                    "json" -> "application/json"
                                    "wasm" -> "application/wasm"
                                    else ->
                                        android.webkit.MimeTypeMap.getSingleton()
                                            .getMimeTypeFromExtension(
                                                resource.substringAfterLast('.')
                                            ) ?: "application/octet-stream"
                                }
                            response(200, type, bytes)
                        } catch (_: Exception) {
                            response(404, "text/plain", "Page file unavailable".toByteArray())
                        }
                    }
                }
            web.webChromeClient =
                object : WebChromeClient() {
                    override fun onPermissionRequest(request: PermissionRequest) {
                        request.deny()
                    }
                }
            // Only the fragment is appended to this page's private document origin.
            val suffix = navigation.fragment?.let { "#$it" }.orEmpty()
            web.loadUrl("$origin/$suffix")
        }
        val observer = LifecycleEventObserver { _, event ->
            when (event) {
                Lifecycle.Event.ON_PAUSE -> {
                    active = false
                    media.pause()
                    web.evaluateJavascript(
                        "document.querySelectorAll('audio,video').forEach(function(m){m.pause();})",
                        null,
                    )
                    requests.cancel()
                    remote.pause()
                    web.evaluateJavascript("window.__mycoEvent({myco:\"remote-disconnect\"})", null)
                    web.onPause()
                    consent?.answer?.invoke(false)
                    consent = null
                    remoteConsent?.answer?.invoke(false)
                    remoteConsent = null
                }
                Lifecycle.Event.ON_RESUME -> {
                    active = true
                    media.resume()
                    remote.resume()
                    if (!requests.isActive) requests = SupervisorJob()
                    web.onResume()
                }
                else -> Unit
            }
        }
        lifecycle.addObserver(observer)
        onDispose {
            active = false
            media.close()
            requests.cancel()
            remote.close()
            lifecycle.removeObserver(observer)
            web.stopLoading()
            web.destroy()
            consent?.answer?.invoke(false)
            consent = null
        }
    }
    LaunchedEffect(web, documentReady, navigation) {
        if (documentReady && navigation != appliedNavigation) {
            val fragment = JsonPrimitive(navigation.fragment.orEmpty()).toString()
            web.evaluateJavascript(
                """
                (function() {
                  var next = new URL(location.href);
                  next.hash = '#' + $fragment;
                  if (next.hash === location.hash) {
                    var here = location.href;
                    window.dispatchEvent(new HashChangeEvent('hashchange', {oldURL: here, newURL: here}));
                  } else { location.hash = next.hash; }
                })();
            """
                    .trimIndent(),
                null,
            )
            appliedNavigation = navigation
        }
    }
    LaunchedEffect(web, requests, active) {
        if (!active) return@LaunchedEffect
        while (isActive) {
            delay(2_000)
            if (bridged)
                try {
                    val state = call("GET", listOf("state")).jsonObject
                    val version = state["version"]?.jsonPrimitive?.intOrNull ?: 0
                    if (version > known) {
                        known = version
                        val event = buildJsonObject {
                            put("myco", "change")
                            put("state", state["state"] ?: JsonNull)
                            put("version", version)
                            put("by", state["updated_by"] ?: JsonPrimitive(""))
                        }
                        web.evaluateJavascript("window.__mycoEvent($event)", null)
                    }
                    if (watching) {
                        val feed =
                            call("GET", listOf("changes"), mapOf("after" to seq.toString()))
                                .jsonObject
                        seq = maxOf(seq, feed["seq"]?.jsonPrimitive?.intOrNull ?: 0)
                        val event = buildJsonObject {
                            put("myco", "data")
                            put("changes", feed["changes"] ?: JsonArray(emptyList()))
                            put("reset", feed["reset"] ?: JsonPrimitive(false))
                        }
                        web.evaluateJavascript("window.__mycoEvent($event)", null)
                    }
                } catch (e: CancellationException) {
                    throw e
                } catch (_: Exception) {}
        }
    }
    val activity = context as? android.app.Activity
    DisposableEffect(remoteActive) {
        val orientation = activity?.requestedOrientation
        val softInput = activity?.window?.attributes?.softInputMode
        val window = activity?.window
        if (remoteActive && window != null) {
            activity.requestedOrientation =
                android.content.pm.ActivityInfo.SCREEN_ORIENTATION_SENSOR_LANDSCAPE
            window.setSoftInputMode(
                android.view.WindowManager.LayoutParams.SOFT_INPUT_ADJUST_NOTHING
            )
            androidx.core.view.WindowCompat.getInsetsController(window, web)
                .hide(androidx.core.view.WindowInsetsCompat.Type.systemBars())
        }
        onDispose {
            if (activity != null && orientation != null) activity.requestedOrientation = orientation
            if (window != null && softInput != null) window.setSoftInputMode(softInput)
            if (window != null)
                androidx.core.view.WindowCompat.getInsetsController(window, web)
                    .show(androidx.core.view.WindowInsetsCompat.Type.systemBars())
        }
    }
    Scaffold(
        topBar = {
            if (!remoteActive)
                TopAppBar(
                    title = { Text(name) },
                    navigationIcon = {
                        IconButton(onClick = back) {
                            Icon(Icons.Outlined.Close, words("关闭", "Close"))
                        }
                    },
                )
        }
    ) { padding ->
        Column(Modifier.fillMaxSize().padding(padding)) {
            problem?.let { Problem(it) }
            Box(Modifier.fillMaxSize()) {
                AndroidView(factory = { web }, modifier = Modifier.fillMaxSize())
                if (remoteActive)
                    FilledTonalIconButton(
                        onClick = {
                            remote.reset()
                            web.evaluateJavascript(
                                "window.__mycoEvent({myco:\"remote-disconnect\"})",
                                null,
                            )
                        },
                        modifier = Modifier.align(androidx.compose.ui.Alignment.TopStart),
                    ) {
                        Icon(Icons.Outlined.Close, words("结束远控", "End remote control"))
                    }
            }
        }
    }
    consent?.let { request ->
        AlertDialog(
            onDismissRequest = {
                request.answer(false)
                consent = null
            },
            title = { Text(words("允许此应用发消息？", "Allow this app to send a message?")) },
            text = { Text("${request.to}\n\n${request.text}") },
            confirmButton = {
                TextButton(
                    onClick = {
                        request.answer(true)
                        consent = null
                    }
                ) {
                    Text(words("允许", "Allow"))
                }
            },
            dismissButton = {
                TextButton(
                    onClick = {
                        request.answer(false)
                        consent = null
                    }
                ) {
                    Text(words("取消", "Cancel"))
                }
            },
        )
    }
    remoteConsent?.let { request ->
        AlertDialog(
            onDismissRequest = {
                request.answer(false)
                remoteConsent = null
            },
            title = { Text(request.to) },
            text = { Text(request.text) },
            confirmButton = {
                TextButton(
                    onClick = {
                        request.answer(true)
                        remoteConsent = null
                    }
                ) {
                    Text(words("允许连接", "Allow connection"))
                }
            },
            dismissButton = {
                TextButton(
                    onClick = {
                        request.answer(false)
                        remoteConsent = null
                    }
                ) {
                    Text(words("取消", "Cancel"))
                }
            },
        )
    }
}

private fun JsonElement.field(name: String): String =
    (this as? JsonObject)?.get(name)?.jsonPrimitive?.contentOrNull.orEmpty()

private fun JsonElement.flag(name: String) =
    (this as? JsonObject)?.get(name)?.jsonPrimitive?.booleanOrNull ?: false
