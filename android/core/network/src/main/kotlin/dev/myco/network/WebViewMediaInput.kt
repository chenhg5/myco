package dev.myco.network

import java.io.FilterInputStream

/**
 * Chromium's intercepted-stream loader applies Range even when the server already returned 206.
 * Represent its initial skip without downloading, buffering or allocating the missing prefix.
 * available() is used as the virtual full-file length, not just the current network buffer size.
 * See components/embedder_support/android/util/input_stream_reader.cc in Chromium.
 */
class WebViewMediaInput(stream: MediaStream, requestRange: String? = null) :
    FilterInputStream(stream.body) {
    private val contentRange = stream.headers["Content-Range"]
    private var prefix =
        when (stream.status) {
            206 ->
                Regex("bytes (\\d+)-\\d+/.*")
                    .matchEntire(contentRange.orEmpty())
                    ?.groupValues
                    ?.get(1)
                    ?.toLongOrNull() ?: 0L
            416 ->
                Regex("bytes=(\\d+)-.*")
                    .matchEntire(requestRange.orEmpty())
                    ?.groupValues
                    ?.get(1)
                    ?.toLongOrNull() ?: 0L
            else -> 0L
        }
    private val total =
        when (stream.status) {
            206 -> contentRange?.substringAfter('/')?.toLongOrNull()
            416 -> null
            else -> stream.headers["Content-Length"]?.toLongOrNull()
        }

    // Chromium inserts its computed length before appending these headers. Avoid conflicting
    // Content-Length values when it cannot represent the full length in a signed int.
    val headers =
        stream.headers.filterKeys { !it.equals("Content-Length", true) || available() > 0 }

    // Unknown/large lengths must return zero: WebView's size validation uses a signed int.
    override fun available(): Int = total?.takeIf { it in 1..Int.MAX_VALUE.toLong() }?.toInt() ?: 0

    override fun skip(n: Long): Long {
        if (n <= 0) return 0
        if (prefix > 0) {
            // The Java/native bridge also narrows each skip result to a signed int.
            val skipped = minOf(n, prefix, Int.MAX_VALUE.toLong())
            prefix -= skipped
            return skipped
        }
        return super.skip(n)
    }
}
