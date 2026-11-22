package dev.myco.workspace

import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.outlined.ArrowBack
import androidx.compose.material.icons.outlined.*
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import dev.myco.network.HubClient
import dev.myco.pages.PageScreen
import dev.myco.ui.*
import kotlinx.coroutines.*
import kotlinx.serialization.json.*

internal fun JsonElement.field(name: String): String =
    (this as? JsonObject)?.get(name)?.jsonPrimitive?.contentOrNull.orEmpty()

internal fun JsonElement.flag(name: String) =
    (this as? JsonObject)?.get(name)?.jsonPrimitive?.booleanOrNull ?: false

@OptIn(ExperimentalMaterial3Api::class)
@Composable
internal fun CollectionScreen(kind: String, hub: HubClient, back: () -> Unit) {
    val title =
        when (kind) {
            "apps" -> words("应用", "Apps")
            "automations" -> words("自动化", "Automations")
            else -> words("工作流", "Workflows")
        }
    var rows by remember(kind) { mutableStateOf<List<JsonElement>>(emptyList()) }
    var pages by remember(kind) { mutableStateOf<List<JsonElement>>(emptyList()) }
    var loaded by remember(kind) { mutableStateOf(false) }
    var problem by remember { mutableStateOf<String?>(null) }
    var detail by remember { mutableStateOf<JsonElement?>(null) }
    var make by remember { mutableStateOf(false) }
    var pendingDelete by remember { mutableStateOf<JsonElement?>(null) }
    var madeApp by remember { mutableStateOf<JsonElement?>(null) }
    var page by remember { mutableStateOf<JsonElement?>(null) }
    var refresh by remember { mutableIntStateOf(0) }
    var busy by remember { mutableStateOf(false) }
    val scope = rememberCoroutineScope()
    val path =
        when (kind) {
            "apps" -> listOf("v1", "templates")
            "automations" -> listOf("v1", "console", "automations")
            else -> listOf("v1", "workflows")
        }
    val key =
        when (kind) {
            "apps" -> "templates"
            "automations" -> "automations"
            else -> "workflows"
        }
    LaunchedEffect(kind, refresh) {
        try {
            rows = hub.request("GET", path).jsonObject[key]?.jsonArray?.toList().orEmpty()
            if (kind == "apps")
                pages =
                    hub.request("GET", listOf("v1", "pages"))
                        .jsonObject["pages"]
                        ?.jsonArray
                        ?.toList()
                        .orEmpty()
            problem = null
        } catch (e: CancellationException) {
            throw e
        } catch (e: Exception) {
            problem = e.message
        } finally {
            loaded = true
        }
    }
    fun action(block: suspend () -> Unit) {
        scope.launch {
            busy = true
            try {
                block()
                refresh++
            } catch (e: CancellationException) {
                throw e
            } catch (e: Exception) {
                problem = e.message
            } finally {
                busy = false
            }
        }
    }
    if (page != null) {
        PageScreen(page!!, hub, { page = null })
        return
    }
    Scaffold(
        topBar = {
            TopAppBar(
                title = { Text(title) },
                navigationIcon = {
                    IconButton(onClick = back) {
                        Icon(Icons.AutoMirrored.Outlined.ArrowBack, words("返回", "Back"))
                    }
                },
                actions = {
                    if (kind == "automations")
                        IconButton(onClick = { make = true }) {
                            Icon(Icons.Outlined.Add, words("新建", "New"))
                        }
                    IconButton(onClick = { refresh++ }) {
                        Icon(Icons.Outlined.Refresh, words("刷新", "Refresh"))
                    }
                },
            )
        }
    ) { padding ->
        Column(Modifier.fillMaxSize().padding(padding)) {
            problem?.let { Problem(it) { refresh++ } }
            if (!loaded || busy) LinearProgressIndicator(Modifier.fillMaxWidth())
            if (loaded && rows.isEmpty() && pages.isEmpty() && problem == null)
                Empty(words("这里还没有内容", "Nothing here yet"))
            LazyColumn(contentPadding = PaddingValues(12.dp)) {
                if (pages.isNotEmpty())
                    item {
                        Text(
                            words("你的应用", "Your apps"),
                            Modifier.padding(16.dp),
                            style = MaterialTheme.typography.labelLarge,
                        )
                    }
                items(pages, key = { "page:" + it.field("id") }) { item ->
                    ListItem(
                        headlineContent = { Text(item.field("name")) },
                        leadingContent = { Icon(Icons.Outlined.Web, null) },
                        trailingContent = { Icon(Icons.Outlined.ChevronRight, null) },
                        modifier = Modifier.clickable { page = item },
                    )
                }
                if (kind == "apps" && rows.isNotEmpty())
                    item {
                        Text(
                            words("应用库", "App library"),
                            Modifier.padding(16.dp),
                            style = MaterialTheme.typography.labelLarge,
                        )
                    }
                items(rows, key = { it.field("id").ifEmpty { it.field("name") } }) { item ->
                    ListItem(
                        headlineContent = {
                            Text(item.field("title").ifEmpty { item.field("name") })
                        },
                        supportingContent = {
                            Text(
                                item.field("description").ifEmpty { item.field("body") },
                                maxLines = 2,
                                overflow = TextOverflow.Ellipsis,
                            )
                        },
                        leadingContent = {
                            Icon(
                                if (kind == "apps") Icons.Outlined.GridView
                                else if (kind == "automations") Icons.Outlined.Schedule
                                else Icons.Outlined.AccountTree,
                                null,
                            )
                        },
                        trailingContent = { Icon(Icons.Outlined.ChevronRight, null) },
                        modifier =
                            Modifier.clickable {
                                if (kind == "apps") madeApp = item else detail = item
                            },
                    )
                }
            }
        }
    }
    if (make)
        AutomationEditor({ make = false }) { body ->
            action {
                hub.request("POST", path, body = body)
                make = false
            }
        }
    if (madeApp != null) {
        var name by remember(madeApp) { mutableStateOf(madeApp!!.field("name")) }
        AlertDialog(
            onDismissRequest = { madeApp = null },
            title = { Text(words("使用应用", "Use app")) },
            text = {
                OutlinedTextField(name, { name = it }, label = { Text(words("名称", "Name")) })
            },
            confirmButton = {
                TextButton(
                    enabled = name.isNotBlank() && !busy,
                    onClick = {
                        action {
                            val created =
                                hub.request(
                                    "POST",
                                    path + listOf(madeApp!!.field("id"), "use"),
                                    body =
                                        buildJsonObject {
                                            put("name", name)
                                            put("sample", true)
                                        },
                                )
                            page = created.jsonObject["page"]
                            madeApp = null
                        }
                    },
                ) {
                    Text(words("打开", "Open"))
                }
            },
            dismissButton = {
                TextButton(onClick = { madeApp = null }) { Text(words("取消", "Cancel")) }
            },
        )
    }
    detail?.let { item ->
        AlertDialog(
            onDismissRequest = { detail = null },
            title = { Text(item.field("name")) },
            text = {
                Column(verticalArrangement = Arrangement.spacedBy(12.dp)) {
                    Text(item.field("body").ifEmpty { item.field("description") })
                    Row {
                        TextButton(
                            enabled = !busy,
                            onClick = {
                                action {
                                    hub.request(
                                        "PATCH",
                                        path + item.field("id"),
                                        body =
                                            buildJsonObject {
                                                put("enabled", !item.flag("enabled"))
                                            },
                                    )
                                    detail = null
                                }
                            },
                        ) {
                            Text(
                                if (item.flag("enabled")) words("停用", "Disable")
                                else words("启用", "Enable")
                            )
                        }
                        if (kind == "automations")
                            TextButton(
                                enabled = !busy,
                                onClick = {
                                    pendingDelete = item
                                    detail = null
                                },
                            ) {
                                Text(words("删除", "Delete"))
                            }
                    }
                }
            },
            confirmButton = {
                TextButton(
                    enabled = !busy,
                    onClick = {
                        action {
                            hub.request(
                                "POST",
                                path +
                                    listOf(
                                        item.field("id"),
                                        if (kind == "automations") "run" else "runs",
                                    ),
                                body =
                                    buildJsonObject {
                                        if (kind != "automations") put("inputs", buildJsonObject {})
                                    },
                            )
                            detail = null
                        }
                    },
                ) {
                    Text(words("立即运行", "Run now"))
                }
            },
            dismissButton = {
                TextButton(onClick = { detail = null }) { Text(words("关闭", "Close")) }
            },
        )
    }
    pendingDelete?.let { item ->
        AlertDialog(
            onDismissRequest = { pendingDelete = null },
            title = { Text(words("删除自动化？", "Delete automation?")) },
            text = { Text(item.field("name")) },
            confirmButton = {
                TextButton(
                    onClick = {
                        action {
                            hub.request("DELETE", path + item.field("id"))
                            pendingDelete = null
                        }
                    }
                ) {
                    Text(words("删除", "Delete"))
                }
            },
            dismissButton = {
                TextButton(onClick = { pendingDelete = null }) { Text(words("取消", "Cancel")) }
            },
        )
    }
}

