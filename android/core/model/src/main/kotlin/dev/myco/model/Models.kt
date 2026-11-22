package dev.myco.model

import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable
import kotlinx.serialization.json.Json

val HubJson = Json {
    ignoreUnknownKeys = true
    coerceInputValues = true
    encodeDefaults = false
    explicitNulls = false
}

@Serializable data class Workspace(val id: String, val name: String, val url: String)

@Serializable
data class Credentials(val workspace: Workspace, val token: String) {
    override fun toString() = "Credentials(workspace=${workspace.id})"
}

@Serializable
data class Member(
    val id: String = "",
    val name: String = "owner",
    @SerialName("display_name") val displayName: String? = null,
)

@Serializable
data class Whoami(
    val member: Member? = null,
    val device: Device? = null,
    @SerialName("can_manage") val canManage: Boolean = false,
) {
    val sender
        get() = "@console:${member?.name ?: "owner"}"
}

@Serializable data class Device(val id: String = "", val name: String = "")

@Serializable
data class Binding(val harness: String = "", val environment: String = "", val workdir: String = "")

@Serializable
data class RuntimeModel(
    val id: String,
    val name: String? = null,
    val efforts: List<String> = emptyList(),
)

@Serializable
data class Runtime(
    val cli: String = "",
    val model: String = "",
    val effort: String = "",
    val access: String = "",
    val models: List<RuntimeModel> = emptyList(),
)

@Serializable
data class Agent(
    val id: String,
    val name: String,
    @SerialName("display_name") val displayName: String? = null,
    val description: String = "",
    val online: Boolean = false,
    val kind: String = "agent",
    val binding: Binding = Binding(),
    val runtime: Runtime? = null,
    val owner: String? = null,
    @SerialName("owner_id") val ownerId: String? = null,
    @SerialName("status_text") val statusText: String? = null,
) {
    val title
        get() = displayName?.takeIf { it.isNotBlank() } ?: name
}

@Serializable
data class AgentStatus(
    val agent: Agent,
    val state: String = "idle",
    val receiving: String = "runner",
    @SerialName("session_in_use") val sessionInUse: Boolean = false,
    val current: List<Message> = emptyList(),
)

@Serializable data class Overview(val agents: List<AgentStatus> = emptyList())

@Serializable
data class ProjectMember(val name: String, val role: String = "member", val note: String = "")

@Serializable
data class Project(
    val name: String,
    val title: String = "",
    val goal: String = "",
    val brief: String = "",
    val members: List<ProjectMember> = emptyList(),
) {
    val displayTitle
        get() = title.ifBlank { name }
}

@Serializable
data class Attachment(
    @SerialName("file_id") val fileId: String,
    val name: String,
    val size: Long = 0,
    @SerialName("content_type") val contentType: String = "application/octet-stream",
)

@Serializable
data class HubFile(
    val id: String,
    val name: String,
    val size: Long = 0,
    @SerialName("content_type") val contentType: String = "application/octet-stream",
)

@Serializable
data class Message(
    val id: String,
    val from: String,
    val to: String,
    @SerialName("thread_id") val threadId: String = "",
    @SerialName("reply_to") val replyTo: String? = null,
    val after: String? = null,
    val type: String = "task",
    val body: String = "",
    val status: String = "queued",
    val error: String? = null,
    val project: String? = null,
    val e2e: Boolean = false,
    @SerialName("created_at") val createdAt: String = "",
    @SerialName("updated_at") val updatedAt: String = "",
    val asking: String? = null,
    val ask: Boolean = false,
    val suggest: String? = null,
    val attachments: List<Attachment> = emptyList(),
) {
    val isOpen
        get() = type == "task" && status in setOf("queued", "delivered", "acked", "held")

    val readableBody
        get() = if (e2e) "Encrypted message" else body
}

@Serializable
data class MessagePage(val messages: List<Message> = emptyList(), val more: Boolean = false)

@Serializable
data class Recent(
    val kind: String = "thread",
    @SerialName("thread_id") val threadId: String = "",
    @SerialName("updated_at") val updatedAt: String = "",
    val last: Message,
    val participants: List<String> = emptyList(),
    val task: Message? = null,
    val open: Message? = null,
    val project: String? = null,
)

@Serializable
data class SendRequest(
    val to: String,
    val body: String,
    @SerialName("thread_id") val threadId: String? = null,
    val project: String? = null,
    val after: String? = null,
    val attachments: List<String> = emptyList(),
)

@Serializable
data class TraceEvent(
    val seq: Int = 0,
    val kind: String = "",
    val at: String = "",
    @SerialName("data") val step: TraceStep? = null,
)

@Serializable
data class TraceStep(
    val text: String? = null,
    val tool: String? = null,
    val input: String? = null,
    val output: String? = null,
    val failed: Boolean = false,
)

@Serializable data class Trace(val events: List<TraceEvent> = emptyList())

@Serializable
data class HubFailure(
    val error: String = "request_failed",
    val message: String = "Request failed",
    val suggestion: String = "",
    val retryable: Boolean = false,
    val latest: kotlinx.serialization.json.JsonElement? = null,
)

class HubException(val status: Int, val failure: HubFailure) : Exception(failure.message)

/** Per-workspace port; implementations own transport, never UI or persistence. */
interface Hub {
    suspend fun whoami(): Whoami

    suspend fun overview(): Overview

    suspend fun projects(): List<Project>

    suspend fun recent(query: String = ""): List<Recent>

    suspend fun exchanges(a: String, b: String, before: Long? = null): MessagePage

    suspend fun send(request: SendRequest): Message

    suspend fun steer(id: String, body: String, attachments: List<String> = emptyList())

    suspend fun stop(id: String)

    suspend fun trace(id: String, after: Int = 0): Trace

    suspend fun upload(name: String, contentType: String, data: ByteArray): HubFile

    suspend fun file(id: String): ByteArray

    suspend fun hidden(): List<String>

    suspend fun setHidden(hide: List<String>, show: List<String>): List<String>
}

interface WorkspaceStore {
    suspend fun workspaces(): List<Workspace>

    suspend fun credentials(id: String): Credentials?

    suspend fun save(credentials: Credentials)

    suspend fun remove(id: String)

    suspend fun selected(): String?

    suspend fun select(id: String?)
}

@Serializable
data class PendingSend(
    val id: String,
    val body: String,
    val files: List<HubFile>,
    val error: String? = null,
    val createdAt: String = java.time.Instant.now().toString(),
)

interface ConversationStore {
    suspend fun pending(key: String): List<PendingSend>

    suspend fun savePending(key: String, pending: List<PendingSend>)

    suspend fun messages(key: String): List<Message>

    suspend fun saveMessages(key: String, messages: List<Message>)

    suspend fun draft(key: String): String

    suspend fun saveDraft(key: String, text: String)
}
