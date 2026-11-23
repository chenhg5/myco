package dev.myco.network

import java.io.FilterInputStream
import java.io.IOException
import java.io.InputStream
import java.net.InetAddress
import java.net.InetSocketAddress
import java.net.Proxy
import java.net.ProxySelector
import java.util.concurrent.TimeUnit
import okhttp3.*
import okhttp3.HttpUrl.Companion.toHttpUrlOrNull

class MediaFailure(val status: Int, message: String) : IOException(message)

data class MediaStream(
    val status: Int,
    val type: String,
    val headers: Map<String, String>,
    val body: InputStream,
)

/** Public media only, through the system proxy, without Hub credentials or browser cookies. */
class PageMedia internal constructor(private val resolver: Dns) : AutoCloseable {
    constructor() : this(Dns.SYSTEM)

    private val lock = Any()
    private var active = true
    private var closed = false
    private var generation = 0L
    private val calls = mutableSetOf<Call>()
    private val proxySelector = ProxySelector.getDefault()
    private val client =
        OkHttpClient.Builder()
            .proxySelector(proxySelector)
            .followRedirects(false)
            .followSslRedirects(false)
            .retryOnConnectionFailure(false)
            .connectTimeout(15, TimeUnit.SECONDS)
            .readTimeout(30, TimeUnit.SECONDS)
            .build()

    /** Called on the WebView resource worker; the body stays streaming until Chromium closes it. */
    fun open(raw: String, method: String, headers: Map<String, String>): MediaStream {
        if (method !in setOf("GET", "HEAD"))
            throw MediaFailure(405, "Only GET and HEAD are supported")
        val started =
            synchronized(lock) {
                if (!active || closed) throw MediaFailure(503, "The page is inactive")
                generation
            }
        var url = target(raw)
        var redirects = 0
        while (true) {
            // Also required when a trusted proxy resolves the target itself.
            publicAddresses(url.host)
            val request =
                Request.Builder()
                    .url(url)
                    .method(method, null)
                    .header("User-Agent", "myco-app")
                    .header("Accept-Encoding", "identity")
            headers.forEach { (key, value) ->
                if (key.equals("Range", true) || key.equals("If-Range", true))
                    request.header(key, value)
            }
            // The configured proxy is trusted, even on the LAN. The media host never is.
            val proxies =
                proxySelector
                    .select(url.toUri())
                    .filter { it.type() == Proxy.Type.HTTP }
                    .mapNotNull {
                        (it.address() as? InetSocketAddress)?.let { a ->
                            a.address?.hostAddress ?: a.hostString
                        }
                    }
                    .toSet()
            val transport =
                client
                    .newBuilder()
                    .dns(
                        object : Dns {
                            override fun lookup(hostname: String): List<InetAddress> =
                                if (hostname in proxies) Dns.SYSTEM.lookup(hostname)
                                else publicAddresses(hostname)
                        }
                    )
                    .build()
            val call = transport.newCall(request.build())
            synchronized(lock) {
                if (!active || closed || generation != started)
                    throw MediaFailure(503, "The page is inactive")
                calls += call
            }
            val response =
                try {
                    call.execute()
                } catch (e: IOException) {
                    release(call)
                    if (e is MediaFailure) throw e
                    throw MediaFailure(502, "The media could not be reached")
                }
            fun discard() {
                response.close()
                release(call)
            }
            if (response.code in setOf(301, 302, 303, 307, 308)) {
                val next = response.header("Location")?.let(url::resolve)
                discard()
                if (++redirects >= 8 || next == null)
                    throw MediaFailure(502, "Invalid media redirect")
                url = target(next.toString())
                continue
            }
            if (response.code !in setOf(200, 206, 416)) {
                discard()
                throw MediaFailure(502, "The media returned an unsuccessful response")
            }
            val type =
                response.header("Content-Type").orEmpty().substringBefore(';').trim().lowercase()
            if (response.code != 416 && !mediaType(type)) {
                discard()
                throw MediaFailure(415, "Not audio or video")
            }
            val safeHeaders =
                mutableMapOf(
                    "Cache-Control" to "no-store",
                    "Content-Security-Policy" to "sandbox",
                    "X-Content-Type-Options" to "nosniff",
                )
            listOf("Content-Length", "Content-Range", "Accept-Ranges", "ETag", "Last-Modified")
                .forEach { key ->
                    response.header(key)?.let { safeHeaders[key] = it }
                }
            val safeType = if (response.code == 416) "text/plain" else type
            safeHeaders["Content-Type"] = safeType
            val stream =
                object : FilterInputStream(response.body!!.byteStream()) {
                    private val reading = Any()
                    private var disposed = false

                    override fun close() {
                        // Cancel first to wake a blocked socket read, then close Okio on one
                        // thread.
                        call.cancel()
                        synchronized(reading) {
                            if (!disposed) {
                                disposed = true
                                discard()
                            }
                        }
                    }

                    override fun available(): Int =
                        synchronized(reading) {
                            if (disposed) 0 else `in`.available()
                        }

                    override fun skip(n: Long): Long =
                        synchronized(reading) {
                            if (disposed) throw IOException("The media stream is closed")
                            try {
                                `in`.skip(n)
                            } catch (e: IOException) {
                                close()
                                throw e
                            }
                        }

                    override fun read(): Int =
                        synchronized(reading) {
                            if (disposed) throw IOException("The media stream is closed")
                            try {
                                `in`.read().also { if (it < 0) release(call) }
                            } catch (e: IOException) {
                                close()
                                throw e
                            }
                        }

                    override fun read(bytes: ByteArray, offset: Int, length: Int): Int =
                        synchronized(reading) {
                            if (disposed) throw IOException("The media stream is closed")
                            try {
                                `in`.read(bytes, offset, length).also { if (it < 0) release(call) }
                            } catch (e: IOException) {
                                close()
                                throw e
                            }
                        }
                }
            // A lifecycle pause may have cancelled the call while its headers were arriving.
            synchronized(lock) {
                if (!active || closed || generation != started || call.isCanceled()) {
                    stream.close()
                    throw MediaFailure(503, "The page is inactive")
                }
            }
            return MediaStream(response.code, safeType, safeHeaders, stream)
        }
    }

