package dev.myco.network

import java.io.ByteArrayInputStream
import org.junit.Assert.*
import org.junit.Test

class WebViewMediaInputTest {
    @Test
    fun chromiumRangeValidationAndSkipDoNotConsumeThePartialBody() {
        val input =
            WebViewMediaInput(
                MediaStream(
                    206,
                    "audio/mpeg",
                    mapOf("Content-Range" to "bytes 20-29/100", "Content-Length" to "10"),
                    ByteArrayInputStream("0123456789".toByteArray()),
                )
            )
        assertEquals(100, input.available())
        assertEquals(20L, input.skip(20))
        assertEquals("0123456789", input.readBytes().decodeToString())
    }

    @Test
    fun rangesBeyondTwoGiBUseBoundedVirtualSkipsAndUnknownSize() {
        val start = 2_202_518_806L
        val input =
            WebViewMediaInput(
                MediaStream(
                    206,
                    "video/mp4",
                    mapOf("Content-Range" to "bytes $start-${start+3}/3000000000"),
                    ByteArrayInputStream("data".toByteArray()),
                )
            )
        assertEquals(0, input.available())
        val first = input.skip(start)
        assertEquals(Int.MAX_VALUE.toLong(), first)
        assertEquals(start - first, input.skip(start - first))
        assertEquals("data", input.readBytes().decodeToString())
    }

    @Test
    fun empty416BodiesAndServersIgnoringRangesKeepTheCorrectStream() {
        val error =
            WebViewMediaInput(
                MediaStream(416, "text/plain", emptyMap(), ByteArrayInputStream(byteArrayOf())),
                "bytes=9999-",
            )
        assertEquals(0, error.available())
        assertEquals(9999L, error.skip(9999))
        assertEquals(-1, error.read())
        val full =
            WebViewMediaInput(
                MediaStream(
                    200,
                    "audio/mpeg",
                    mapOf("Content-Length" to "10"),
                    ByteArrayInputStream("0123456789".toByteArray()),
                )
            )
        assertEquals(10, full.available())
        assertEquals(3L, full.skip(3))
        assertEquals("3456789", full.readBytes().decodeToString())
    }
}
