package dev.myco.network

import java.io.IOException
import java.util.Base64
import java.util.concurrent.TimeUnit
import kotlin.coroutines.resume
import kotlin.coroutines.resumeWithException
import kotlinx.coroutines.suspendCancellableCoroutine
import kotlinx.serialization.json.*
import okhttp3.*
import okhttp3.HttpUrl.Companion.toHttpUrl
import okhttp3.MediaType.Companion.toMediaTypeOrNull
import okhttp3.RequestBody.Companion.toRequestBody

/** A separate client with no credential, cookie jar, authenticator or workspace interceptors. */
class PageNetwork(private val client: OkHttpClient = HubClient.defaultClient()) {
    suspend fun fetch(args: JsonObject): JsonObject {
        fun text(key: String, default: String = "") =
            args[key]?.jsonPrimitive?.contentOrNull ?: default
        val url = text("url").toHttpUrl()
        require(url.username.isEmpty() && url.password.isEmpty()) {
            "Enter an HTTP(S) address without a username or password"
        }
        val method = text("method", "GET").uppercase()
        require(method in setOf("GET", "HEAD", "POST", "PUT", "PATCH", "DELETE", "OPTIONS")) {
            "Unsupported HTTP method"
        }
        val mode = text("response", "auto")
        require(mode in setOf("auto", "text", "base64")) { "Unsupported response format" }
        val limit =
            (args["max_bytes"]?.jsonPrimitive?.intOrNull?.takeIf { it > 0 } ?: 16 * 1024 * 1024)
                .coerceAtMost(64 * 1024 * 1024)
        val timeout =
            (args["timeout_ms"]?.jsonPrimitive?.longOrNull?.takeIf { it > 0 } ?: 30_000)
                .coerceAtMost(600_000)
        val bytes =
            if (text("body_encoding") == "base64") Base64.getDecoder().decode(text("body"))
            else text("body").toByteArray()
        require(bytes.size <= 64 * 1024 * 1024) { "Request body is too large" }
        val headers =
            Headers.Builder()
                .apply {
                    (args["headers"] as? JsonObject)?.forEach { (key, value) ->
                        add(key, value.jsonPrimitive.content)
                    }
                    if (get("User-Agent") == null) add("User-Agent", "myco-app")
                }
                .build()
        val body =
            if (method in setOf("GET", "HEAD")) null
            else bytes.toRequestBody(headers["Content-Type"]?.toMediaTypeOrNull())
        var request = Request.Builder().url(url).headers(headers).method(method, body).build()
        val transport =
            client
                .newBuilder()
                .callTimeout(timeout, TimeUnit.MILLISECONDS)
                .readTimeout(timeout, TimeUnit.MILLISECONDS)
                .build()
        var redirects = 0
        while (true) {
            val response = exchange(transport, request, limit, mode)
            val location =
                response["headers"]?.jsonObject?.get("location")?.jsonPrimitive?.contentOrNull
            val status = response["status"]!!.jsonPrimitive.int
            if (
                args["redirect"]?.jsonPrimitive?.booleanOrNull == false ||
                    status !in setOf(301, 302, 303, 307, 308) ||
                    location == null
            )
                return response
            require(++redirects <= 5) { "Too many redirects" }
            val next = request.url.resolve(location) ?: error("Invalid redirect")
            require(next.username.isEmpty() && next.password.isEmpty()) { "Invalid redirect" }
            val builder = request.newBuilder().url(next)
            if (
                next.host != request.url.host ||
                    next.port != request.url.port ||
                    next.scheme != request.url.scheme
            ) {
                builder.removeHeader("Authorization")
                builder.removeHeader("Cookie")
                builder.removeHeader("Proxy-Authorization")
            }
            if (status == 303 || ((status == 301 || status == 302) && request.method == "POST"))
                builder.get().removeHeader("Content-Type")
            request = builder.build()
        }
    }

    private suspend fun exchange(
        client: OkHttpClient,
        request: Request,
        limit: Int,
        mode: String,
    ): JsonObject = suspendCancellableCoroutine { c ->
        val call = client.newCall(request)
        c.invokeOnCancellation { call.cancel() }
        call.enqueue(
            object : Callback {
                override fun onFailure(call: Call, e: IOException) {
                    if (!c.isCancelled)
                        c.resumeWithException(
                            IOException(
                                "The connection failed. Check your network and try again.",
                                e,
                            )
                        )
                }

                override fun onResponse(call: Call, response: Response) {
                    response.use { r ->
                        try {
                            val input = r.body?.byteStream()
                            val output = java.io.ByteArrayOutputStream(minOf(limit, 8192))
                            val buffer = ByteArray(8192)
                            var truncated = false
                            if (input != null)
                                while (true) {
                                    val n =
                                        input.read(
                                            buffer,
                                            0,
                                            minOf(buffer.size, limit - output.size() + 1),
                                        )
                                    if (n < 0) break
                                    val remaining = limit - output.size()
                                    output.write(buffer, 0, minOf(n, remaining))
                                    if (n > remaining) {
                                        truncated = true
                                        break
                                    }
                                }
                            val bytes = output.toByteArray()
                            val type = r.header("Content-Type").orEmpty().lowercase()
                            val asText =
                                mode == "text" ||
                                    (mode == "auto" &&
                                        (type.isEmpty() ||
                                            type.startsWith("text/") ||
                                            listOf("json", "xml", "javascript", "yaml").any {
                                                type.contains(it)
                                            }))
                            val result = buildJsonObject {
                                put("status", r.code)
                                put("status_text", r.message)
                                put("url", r.request.url.toString())
                                put(
                                    "headers",
                                    buildJsonObject {
                                        r.headers.names().forEach {
                                            put(it.lowercase(), r.headers[it].orEmpty())
                                        }
                                    },
                                )
                                put(
                                    "body",
                                    if (asText) bytes.decodeToString()
                                    else Base64.getEncoder().encodeToString(bytes),
                                )
                                put("encoding", if (asText) "utf8" else "base64")
                                put("truncated", truncated)
                            }
                            if (!c.isCancelled) c.resume(result)
                        } catch (e: Exception) {
                            if (!c.isCancelled) c.resumeWithException(e)
                        }
                    }
                }
            }
        )
    }
}
