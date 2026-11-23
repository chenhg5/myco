package dev.myco.network

import java.io.IOException
import java.net.*
import java.util.concurrent.*
import java.util.concurrent.atomic.AtomicInteger
import okhttp3.Dns
import okhttp3.mockwebserver.*
import org.junit.Assert.*
import org.junit.Test

class PageMediaTest {
    private fun dns(resolve: (String) -> List<InetAddress>) =
        object : Dns {
            override fun lookup(hostname: String) = resolve(hostname)
        }

    private val publicIP = InetAddress.getByName("93.184.216.34")

    private fun failure(status: Int, block: () -> Unit) {
        try {
            block()
            fail("Expected media rejection")
        } catch (e: MediaFailure) {
            assertEquals(e.message, status, e.status)
        }
    }

    /** The trusted App proxy is local; the requested media must still resolve publicly. */
    private fun proxy(server: MockWebServer, block: () -> Unit) {
        val previous = ProxySelector.getDefault()
        ProxySelector.setDefault(
            object : ProxySelector() {
                override fun select(uri: URI) =
                    listOf(Proxy(Proxy.Type.HTTP, InetSocketAddress("127.0.0.1", server.port)))

                override fun connectFailed(uri: URI, address: SocketAddress, e: IOException) {}
            }
        )
        try {
            block()
        } finally {
            ProxySelector.setDefault(previous)
        }
    }

    @Test
    fun rangeHeadersAndOnlyMediaResponseHeadersPassThrough() {
        MockWebServer().use { server ->
            server.enqueue(
                MockResponse()
                    .setResponseCode(206)
                    .setBody("part")
                    .setHeader("Content-Type", "audio/mpeg; charset=binary")
                    .setHeader("Content-Range", "bytes 5-8/20")
                    .setHeader("Accept-Ranges", "bytes")
                    .setHeader("ETag", "version")
                    .setHeader("Set-Cookie", "secret=1")
            )
            proxy(server) {
                PageMedia(dns { listOf(publicIP) }).use { media ->
                    val stream =
                        media.open(
                            "http://media.example/episode#location",
                            "GET",
                            mapOf(
                                "Range" to "bytes=5-8",
                                "If-Range" to "version",
                                "Authorization" to "never-forward",
                                "Cookie" to "never-forward",
                            ),
                        )
                    assertEquals(206, stream.status)
                    assertEquals("audio/mpeg", stream.type)
                    assertEquals("bytes 5-8/20", stream.headers["Content-Range"])
                    assertEquals("no-store", stream.headers["Cache-Control"])
                    assertEquals("sandbox", stream.headers["Content-Security-Policy"])
                    assertNull(stream.headers["Set-Cookie"])
                    stream.body.use {
                        assertEquals("part", it.readBytes().decodeToString())
                        assertEquals(-1, it.read())
                    }
                    val request = server.takeRequest()
                    assertTrue(request.requestLine.contains("http://media.example/episode "))
                    assertEquals("bytes=5-8", request.getHeader("Range"))
                    assertEquals("version", request.getHeader("If-Range"))
                    assertEquals("identity", request.getHeader("Accept-Encoding"))
                    listOf("Authorization", "Cookie", "Referer", "Origin", "Proxy-Authorization")
                        .forEach {
                            assertNull(it, request.getHeader(it))
                        }
                }
            }
        }
    }

    @Test
    fun headAndUnsatisfiableRangesKeepTheirStatus() {
        MockWebServer().use { server ->
            server.enqueue(
                MockResponse()
                    .setHeader("Content-Type", "video/mp4")
                    .setHeader("Content-Length", "50")
            )
            server.enqueue(
                MockResponse().setResponseCode(416).setHeader("Content-Range", "bytes */50")
            )
            proxy(server) {
                PageMedia(dns { listOf(publicIP) }).use { media ->
                    media.open("http://media.example/video", "HEAD", emptyMap()).body.close()
                    assertEquals("HEAD", server.takeRequest().method)
                    val stream =
                        media.open(
                            "http://media.example/video",
                            "GET",
                            mapOf("Range" to "bytes=99-"),
                        )
                    assertEquals(416, stream.status)
                    assertEquals("bytes */50", stream.headers["Content-Range"])
                    stream.body.close()
                }
            }
        }
    }

