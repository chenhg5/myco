package dev.myco.conversation

import dev.myco.model.*
import org.junit.Assert.*
import org.junit.Test

class ConversationRulesTest {
    private val me = "@console:person"

    private fun task(
        id: String = "task",
        status: String = "acked",
        after: String? = null,
        asking: String? = null,
    ) =
        Message(
            id,
            me,
            "agent",
            id,
            after = after,
            status = status,
            asking = asking,
            createdAt = "2026-11-22T00:00:00Z",
        )

    @Test
    fun heldTasksStartANewConversationUnlessTheyAsk() {
        assertNull(ConversationRules.openTask(me, listOf(task(status = "held"))))
        assertNotNull(
            ConversationRules.openTask(me, listOf(task(status = "held", asking = "question")))
        )
    }

    @Test
    fun answerWinsOverLaggingTaskStatus() {
        val answer = Message("answer", "agent", me, "task", "task", type = "reply", status = "done")
        assertNull(ConversationRules.openTask(me, listOf(task(), answer)))
    }

    @Test
    fun queuedFollowupIsNotTheActiveTask() {
        val first = task()
        val next = task("next", status = "queued", after = "task")
        assertEquals("task", ConversationRules.openTask(me, listOf(first, next))?.id)
        assertEquals(listOf(next), ConversationRules.waiting(listOf(first, next)))
        assertEquals(
            listOf(first),
            ConversationRules.visible(ConversationState(messages = listOf(first, next)), me),
        )
    }

    @Test
    fun endingATurnReturnsToANormalMessageBox() {
        val traces = mapOf("task" to listOf(TraceEvent(1, "tool"), TraceEvent(2, "turn_end")))
        assertNull(ConversationRules.openTask(me, listOf(task()), traces))
        assertNotNull(
            ConversationRules.openTask(
                me,
                listOf(task()),
                traces.mapValues { it.value + TraceEvent(3, "tool") },
            )
        )
    }

    @Test
    fun composerUsesTextAndFilesAndNeverStopsAHeldTask() {
        assertEquals(
            ComposerAction.Stop,
            ConversationRules.action(me, ConversationState(messages = listOf(task()))),
        )
        assertEquals(
            ComposerAction.Steer,
            ConversationRules.action(
                me,
                ConversationState(
                    messages = listOf(task()),
                    files = listOf(PendingFile("one", "picture")),
                ),
            ),
        )
        assertEquals(
            ComposerAction.Send,
            ConversationRules.action(
                me,
                ConversationState(messages = listOf(task(status = "held"))),
            ),
        )
    }

    @Test
    fun withdrawnQueuedTaskNeverAppearsAsASentMessage() {
        val withdrawn =
            task("queued", "failed", after = "task").copy(error = "stopped by the sender")
        assertTrue(
            ConversationRules.visible(ConversationState(messages = listOf(withdrawn)), me).isEmpty()
        )
    }

    @Test
    fun timestampsWithFractionalSecondsStillFollowChronologicalOrder() {
        val older = task("older").copy(createdAt = "2026-11-22T00:00:01Z")
        val newer = task("newer").copy(createdAt = "2026-11-22T00:00:01.500Z")
        assertEquals("newer", ConversationRules.openTask(me, listOf(older, newer))?.id)
    }
}
