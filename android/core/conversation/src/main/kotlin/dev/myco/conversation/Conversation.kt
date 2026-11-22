package dev.myco.conversation

import dev.myco.model.*
import java.time.Instant
import java.util.UUID
import kotlinx.coroutines.*
import kotlinx.coroutines.flow.*
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock

private fun timestamp(value: String): Long = runCatching {
    Instant.parse(value).toEpochMilli()
}
    .getOrDefault(0L)

enum class ComposerAction {
    Send,
    Steer,
    Stop,
}

data class PendingFile(
    val localId: String,
    val name: String,
    val file: HubFile? = null,
    val uploading: Boolean = true,
    val error: String? = null,
)

typealias Outgoing = PendingSend

data class ConversationState(
    val messages: List<Message> = emptyList(),
    val draft: String = "",
    val files: List<PendingFile> = emptyList(),
    val outgoing: List<Outgoing> = emptyList(),
    val traces: Map<String, List<TraceEvent>> = emptyMap(),
    val hidden: Set<String> = emptySet(),
    val loaded: Boolean = false,
    val more: Boolean = false,
    val loadingEarlier: Boolean = false,
    val sending: Boolean = false,
    val stale: Boolean = false,
    val failure: String? = null,
)

/**
 * The same task/held/answered rules as iOS ConversationBuilder. Pure and independently testable.
 */
object ConversationRules {
    fun waiting(messages: List<Message>): List<Message> {
        val byId = messages.associateBy { it.id }
        return messages
            .filter { it.isOpen && byId[it.after]?.isOpen == true }
            .sortedBy { timestamp(it.createdAt) }
    }

    fun paused(events: List<TraceEvent>): Boolean {
        val last = events.lastOrNull() ?: return false
        if (last.kind == "usage") return true
        val end = events.indexOfLast { it.kind == "turn_end" }
        return end >= 0 &&
            events.drop(end + 1).none { it.kind in setOf("tool", "text", "thinking") }
    }

    fun openTask(
        me: String,
        messages: List<Message>,
        traces: Map<String, List<TraceEvent>> = emptyMap(),
    ): Message? {
        val waiting = waiting(messages).map { it.id }.toSet()
        val answered =
            messages.filter { it.type in setOf("reply", "event") }.mapNotNull { it.replyTo }.toSet()
        return messages
            .filter { it.from == me && it.isOpen && it.id !in waiting && it.id !in answered }
            .filter {
                !it.asking.isNullOrBlank() ||
                    (it.status != "held" && !paused(traces[it.id].orEmpty()))
            }
            .maxByOrNull { timestamp(it.createdAt) }
    }

    fun action(me: String, state: ConversationState): ComposerAction {
        if (openTask(me, state.messages, state.traces) == null) return ComposerAction.Send
        return if (state.draft.isNotBlank() || state.files.isNotEmpty()) ComposerAction.Steer
        else ComposerAction.Stop
    }

    fun visible(state: ConversationState, me: String): List<Message> {
        val waiting = waiting(state.messages).map { it.id }.toSet()
        return state.messages.filter {
            it.id !in state.hidden &&
                it.id !in waiting &&
                !(it.from == me &&
                    it.after != null &&
                    it.status == "failed" &&
                    it.error == "stopped by the sender")
        }
    }
}

