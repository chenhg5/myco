package dev.myco.network

import kotlinx.coroutines.*
import kotlinx.serialization.json.*

interface RemoteTransport {
    suspend fun devices(): JsonArray

    suspend fun connect(machine: String, offer: JsonObject): JsonObject

    suspend fun session(id: String): JsonObject

    suspend fun disconnect(id: String)
}

class HubRemoteTransport(private val hub: HubClient) : RemoteTransport {
    override suspend fun devices() =
        hub.request("GET", listOf("v1", "remote", "machines")).jsonObject["machines"]?.jsonArray
            ?: JsonArray(emptyList())

    override suspend fun connect(machine: String, offer: JsonObject) =
        hub.request(
                "POST",
                listOf("v1", "remote", "sessions"),
                body =
                    buildJsonObject {
                        put("machine", machine)
                        put("offer", offer)
                    },
            )
            .jsonObject

    override suspend fun session(id: String) =
        hub.request("GET", listOf("v1", "remote", "sessions", id)).jsonObject

    override suspend fun disconnect(id: String) {
        hub.request("DELETE", listOf("v1", "remote", "sessions", id))
    }
}

/**
 * One page owns these sessions. Revocation occurs synchronously before cleanup awaits the network.
 */
class RemoteSessions(
    private val transport: RemoteTransport,
    private val scope: CoroutineScope,
    private val ask: suspend (JsonObject, Boolean) -> Boolean,
    private val changed: (Boolean) -> Unit,
) {
    private var generation = 0
    private var suspended = false
    private var closed = false
    private var connecting = false
    private val sessions = mutableSetOf<String>()

    suspend fun call(action: String, args: JsonObject): JsonElement {
        if (action == "close") {
            reset()
            return JsonPrimitive(true)
        }
        check(!closed && !suspended) { "The page is closed or paused. Connect again." }
        fun text(key: String) = args[key]?.jsonPrimitive?.contentOrNull.orEmpty()
        return when (action) {
            "status" ->
                buildJsonObject {
                    put("protocol", 1)
                    put("version", "controller")
                    put("installed", false)
                    put("supported", false)
                    put("running", false)
                    put("reason", "controller_only")
                }
            "devices" -> transport.devices()
            "connect" -> {
                check(!connecting && sessions.isEmpty()) { "End the current connection first" }
                val machine = text("machine")
                val offer = args["offer"] as? JsonObject ?: error("Invalid connection request")
                val description =
                    offer["description"] as? JsonObject ?: error("Invalid connection request")
                require(
                    offer.toString().toByteArray().size <= 70 * 1024 &&
                        description["type"]?.jsonPrimitive?.content == "offer" &&
                        description["sdp"]?.jsonPrimitive?.content?.startsWith("v=0") == true &&
                        (offer["pairing_code"]?.jsonPrimitive?.content?.length ?: 0) <= 32
                ) {
                    "Invalid connection request"
                }
                connecting = true
                val epoch = generation
                try {
                    val computer =
                        transport
                            .devices()
                            .map { it.jsonObject }
                            .firstOrNull { it["machine"]?.jsonPrimitive?.content == machine }
                            ?: error("This computer stopped sharing")
                    val control = offer["control"]?.jsonPrimitive?.booleanOrNull ?: false
                    check(!control || computer["control"]?.jsonPrimitive?.booleanOrNull == true) {
                        "This computer has not allowed keyboard and mouse control"
                    }
                    check(
                        epoch == generation &&
                            !closed &&
                            !suspended &&
                            ask(computer, control) &&
                            epoch == generation &&
                            !closed &&
                            !suspended
                    ) {
                        "The connection was not allowed"
                    }
                    // If cancellation races with a server response, retract the connection before
                    // returning.
                    withContext(NonCancellable) {
                        val result = transport.connect(machine, offer)
                        val id = result["id"]?.jsonPrimitive?.content ?: error("Missing session")
                        if (epoch != generation || closed || suspended) {
                            runCatching { transport.disconnect(id) }
                            error("The page closed; the connection was cancelled")
                        }
                        sessions += id
                        changed(true)
                        result
                    }
                } finally {
                    connecting = false
                }
            }
            "session",
            "disconnect" -> {
                val id = text("id")
                check(id in sessions) { "This connection does not belong to this page" }
                if (action == "disconnect") {
                    sessions -= id
                    if (sessions.isEmpty()) changed(false)
                    transport.disconnect(id)
                    JsonPrimitive(true)
                } else transport.session(id)
            }
            else ->
                error(
                    "This phone can connect to computers; it cannot share its screen or install host components"
                )
        }
    }

    fun reset() {
        generation++
        val ids = sessions.toList()
        sessions.clear()
        if (ids.isNotEmpty()) changed(false)
        scope.launch { ids.forEach { runCatching { transport.disconnect(it) } } }
    }

    fun pause() {
        suspended = true
        reset()
    }

    fun resume() {
        if (!closed) suspended = false
    }

    fun close() {
        closed = true
        reset()
    }
}
