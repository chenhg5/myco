package dev.myco.network

import dev.myco.model.*
import dev.myco.model.Credentials
import java.io.IOException
import java.util.concurrent.TimeUnit
import kotlin.coroutines.resume
import kotlin.coroutines.resumeWithException
import kotlinx.coroutines.suspendCancellableCoroutine
import kotlinx.serialization.decodeFromString
import kotlinx.serialization.json.*
import okhttp3.*
import okhttp3.HttpUrl.Companion.toHttpUrl
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.RequestBody.Companion.toRequestBody

/** Redirects are refused so a workspace credential cannot cross an origin. */
class HubClient(
    val baseUrl: String,
    private val token: String,
    private val client: OkHttpClient = defaultClient(),
) : Hub {
    private val base = baseUrl.trimEnd('/').toHttpUrl()

    suspend fun request(
        method: String,
        segments: List<String>,
        query: Map<String, String> = emptyMap(),
        body: JsonElement? = null,
    ): JsonElement {
        val bytes = raw(method, segments, query, body?.toString()?.toByteArray())
        return if (bytes.isEmpty()) JsonNull else HubJson.parseToJsonElement(bytes.decodeToString())
    }

    suspend fun raw(
        method: String,
        segments: List<String>,
        query: Map<String, String> = emptyMap(),
        body: ByteArray? = null,
        contentType: String = "application/json",
    ): ByteArray {
        val url =
            base
                .newBuilder()
                .apply {
                    segments.forEach { addPathSegment(it) }
                    query.forEach { (k, v) -> addQueryParameter(k, v) }
                }
                .build()
        val requestBody =
            if (method in setOf("POST", "PUT", "PATCH"))
                (body ?: "{}".toByteArray()).toRequestBody(contentType.toMediaType())
            else null
        val request =
            Request.Builder()
                .url(url)
                .method(method, requestBody)
                .apply {
                    if (token.isNotBlank()) header("Authorization", "Bearer $token")
                    header("Accept", "application/json")
                }
                .build()
        return suspendCancellableCoroutine { continuation ->
            val call = client.newCall(request)
            continuation.invokeOnCancellation { call.cancel() }
            call.enqueue(
                object : Callback {
                    override fun onFailure(call: Call, e: IOException) {
                        if (!continuation.isCancelled)
                            continuation.resumeWithException(
                                IOException("Could not reach the workspace", e)
                            )
                    }

                    override fun onResponse(call: Call, response: Response) {
                        response.use {
                            try {
                                val data = it.body?.bytes() ?: byteArrayOf()
                                if (!it.isSuccessful) {
                                    val failure = runCatching {
                                        HubJson.decodeFromString<HubFailure>(data.decodeToString())
                                    }
                                        .getOrDefault(
                                            HubFailure(
                                                message = "Workspace returned HTTP ${it.code}"
                                            )
                                        )
                                    throw HubException(it.code, failure)
                                }
                                if (!continuation.isCancelled) continuation.resume(data)
                            } catch (e: Exception) {
                                if (!continuation.isCancelled) continuation.resumeWithException(e)
                            }
                        }
                    }
                }
            )
        }
    }

    private suspend inline fun <reified T> get(
        vararg path: String,
        query: Map<String, String> = emptyMap(),
    ): T = HubJson.decodeFromJsonElement(request("GET", path.toList(), query))

    override suspend fun whoami() = get<Whoami>("v1", "console", "me")

    override suspend fun overview() =
        get<Overview>("v1", "overview", query = mapOf("recent" to "0"))

    override suspend fun projects(): List<Project> =
        HubJson.decodeFromJsonElement(
            request("GET", listOf("v1", "projects")).jsonObject["projects"]
                ?: JsonArray(emptyList())
        )

    override suspend fun recent(query: String): List<Recent> =
        HubJson.decodeFromJsonElement(
            request(
                    "GET",
                    listOf("v1", "console", "recent"),
                    mapOf("group" to "pair", "side" to "me", "limit" to "100", "q" to query),
                )
                .jsonObject["items"] ?: JsonArray(emptyList())
        )

    override suspend fun exchanges(a: String, b: String, before: Long?): MessagePage =
        get(
            "v1",
            "console",
            "exchanges",
            query =
                buildMap {
                    put("a", a)
                    put("b", b)
                    put("limit", "100")
                    before?.let { put("before", it.toString()) }
                },
        )

    override suspend fun send(request: SendRequest): Message =
        HubJson.decodeFromJsonElement(
            this.request(
                    "POST",
                    listOf("v1", "console", "messages"),
                    body = HubJson.encodeToJsonElement(request),
                )
                .jsonObject
                .getValue("message")
        )

    override suspend fun steer(id: String, body: String, attachments: List<String>) {
        request(
            "POST",
            listOf("v1", "console", "messages", id, "steer"),
            body =
                buildJsonObject {
                    put("body", body)
                    if (attachments.isNotEmpty())
                        put("attachments", JsonArray(attachments.map(::JsonPrimitive)))
                },
        )
    }

    override suspend fun stop(id: String) {
        request("POST", listOf("v1", "console", "messages", id, "stop"))
    }

    override suspend fun trace(id: String, after: Int): Trace =
        get("v1", "console", "messages", id, "trace", query = mapOf("after" to after.toString()))

    override suspend fun upload(name: String, contentType: String, data: ByteArray): HubFile {
        require(data.size <= 50 * 1024 * 1024) { "Files must be smaller than 50 MB" }
        return HubJson.decodeFromString(
            raw("POST", listOf("v1", "console", "files"), mapOf("name" to name), data, contentType)
                .decodeToString()
        )
    }

    override suspend fun file(id: String) = raw("GET", listOf("v1", "console", "files", id))

    override suspend fun hidden(): List<String> =
        HubJson.decodeFromJsonElement(
            request("GET", listOf("v1", "console", "hidden")).jsonObject["ids"]
                ?: JsonArray(emptyList())
        )

    override suspend fun setHidden(hide: List<String>, show: List<String>): List<String> =
        HubJson.decodeFromJsonElement(
            request(
                    "POST",
                    listOf("v1", "console", "hidden"),
                    body =
                        buildJsonObject {
                            put("hide", JsonArray(hide.map(::JsonPrimitive)))
                            put("show", JsonArray(show.map(::JsonPrimitive)))
                        },
                )
                .jsonObject["ids"] ?: JsonArray(emptyList())
        )

    companion object {
        fun defaultClient() =
            OkHttpClient.Builder()
                .followRedirects(false)
                .followSslRedirects(false)
                .retryOnConnectionFailure(false)
                .connectTimeout(20, TimeUnit.SECONDS)
                .readTimeout(20, TimeUnit.SECONDS)
                .callTimeout(60, TimeUnit.SECONDS)
                .build()

        suspend fun join(link: String, device: String): Credentials {
            val parsed = JoinLink.parse(link)
            val client = HubClient(parsed.baseUrl, "")
            val token =
                if (parsed.credential.startsWith("myco_pair_")) {
                    client
                        .request(
                            "POST",
                            listOf("v1", "console", "pair"),
                            body =
                                buildJsonObject {
                                    put("code", parsed.credential)
                                    put("device", device)
                                },
                        )
                        .jsonObject
                        .getValue("token")
                        .jsonPrimitive
                        .content
                } else parsed.credential
            HubClient(parsed.baseUrl, token).whoami()
            val url = parsed.baseUrl.toHttpUrl()
            val name = url.pathSegments.lastOrNull { it.isNotEmpty() } ?: url.host
            return Credentials(
                Workspace(java.util.UUID.randomUUID().toString(), name, parsed.baseUrl),
                token,
            )
        }
    }
}

