package dev.myco.network

import kotlinx.coroutines.*
import kotlinx.coroutines.test.*
import kotlinx.serialization.json.*
import org.junit.Assert.*
import org.junit.Test

@OptIn(ExperimentalCoroutinesApi::class)
class RemoteSessionsTest {
    private class Transport : RemoteTransport {
        var creates = 0
        val removed = mutableListOf<String>()
        var connected: CompletableDeferred<Unit>? = null

        override suspend fun devices() =
            JsonArray(
                listOf(
                    buildJsonObject {
                        put("machine", "actual")
                        put("name", "Trusted computer")
                        put("control", true)
                    }
                )
            )

        override suspend fun connect(machine: String, offer: JsonObject): JsonObject {
            creates++
            connected?.await()
            return buildJsonObject {
                put("id", "session")
                put("machine", machine)
            }
        }

        override suspend fun session(id: String) = buildJsonObject { put("id", id) }

        override suspend fun disconnect(id: String) {
            removed += id
        }
    }

    private fun offer(machine: String = "actual") = buildJsonObject {
        put("machine", machine)
        put(
            "offer",
            buildJsonObject {
                put(
                    "description",
                    buildJsonObject {
                        put("type", "offer")
                        put("sdp", "v=0\r\n")
                    },
                )
                put("control", true)
                put("pairing_code", "123456")
            },
        )
    }

    @Test
    fun pageCannotPickAnUnsharedComputerOrHostItsOwnScreen() = runTest {
        val transport = Transport()
        val remote = RemoteSessions(transport, backgroundScope, { _, _ -> true }, {})
        try {
            remote.call("connect", offer("untrusted"))
            fail("must refuse")
        } catch (_: IllegalStateException) {}
        try {
            remote.call("enable", buildJsonObject {})
            fail("must refuse host")
        } catch (_: IllegalStateException) {}
        assertEquals(0, transport.creates)
    }

    @Test
    fun consentUsesTheComputerNameFromTheWorkspaceAndNotThePage() = runTest {
        val transport = Transport()
        var name = ""
        val remote =
            RemoteSessions(
                transport,
                backgroundScope,
                { computer, _ ->
                    name = computer["name"]!!.jsonPrimitive.content
                    false
                },
                {},
            )
        try {
            remote.call("connect", offer())
            fail("must refuse")
        } catch (_: IllegalStateException) {}
        assertEquals("Trusted computer", name)
        assertEquals(0, transport.creates)
    }

    @Test
    fun aLateConnectIsRetractedAfterThePageCloses() = runTest {
        val transport = Transport().apply { connected = CompletableDeferred() }
        val remote = RemoteSessions(transport, backgroundScope, { _, _ -> true }, {})
        val connect = async { runCatching { remote.call("connect", offer()) } }
        runCurrent()
        remote.close()
        transport.connected!!.complete(Unit)
        runCurrent()
        assertTrue(connect.await().isFailure)
        assertEquals(listOf("session"), transport.removed)
    }

    @Test
    fun sessionsCannotBeReadByAnotherPageAndBackgroundingRevokesThem() = runTest {
        val transport = Transport()
        val remote = RemoteSessions(transport, backgroundScope, { _, _ -> true }, {})
        remote.call("connect", offer())
        remote.pause()
        runCurrent()
        assertEquals(listOf("session"), transport.removed)
        remote.resume()
        try {
            remote.call("session", buildJsonObject { put("id", "session") })
            fail("revoked")
        } catch (_: IllegalStateException) {}
    }
}
