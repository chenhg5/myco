package dev.myco.conversation

import dev.myco.model.*
import java.io.IOException
import kotlinx.coroutines.*
import kotlinx.coroutines.test.*
import org.junit.Assert.*
import org.junit.Test

class ConversationControllerTest {
    private class Store : ConversationStore {
        var saved = listOf<Message>()
        var text = ""
        var outbox = listOf<PendingSend>()

        override suspend fun messages(key: String) = saved

        override suspend fun saveMessages(key: String, messages: List<Message>) {
            saved = messages
        }

        override suspend fun draft(key: String) = text

        override suspend fun saveDraft(key: String, text: String) {
            this.text = text
        }

        override suspend fun pending(key: String) = outbox

        override suspend fun savePending(key: String, pending: List<PendingSend>) {
            outbox = pending
        }
    }

    private class Server : Hub {
        var messages = listOf<Message>()
        var unavailable = false
        var sent = 0
        var failSend = false
        var blockTrace = false

        override suspend fun whoami() = Whoami(Member(name = "person"))

        override suspend fun overview() = Overview()

        override suspend fun projects() = emptyList<Project>()

        override suspend fun recent(query: String) = emptyList<Recent>()

        override suspend fun exchanges(a: String, b: String, before: Long?): MessagePage {
            if (unavailable) throw IOException("Offline")
            return MessagePage(messages)
        }

        override suspend fun send(request: SendRequest): Message {
            sent++
            if (failSend) throw IOException("Connection lost")
            return Message(
                "new",
                "@console:person",
                "agent",
                body = request.body,
                createdAt = "2026-11-22T00:00:00Z",
            )
        }

        override suspend fun steer(id: String, body: String, attachments: List<String>) {}

        override suspend fun stop(id: String) {}

        override suspend fun trace(id: String, after: Int): Trace {
            if (blockTrace) awaitCancellation()
            return Trace()
        }

        override suspend fun upload(name: String, contentType: String, data: ByteArray) =
            HubFile("file", name, data.size.toLong(), contentType)

        override suspend fun file(id: String) = byteArrayOf()

        override suspend fun hidden() = emptyList<String>()

        override suspend fun setHidden(hide: List<String>, show: List<String>) = hide
    }

    @Test
    fun draftSurvivesAnAmbiguousSendAndIsNeverAutomaticallyRetried() = runTest {
        val store = Store()
        val server = Server().apply { failSend = true }
        val controller =
            ConversationController(
                server,
                "@console:person",
                "agent",
                "workspace:agent",
                store,
                backgroundScope,
            )
        controller.setDraft("Keep my words")
        controller.submit()
        assertEquals(1, server.sent)
        assertEquals("Keep my words", store.outbox.single().body)
        assertNotNull(controller.state.value.outgoing.single().error)
        val reopened =
            ConversationController(
                server,
                "@console:person",
                "agent",
                "workspace:agent",
                store,
                backgroundScope,
            )
        reopened.restore()
        assertNotNull(reopened.state.value.outgoing.single().error)
        reopened.refresh()
        assertEquals(1, server.sent)
        reopened.recoverOutgoing(reopened.state.value.outgoing.single().id)
        assertEquals("Keep my words", reopened.state.value.draft)
    }

    @Test
    fun aSlowTraceDoesNotHoldBackMessagesAndStopsOnBackground() = runTest {
        val server =
            Server().apply {
                messages = listOf(Message("task", "@console:person", "agent", status = "acked"))
                blockTrace = true
            }
        val controller =
            ConversationController(
                server,
                "@console:person",
                "agent",
                "w:a",
                Store(),
                backgroundScope,
            )
        controller.refresh()
        runCurrent()
        assertEquals(1, controller.state.value.messages.size)
        controller.stopPolling()
        runCurrent()
    }

    @Test
    fun aSingleTransientFailureKeepsTheConversationQuiet() = runTest {
        val server =
            Server().apply {
                messages = listOf(Message("reply", "agent", "@console:person", type = "reply"))
            }
        val controller =
            ConversationController(
                server,
                "@console:person",
                "agent",
                "w:a",
                Store(),
                backgroundScope,
            )
        controller.refresh()
        server.unavailable = true
        controller.refresh()
        assertFalse(controller.state.value.stale)
        controller.refresh()
        assertTrue(controller.state.value.stale)
        assertEquals(1, controller.state.value.messages.size)
        server.unavailable = false
        controller.refresh()
        assertFalse(controller.state.value.stale)
    }
}
