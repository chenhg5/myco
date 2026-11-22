package dev.myco.android

import dev.myco.model.*
import java.time.Instant
import java.util.UUID
import kotlinx.coroutines.delay

/** Offline demonstration. It never sends data or joins a real workspace. */
class DemoHub : Hub {
    private val agent =
        Agent("demo-agent", "willow", "Willow", "Turns ideas into working software", online = true)
    private val messages =
        mutableListOf(
            Message(
                "welcome",
                "willow",
                "@console:demo",
                "welcome",
                type = "note",
                body =
                    "Hello! Tell me what you’re working on.\n\nYou can try messages, attachments, and stopping a task here before joining your workspace.",
                status = "done",
                createdAt = "2026-11-22T01:00:00Z",
                updatedAt = "2026-11-22T01:00:00Z",
            )
        )
    private val files = mutableMapOf<String, ByteArray>()
    private val hidden = mutableSetOf<String>()

    override suspend fun whoami() = Whoami(Member("demo-member", "demo"), canManage = true)

    override suspend fun overview() =
        Overview(listOf(AgentStatus(agent, if (messages.any { it.isOpen }) "working" else "idle")))

    override suspend fun projects() =
        listOf(
            Project(
                "demo",
                "A little idea",
                "Make something useful together",
                "This workspace is on your phone. Join a real workspace whenever you’re ready.",
                listOf(ProjectMember("willow", "coordinator")),
            )
        )

    override suspend fun recent(query: String) =
        listOf(
            Recent(
                threadId = "welcome",
                last = messages.last(),
                participants = listOf("@console:demo", "willow"),
            )
        )

    override suspend fun exchanges(a: String, b: String, before: Long?): MessagePage {
        delay(100)
        val now = Instant.now()
        messages
            .filter {
                it.isOpen &&
                    java.time.Duration.between(Instant.parse(it.createdAt), now).seconds >= 7
            }
            .toList()
            .forEach { task ->
                messages[messages.indexOf(task)] =
                    task.copy(status = "done", updatedAt = now.toString())
                messages +=
                    Message(
                        UUID.randomUUID().toString(),
                        "willow",
                        "@console:demo",
                        task.threadId,
                        task.id,
                        type = "reply",
                        body =
                            "Got it. In your workspace, this is where your agent’s answer arrives.\n\nYour words and files stay with the conversation, and you can follow its steps as it works.",
                        status = "done",
                        createdAt = now.toString(),
                        updatedAt = now.toString(),
                    )
            }
        return MessagePage(messages.sortedByDescending { it.createdAt })
    }

    override suspend fun send(request: SendRequest): Message {
        val now = Instant.now().toString()
        val id = UUID.randomUUID().toString()
        val message =
            Message(
                id,
                "@console:demo",
                request.to,
                request.threadId ?: id,
                after = request.after,
                body = request.body,
                status = "acked",
                createdAt = now,
                updatedAt = now,
                attachments =
                    request.attachments.map {
                        Attachment(it, "Attachment", files[it]?.size?.toLong() ?: 0)
                    },
            )
        messages += message
        return message
    }

    override suspend fun steer(id: String, body: String, attachments: List<String>) {
        val now = Instant.now().toString()
        messages +=
            Message(
                UUID.randomUUID().toString(),
                "@console:demo",
                "willow",
                id,
                id,
                type = "steer",
                body = body,
                status = "done",
                createdAt = now,
                updatedAt = now,
            )
    }

    override suspend fun stop(id: String) {
        val index = messages.indexOfFirst { it.id == id }
        if (index >= 0)
            messages[index] =
                messages[index].copy(
                    status = "failed",
                    error = "stopped by the sender",
                    updatedAt = Instant.now().toString(),
                )
    }

    override suspend fun trace(id: String, after: Int) =
        Trace(
            listOf(TraceEvent(1, "text", step = TraceStep(text = "Thinking about your request")))
                .filter { it.seq > after }
        )

    override suspend fun upload(name: String, contentType: String, data: ByteArray): HubFile {
        val id = UUID.randomUUID().toString()
        files[id] = data
        return HubFile(id, name, data.size.toLong(), contentType)
    }

    override suspend fun file(id: String) = files[id] ?: byteArrayOf()

    override suspend fun hidden() = hidden.toList()

    override suspend fun setHidden(hide: List<String>, show: List<String>): List<String> {
        hidden.addAll(hide)
        hidden.removeAll(show.toSet())
        return hidden.toList()
    }
}

/** The demonstration has no durable workspace identity or history. */
class DemoStore : ConversationStore {
    private val history = mutableMapOf<String, List<Message>>()
    private val drafts = mutableMapOf<String, String>()
    private val outbox = mutableMapOf<String, List<PendingSend>>()

    override suspend fun messages(key: String) = history[key].orEmpty()

    override suspend fun saveMessages(key: String, messages: List<Message>) {
        history[key] = messages
    }

    override suspend fun draft(key: String) = drafts[key].orEmpty()

    override suspend fun saveDraft(key: String, text: String) {
        drafts[key] = text
    }

    override suspend fun pending(key: String) = outbox[key].orEmpty()

    override suspend fun savePending(key: String, pending: List<PendingSend>) {
        outbox[key] = pending
    }
}