@Composable
private fun AutomationEditor(close: () -> Unit, save: (JsonObject) -> Unit) {
    var name by remember { mutableStateOf("") }
    var to by remember { mutableStateOf("") }
    var text by remember { mutableStateOf("") }
    var cron by remember { mutableStateOf("0 9 * * 1-5") }
    AlertDialog(
        onDismissRequest = close,
        title = { Text(words("新建自动化", "New automation")) },
        text = {
            Column(verticalArrangement = Arrangement.spacedBy(12.dp)) {
                OutlinedTextField(name, { name = it }, label = { Text(words("名称", "Name")) })
                OutlinedTextField(to, { to = it }, label = { Text(words("接收者", "To")) })
                OutlinedTextField(
                    text,
                    { text = it },
                    label = { Text(words("消息", "Message")) },
                    minLines = 2,
                )
                OutlinedTextField(
                    cron,
                    { cron = it },
                    label = { Text(words("定时规则", "Schedule (cron)")) },
                    supportingText = { Text(java.util.TimeZone.getDefault().id) },
                )
            }
        },
        confirmButton = {
            TextButton(
                enabled = name.isNotBlank() && to.isNotBlank() && text.isNotBlank(),
                onClick = {
                    save(
                        buildJsonObject {
                            put("name", name)
                            put("to", to)
                            put("body", text)
                            put(
                                "schedule",
                                buildJsonObject {
                                    put("cron", cron)
                                    put("tz", java.util.TimeZone.getDefault().id)
                                },
                            )
                            put("enabled", true)
                        }
                    )
                },
            ) {
                Text(words("保存", "Save"))
            }
        },
        dismissButton = { TextButton(onClick = close) { Text(words("取消", "Cancel")) } },
    )
}
