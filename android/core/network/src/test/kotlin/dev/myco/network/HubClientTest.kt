package dev.myco.network

import dev.myco.model.*
import java.util.concurrent.TimeUnit
import kotlinx.coroutines.*
import kotlinx.coroutines.test.runTest
import kotlinx.serialization.json.*
import okhttp3.mockwebserver.*
import org.junit.Assert.*
import org.junit.Test

class HubClientTest {
    @Test
    fun joinLinkPreservesWorkspacePrefixAndDoesNotRevealCredential() {
        val link = JoinLink.parse("https://hub.test/w/team/c/myco_pair_test")
        assertEquals("https://hub.test/w/team", link.baseUrl)
        assertEquals("myco_pair_test", link.credential)
        assertFalse(link.toString().contains("myco_pair_test"))
        assertEquals(
            link.baseUrl,
            JoinLink.parse(
                    "myco://join?link=https%3A%2F%2Fhub.test%2Fw%2Fteam%2Fc%2Fmyco_pair_test"
                )
                .baseUrl,
        )
        assertThrows(IllegalArgumentException::class.java) {
            JoinLink.parse("https://user:password@hub.test/c/myco_pair_test")
        }
    }

    @Test
    fun consoleContractUsesErrorFieldAndKeepsWorkspacePrefix() = runTest {
        MockWebServer().use { server ->
            server.enqueue(
                MockResponse()
                    .setResponseCode(403)
                    .setBody(
                        """{"error":"permission_denied","message":"Not yours","suggestion":"Ask the owner","retryable":false}"""
                    )
            )
            val hub = HubClient(server.url("/w/team").toString(), "test-device")
            try {
                hub.whoami()
                fail("must refuse")
            } catch (e: HubException) {
                assertEquals("permission_denied", e.failure.error)
                assertEquals(403, e.status)
            }
            val request = server.takeRequest()
            assertEquals("/w/team/v1/console/me", request.path)
            assertEquals("Bearer test-device", request.getHeader("Authorization"))
        }
    }

    @Test
    fun sendAndPaginationMatchHubContract() = runTest {
        MockWebServer().use { server ->
            server.enqueue(
                MockResponse()
                    .setBody(
                        """{"message":{"id":"m","from":"@console:person","to":"agent","thread_id":"thread","status":"queued","created_at":"2026-11-22T00:00:00Z"}}"""
                    )
            )
            server.enqueue(MockResponse().setBody("""{"messages":null,"more":true}"""))
            val hub = HubClient(server.url("/").toString(), "test")
            assertEquals(
                "m",
                hub.send(
                        SendRequest("agent", "Hi", after = "earlier", attachments = listOf("file"))
                    )
                    .id,
            )
            val sent = HubJson.parseToJsonElement(server.takeRequest().body.readUtf8()).jsonObject
            assertEquals("earlier", sent["after"]?.jsonPrimitive?.content)
            assertEquals("file", sent["attachments"]?.jsonArray?.first()?.jsonPrimitive?.content)
            val page = hub.exchanges("@console:person", "agent", 1234)
            assertTrue(page.more)
            assertTrue(page.messages.isEmpty())
            assertTrue(server.takeRequest().path!!.contains("before=1234"))
        }
    }

    @Test
    fun authenticatedRedirectIsNeverFollowed() = runTest {
        MockWebServer().use { workspace ->
            MockWebServer().use { other ->
                workspace.enqueue(
                    MockResponse().setResponseCode(302).setHeader("Location", other.url("/steal"))
                )
                try {
                    HubClient(workspace.url("/").toString(), "private").whoami()
                    fail("redirect must be refused")
                } catch (e: HubException) {
                    assertEquals(302, e.status)
                }
                assertNull(other.takeRequest(100, TimeUnit.MILLISECONDS))
            }
        }
    }

    @Test
    fun cancellationClosesTheNetworkCall() = runTest {
        MockWebServer().use { server ->
            server.enqueue(MockResponse().setSocketPolicy(SocketPolicy.NO_RESPONSE))
            val job =
                launch(Dispatchers.IO) { HubClient(server.url("/").toString(), "test").whoami() }
            withContext(Dispatchers.IO) { assertNotNull(server.takeRequest(5, TimeUnit.SECONDS)) }
            job.cancelAndJoin()
            assertTrue(job.isCancelled)
        }
    }
}