/** One screen owns one controller; cancelling its scope cancels every transport request. */
class ConversationController(
    val hub: Hub,
    val me: String,
    val agent: String,
    private val key: String,
    private val store: ConversationStore,
    private val scope: CoroutineScope,
    private val project: String? = null,
) {
    private val mutable = MutableStateFlow(ConversationState())
    val state = mutable.asStateFlow()
    private val refreshLock = Mutex()
    private val actionLock = Mutex()
    private var failures = 0
    private var poll: Job? = null
    private var details: Job? = null
    private var restored = false
    private var edited = false
    private val pendingLock = Mutex()
    private var draftSave: Job? = null
    private var earlierLoaded = false
    private var startedProjectThread: String? = null

    suspend fun restore() {
        if (restored) return
        val cached = recover(emptyList<Message>()) { store.messages(key) }
        val draft = recover("") { store.draft(key) }
        val pending =
            recover(emptyList<PendingSend>()) { store.pending(key) }
                .map {
                    it.copy(
                        error =
                            it.error
                                ?: "Sending was interrupted. Check the conversation before trying again."
                    )
                }
        mutable.update {
            it.copy(
                messages = cached,
                draft = if (!edited) draft else it.draft,
                outgoing = pending,
                loaded = cached.isNotEmpty(),
            )
        }
        restored = true
    }

    fun setDraft(value: String) {
        edited = true
        mutable.update { it.copy(draft = value) }
        draftSave?.cancel()
        draftSave = scope.launch {
            delay(250)
            recover(Unit) { store.saveDraft(key, value) }
        }
    }

    fun start() {
        if (poll?.isActive == true) return
        poll = scope.launch {
            restore()
            launch {
                try {
                    val ids = hub.hidden()
                    mutable.update { it.copy(hidden = ids.toSet()) }
                } catch (e: CancellationException) {
                    throw e
                } catch (_: Exception) {}
            }
            while (isActive) {
                refresh()
                delay(2_000)
            }
        }
    }

    fun stopPolling() {
        poll?.cancel()
        poll = null
        details?.cancel()
        details = null
        val draft = mutable.value.draft
        draftSave?.cancel()
        scope.launch(NonCancellable) { recover(Unit) { store.saveDraft(key, draft) } }
    }

    private suspend fun <T> recover(default: T, operation: suspend () -> T): T =
        try {
            operation()
        } catch (e: CancellationException) {
            throw e
        } catch (_: Exception) {
            default
        }

    private suspend fun persistPending() = pendingLock.withLock {
        store.savePending(key, mutable.value.outgoing)
    }

    suspend fun refresh(userInitiated: Boolean = false) = refreshLock.withLock {
        try {
            val page = hub.exchanges(me, agent)
            merge(page.messages)
            mutable.update {
                it.copy(
                    loaded = true,
                    stale = false,
                    failure = if (userInitiated) null else it.failure,
                    more = if (earlierLoaded) it.more else page.more,
                )
            }
            failures = 0
            // Details are independent: a slow trace never delays a newly received reply.
            if (details?.isActive != true) details = scope.launch { loadTraces() }
            runCatching {
                store.saveMessages(key, mutable.value.messages)
                persistPending()
            }
        } catch (e: CancellationException) {
            throw e
        } catch (e: Exception) {
            failures++
            mutable.update {
                it.copy(
                    loaded = true,
                    stale = userInitiated || failures >= 2 || it.messages.isEmpty(),
                    failure = if (userInitiated || it.messages.isEmpty()) words(e) else it.failure,
                )
            }
        }
    }

    private fun merge(incoming: List<Message>) {
        mutable.update { state ->
            val known = state.messages.associateBy { it.id }.toMutableMap()
            incoming.forEach { message ->
                val old = known[message.id]
                if (old == null || timestamp(old.updatedAt) <= timestamp(message.updatedAt))
                    known[message.id] = message
            }
            val sorted =
                known.values.sortedWith(
                    compareBy<Message> { timestamp(it.createdAt) }.thenBy { it.id }
                )
            val candidates = incoming.distinctBy { it.id }.toMutableList()
            val pending =
                state.outgoing.filterNot { local ->
                    val match = candidates.indexOfFirst {
                        it.from == me &&
                            it.body == local.body &&
                            timestamp(it.createdAt) >= timestamp(local.createdAt) &&
                            it.attachments.map { file -> file.fileId }.toSet() ==
                                local.files.map { file -> file.id }.toSet()
                    }
                    if (match >= 0) {
                        candidates.removeAt(match)
                        true
                    } else false
                }
            state.copy(messages = sorted, outgoing = pending)
        }
    }

    private val traceLock = Mutex()

    private suspend fun loadTraces() {
        if (!traceLock.tryLock()) return
        try {
            val tasks =
                mutable.value.messages.filter { it.type == "task" && it.from == me }.takeLast(6)
            supervisorScope {
                tasks
                    .map { task ->
                        launch {
                            try {
                                val old = mutable.value.traces[task.id].orEmpty()
                                if (!task.isOpen && old.isNotEmpty()) return@launch
                                val trace = hub.trace(task.id, old.lastOrNull()?.seq ?: 0)
                                mutable.update {
                                    it.copy(
                                        traces =
                                            it.traces +
                                                (task.id to
                                                    (old + trace.events)
                                                        .distinctBy { e -> e.seq }
                                                        .sortedBy { e -> e.seq })
                                    )
                                }
                            } catch (e: CancellationException) {
                                throw e
                            } catch (_: Exception) {}
                        }
                    }
                    .joinAll()
            }
        } finally {
            traceLock.unlock()
        }
    }

    suspend fun earlier() {
        if (mutable.value.loadingEarlier || !mutable.value.more) return
        mutable.update { it.copy(loadingEarlier = true) }
        try {
            val oldest =
                mutable.value.messages.firstOrNull()?.createdAt?.let {
                    Instant.parse(it).toEpochMilli()
                }
            val page = hub.exchanges(me, agent, oldest)
            earlierLoaded = true
            merge(page.messages)
            mutable.update { it.copy(more = page.more) }
        } catch (e: CancellationException) {
            throw e
        } catch (e: Exception) {
            mutable.update { it.copy(failure = words(e)) }
        } finally {
            mutable.update { it.copy(loadingEarlier = false) }
        }
    }

    suspend fun submit(queue: Boolean = false) {
        if (!actionLock.tryLock()) return
        try {
            val current = mutable.value
            val action = ConversationRules.action(me, current)
            val open = ConversationRules.openTask(me, current.messages, current.traces)
            if (action == ComposerAction.Stop && !queue) {
                mutable.update { it.copy(sending = true) }
                hub.stop(requireNotNull(open).id)
                refresh()
                return
            }
            if (current.files.any { it.uploading || it.error != null }) return
            val body = current.draft.trim()
            val files = current.files.mapNotNull { it.file }
            if (body.isEmpty() && files.isEmpty()) return
            val local = Outgoing(UUID.randomUUID().toString(), body, files)
            // Persist before sending: an interrupted process must not lose the person's words.
            mutable.update {
                it.copy(
                    sending = true,
                    draft = "",
                    files = emptyList(),
                    outgoing = it.outgoing + local,
                    failure = null,
                )
            }
            draftSave?.cancel()
            try {
                persistPending()
                recover(Unit) { store.saveDraft(key, mutable.value.draft) }
                if (action == ComposerAction.Steer && !queue) {
                    hub.steer(requireNotNull(open).id, body, files.map { it.id })
                    // Retain the local bubble until an authoritative read finishes.
                    refresh()
                } else {
                    val thread =
                        if (project == null) current.messages.lastOrNull()?.threadId
                        else startedProjectThread
                    val sent =
                        hub.send(
                            SendRequest(
                                agent,
                                body,
                                thread?.takeIf { it.isNotEmpty() },
                                project,
                                if (queue) open?.id else null,
                                files.map { it.id },
                            )
                        )
                    if (project != null) startedProjectThread = sent.threadId
                    merge(listOf(sent))
                }
                if (
                    action != ComposerAction.Steer ||
                        queue ||
                        mutable.value.messages.any {
                            it.from == me &&
                                it.type == "steer" &&
                                it.body == body &&
                                timestamp(it.createdAt) >= timestamp(local.createdAt)
                        }
                ) {
                    mutable.update {
                        it.copy(outgoing = it.outgoing.filterNot { o -> o.id == local.id })
                    }
                }
            } catch (e: CancellationException) {
                throw e
            } catch (e: Exception) {
                mutable.update {
                    it.copy(
                        outgoing =
                            it.outgoing.map { o ->
                                if (o.id == local.id) o.copy(error = words(e)) else o
                            }
                    )
                }
            }
        } catch (e: CancellationException) {
            throw e
        } catch (e: Exception) {
            mutable.update { it.copy(failure = words(e)) }
        } finally {
            withContext(NonCancellable) {
                runCatching { persistPending() }
            }
            mutable.update { it.copy(sending = false) }
            actionLock.unlock()
        }
    }

    fun recoverOutgoing(id: String) {
        val item = mutable.value.outgoing.find { it.id == id } ?: return
        if (mutable.value.draft.isNotBlank()) return
        mutable.update {
            it.copy(
                draft = item.body,
                files = item.files.map { f -> PendingFile(f.id, f.name, f, false) },
                outgoing = it.outgoing.filterNot { o -> o.id == id },
            )
        }
        setDraft(item.body)
        scope.launch { recover(Unit) { persistPending() } }
    }

    suspend fun attach(name: String, type: String, bytes: ByteArray) {
        val id = UUID.randomUUID().toString()
        mutable.update { it.copy(files = it.files + PendingFile(id, name)) }
        try {
            val file = hub.upload(name, type, bytes)
            mutable.update {
                it.copy(
                    files =
                        it.files.map { f ->
                            if (f.localId == id) f.copy(file = file, uploading = false) else f
                        }
                )
            }
        } catch (e: CancellationException) {
            throw e
        } catch (e: Exception) {
            mutable.update {
                it.copy(
                    files =
                        it.files.map { f ->
                            if (f.localId == id) f.copy(uploading = false, error = words(e)) else f
                        }
                )
            }
        }
    }

    fun removeFile(id: String) {
        mutable.update { it.copy(files = it.files.filterNot { f -> f.localId == id }) }
    }

    suspend fun hide(id: String, hidden: Boolean) {
        val ids =
            hub.setHidden(
                if (hidden) listOf(id) else emptyList(),
                if (hidden) emptyList() else listOf(id),
            )
        mutable.update { it.copy(hidden = ids.toSet()) }
    }

    suspend fun withdraw(message: Message, edit: Boolean) {
        hub.stop(message.id)
        merge(
            listOf(
                message.copy(
                    status = "failed",
                    error = "stopped by the sender",
                    updatedAt = Instant.now().toString(),
                )
            )
        )
        if (edit) {
            mutable.update {
                it.copy(
                    files =
                        message.attachments.map { f ->
                            PendingFile(
                                f.fileId,
                                f.name,
                                HubFile(f.fileId, f.name, f.size, f.contentType),
                                false,
                            )
                        }
                )
            }
            setDraft(message.body)
        }
    }

    suspend fun resend(message: Message) {
        merge(
            listOf(
                hub.send(
                    SendRequest(
                        agent,
                        message.body,
                        message.threadId,
                        project,
                        attachments = message.attachments.map { it.fileId },
                    )
                )
            )
        )
    }

    fun clearFailure() {
        mutable.update { it.copy(failure = null) }
    }

    private fun words(e: Exception) =
        when (e) {
            is HubException ->
                listOf(e.failure.message, e.failure.suggestion)
                    .filter { it.isNotBlank() }
                    .joinToString(". ")
            else -> e.message ?: "Could not reach the workspace"
        }
}
