package dev.myco.network

import dev.myco.model.*
import java.io.File
import kotlinx.coroutines.test.runTest
import kotlinx.serialization.json.*
import org.junit.Assert.*
import org.junit.Assume.assumeTrue
import org.junit.Test

/** Run with tools/hub-fixture.py: both sides here use the actual Go API contract. */
class HubIntegrationTest {
    @Test
    fun aRealWorkspacePairsSendsUploadsAndReadsItsOwnConversation() = runTest {
        val path = System.getenv("MYCO_ANDROID_FIXTURE")
        assumeTrue("Use tools/hub-fixture.py to start an isolated Hub", path != null)
        val config = HubJson.parseToJsonElement(File(path!!).readText()).jsonObject
        fun value(key: String) = config.getValue(key).jsonPrimitive.content
        val console = HubClient(value("url"), value("console"))
        val agent = HubClient(value("url"), value("agent"))
        val identity = console.whoami()
        assertTrue(identity.sender.startsWith("@console:"))
        assertTrue(console.overview().agents.any { it.agent.name == value("name") })
        val bytes = "Android attachment".toByteArray()
        val file = console.upload("notes.txt", "text/plain", bytes)
        val task =
            console.send(
                SendRequest(value("name"), "A real request", attachments = listOf(file.id))
            )
        assertEquals("queued", task.status)
        val inbox =
            agent.request(
                "POST",
                listOf("v1", "inbox", "pull"),
                body = buildJsonObject { put("limit", 10) },
            )
        assertEquals(
            task.id,
            inbox.jsonObject
                .getValue("messages")
                .jsonArray
                .first()
                .jsonObject
                .getValue("id")
                .jsonPrimitive
                .content,
        )
        agent.request("POST", listOf("v1", "messages", task.id, "ack"))
        agent.request(
            "POST",
            listOf("v1", "messages", task.id, "progress"),
            body = buildJsonObject { put("body", "Working on it") },
        )
        console.steer(task.id, "A correction")
        agent.request(
            "POST",
            listOf("v1", "messages", task.id, "reply"),
            body = buildJsonObject { put("body", "A real answer") },
        )
        val conversation = console.exchanges(identity.sender, value("name")).messages
        assertTrue(conversation.any { it.type == "reply" && it.body == "A real answer" })
        assertTrue(conversation.any { it.type == "steer" && it.body == "A correction" })
        assertTrue(conversation.any { it.id == task.id && it.status == "done" })
        assertArrayEquals(bytes, console.file(file.id))
        assertTrue(task.id in console.setHidden(listOf(task.id), emptyList()))
        assertFalse(task.id in console.setHidden(emptyList(), listOf(task.id)))
        try {
            agent.whoami()
            fail("Agent tokens must not act as people")
        } catch (error: HubException) {
            assertEquals("permission_denied", error.failure.error)
        }
    }
}