class JoinLink private constructor(val baseUrl: String, val credential: String) {
    override fun toString() = "JoinLink(baseUrl=$baseUrl)"

    companion object {
        fun parse(raw: String): JoinLink =
            try {
                parseLink(raw)
            } catch (_: IllegalArgumentException) {
                throw IllegalArgumentException("Use an add-device link from Settings")
            }

        private fun parseLink(raw: String): JoinLink {
            var text = raw.trim()
            if (text.startsWith("myco://")) {
                val outer = text.replaceFirst("myco://", "https://").toHttpUrl()
                text = outer.queryParameter("link") ?: ""
            }
            val url = text.toHttpUrl()
            require(url.username.isEmpty() && url.password.isEmpty()) {
                "Use an add-device link from Settings"
            }
            val parts = url.pathSegments
            val marker = parts.indexOf("c")
            require(marker >= 0 && parts.drop(marker + 2).all { it.isEmpty() }) {
                "Use an add-device link from Settings"
            }
            val code = parts.getOrNull(marker + 1).orEmpty()
            require(code.startsWith("myco_pair_") || code.startsWith("myco_console_")) {
                "Use an add-device link from Settings"
            }
            val base =
                url.newBuilder()
                    .query(null)
                    .fragment(null)
                    .encodedPath("/")
                    .apply { parts.take(marker).forEach { addPathSegment(it) } }
                    .build()
                    .toString()
                    .trimEnd('/')
            return JoinLink(base, code)
        }
    }
}
