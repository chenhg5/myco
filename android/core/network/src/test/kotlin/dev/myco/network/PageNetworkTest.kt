package dev.myco.network

import kotlinx.coroutines.test.runTest
import kotlinx.serialization.json.*
import okhttp3.mockwebserver.*
import org.junit.Assert.*
import org.junit.Test

class PageNetworkTest {
    @Test
    fun pageRequestsAreBoundedAndNeverCarryWorkspaceCredentials() = runTest {
        MockWebServer().use { server ->
            server.enqueue(MockResponse().setHeader("Content-Type", "text/plain").setBody("abcdef"))
            val result =
                PageNetwork()
                    .fetch(
                        buildJsonObject {
                            put("url", server.url("/catalog").toString())
                            put("max_bytes", 3)
                        }
                    )
            assertEquals("abc", result["body"]!!.jsonPrimitive.content)
            assertTrue(result["truncated"]!!.jsonPrimitive.boolean)
            val request = server.takeRequest()
            assertNull(request.getHeader("Authorization"))
            assertNull(request.getHeader("Cookie"))
        }
    }

    @Test
    fun aBinaryResponseUsesTheSharedBridgeShape() = runTest {
        MockWebServer().use { server ->
            server.enqueue(
                MockResponse().setHeader("Content-Type", "application/octet-stream").setBody("data")
            )
            val result =
                PageNetwork().fetch(buildJsonObject { put("url", server.url("/video").toString()) })
            assertEquals("base64", result["encoding"]!!.jsonPrimitive.content)
            assertEquals("ZGF0YQ==", result["body"]!!.jsonPrimitive.content)
        }
    }
}