    private fun release(call: Call) {
        synchronized(lock) { calls -= call }
    }

    fun pause() {
        synchronized(lock) {
                active = false
                generation++
                calls.toList().also { calls.clear() }
            }
            .forEach { it.cancel() }
    }

    fun resume() {
        synchronized(lock) { if (!closed) active = true }
    }

    override fun close() {
        synchronized(lock) { closed = true }
        pause()
        client.connectionPool.evictAll()
    }

    private fun publicAddresses(host: String): List<InetAddress> {
        val name = host.trimEnd('.').lowercase()
        if (
            name == "localhost" ||
                name.endsWith(".localhost") ||
                name.endsWith(".local") ||
                '%' in name
        )
            throw MediaFailure(403, "Only public addresses can be streamed")
        val addresses =
            try {
                resolver.lookup(host)
            } catch (_: IOException) {
                throw MediaFailure(502, "The media host could not be reached")
            }
        if (addresses.isEmpty() || addresses.any { !publicAddress(it) })
            throw MediaFailure(403, "Only public addresses can be streamed")
        return addresses
    }

    companion object {
        internal fun target(raw: String): HttpUrl {
            val url =
                raw.takeIf { it.length <= 4096 }?.toHttpUrlOrNull()
                    ?: throw MediaFailure(400, "Enter a public HTTP(S) address")
            if (url.username.isNotEmpty() || url.password.isNotEmpty())
                throw MediaFailure(400, "Media addresses cannot contain credentials")
            return url.newBuilder().fragment(null).build()
        }

        internal fun mediaType(type: String) =
            type.startsWith("audio/") ||
                type.startsWith("video/") ||
                type in
                    setOf(
                        "application/ogg",
                        "application/octet-stream",
                        "binary/octet-stream",
                        "application/vnd.apple.mpegurl",
                        "application/x-mpegurl",
                    )

        internal fun publicAddress(ip: InetAddress): Boolean {
            if (
                ip.isAnyLocalAddress ||
                    ip.isLoopbackAddress ||
                    ip.isLinkLocalAddress ||
                    ip.isSiteLocalAddress ||
                    ip.isMulticastAddress
            )
                return false
            val bytes = ip.address.map { it.toInt() and 255 }
            return when (bytes.size) {
                4 ->
                    bytes[0] != 0 &&
                        bytes[0] < 224 &&
                        !(bytes[0] == 100 && bytes[1] in 64..127) &&
                        !(bytes[0] == 198 && bytes[1] in 18..19)
                // Accept global unicast only; rejects ULA, IPv4 translation and scoped addresses.
                16 ->
                    bytes[0] in 0x20..0x3f &&
                        !(bytes[0] == 0x20 && bytes[1] == 0x02) &&
                        !(bytes.take(4) == listOf(0x20, 0x01, 0, 0))
                else -> false
            }
        }
    }
}