    @Test
    fun privateInitialTargetsRedirectsAndMixedDnsAnswersAreRejected() {
        listOf(
                "127.0.0.1",
                "2130706433",
                "10.1.2.3",
                "172.16.0.1",
                "192.168.1.1",
                "169.254.169.254",
                "100.64.0.1",
                "0.0.0.0",
                "224.0.0.1",
                "[::1]",
                "[fc00::1]",
                "[fe80::1]",
                "[::ffff:127.0.0.1]",
                "localhost",
                "host.local",
                "host.localhost.",
            )
            .forEach { host ->
                PageMedia().use { media ->
                    failure(403) { media.open("http://$host/file", "GET", emptyMap()) }
                }
            }
        PageMedia(dns { listOf(publicIP, InetAddress.getByName("10.0.0.1")) }).use { media ->
            failure(403) { media.open("https://mixed.example/file", "GET", emptyMap()) }
        }
        MockWebServer().use { server ->
            server.enqueue(
                MockResponse()
                    .setResponseCode(302)
                    .setHeader("Location", "http://private.example/secret")
            )
            proxy(server) {
                PageMedia(
                        dns { host ->
                            if (host == "private.example")
                                listOf(InetAddress.getByName("127.0.0.1"))
                            else listOf(publicIP)
                        }
                    )
                    .use { media ->
                        failure(403) { media.open("http://media.example/file", "GET", emptyMap()) }
                        assertEquals(1, server.requestCount)
                    }
            }
        }
    }

    @Test
    fun dnsIsValidatedAgainOnTheAddressesUsedForADirectConnection() {
        val lookups = AtomicInteger()
        val previous = ProxySelector.getDefault()
        ProxySelector.setDefault(
            object : ProxySelector() {
                override fun select(uri: URI) = listOf(Proxy.NO_PROXY)

                override fun connectFailed(uri: URI, address: SocketAddress, e: IOException) {}
            }
        )
        try {
            PageMedia(
                    dns {
                        if (lookups.incrementAndGet() == 1) listOf(publicIP)
                        else listOf(InetAddress.getByName("127.0.0.1"))
                    }
                )
                .use { media ->
                    failure(403) { media.open("http://rebind.example/file", "GET", emptyMap()) }
                    assertEquals(2, lookups.get())
                }
        } finally {
            ProxySelector.setDefault(previous)
        }
    }

    @Test
    fun credentialsUnsupportedSchemesTypesAndRedirectLoopsFailClosed() {
        listOf(
                "file:///etc/passwd",
                "data:audio/mpeg;base64,aA==",
                "ftp://media.example/file",
                "http://user:pass@media.example/file",
                "https://media.example/" + "a".repeat(4096),
            )
            .forEach {
                failure(400) { PageMedia.target(it) }
            }
        MockWebServer().use { server ->
            server.enqueue(
                MockResponse()
                    .setBody("<script>bad</script>")
                    .setHeader("Content-Type", "text/html")
            )
            repeat(8) {
                server.enqueue(MockResponse().setResponseCode(302).setHeader("Location", "/again"))
            }
            proxy(server) {
                PageMedia(dns { listOf(publicIP) }).use { media ->
                    failure(415) { media.open("http://media.example/html", "GET", emptyMap()) }
                    failure(502) { media.open("http://media.example/loop", "GET", emptyMap()) }
                    failure(405) { media.open("http://media.example/file", "POST", emptyMap()) }
                }
            }
        }
        listOf(
                "audio/mpeg",
                "video/mp4",
                "application/ogg",
                "application/octet-stream",
                "binary/octet-stream",
                "application/vnd.apple.mpegurl",
                "application/x-mpegurl",
            )
            .forEach {
                assertTrue(it, PageMedia.mediaType(it))
            }
    }

    @Test
    fun streamingStartsBeforeTheBodyAndCloseOrPauseCancelsBlockedReads() {
        MockWebServer().use { server ->
            repeat(2) {
                server.enqueue(
                    MockResponse()
                        .setHeader("Content-Type", "audio/mpeg")
                        .setBody("delayed")
                        .setBodyDelay(3, TimeUnit.SECONDS)
                )
            }
            proxy(server) {
                PageMedia(dns { listOf(publicIP) }).use { media ->
                    val executor = Executors.newSingleThreadExecutor()
                    try {
                        repeat(2) { attempt ->
                            val stream =
                                executor
                                    .submit<MediaStream> {
                                        media.open("http://media.example/file", "GET", emptyMap())
                                    }
                                    .get(2, TimeUnit.SECONDS)
                            val reading =
                                executor.submit<Boolean> {
                                    try {
                                        stream.body.read()
                                        false
                                    } catch (_: IOException) {
                                        true
                                    }
                                }
                            Thread.sleep(100)
                            if (attempt == 0) stream.body.close() else media.pause()
                            assertTrue(reading.get(2, TimeUnit.SECONDS))
                            stream.body.close()
                            if (attempt == 1) {
                                failure(503) {
                                    media.open("http://media.example/file", "GET", emptyMap())
                                }
                                media.resume()
                            }
                        }
                    } finally {
                        executor.shutdownNow()
                    }
                }
            }
        }
    }
}
