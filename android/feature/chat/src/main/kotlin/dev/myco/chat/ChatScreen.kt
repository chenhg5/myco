package dev.myco.chat

import android.content.ClipData
import android.content.ClipboardManager
import android.content.Context
import android.provider.OpenableColumns
import android.widget.TextView
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.animation.animateContentSize
import androidx.compose.foundation.*
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.lazy.rememberLazyListState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.selection.SelectionContainer
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.outlined.ArrowBack
import androidx.compose.material.icons.automirrored.outlined.Send
import androidx.compose.material.icons.outlined.*
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.toArgb
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.unit.dp
import androidx.compose.ui.viewinterop.AndroidView
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.compose.LifecycleEventEffect
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import dev.myco.conversation.*
import dev.myco.model.*
import dev.myco.ui.*
import io.noties.markwon.Markwon
import kotlinx.coroutines.*

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun ChatScreen(
    controller: ConversationController,
    title: String,
    onBack: () -> Unit,
    openFile: (Attachment) -> Unit,
    export: (String) -> Unit,
    settings: () -> Unit = {},
) {
    val state by controller.state.collectAsStateWithLifecycle()
    val scope = rememberCoroutineScope()
    val context = LocalContext.current
    val snackbar = remember { SnackbarHostState() }
    val list = rememberLazyListState()
    var fileProblem by remember { mutableStateOf<String?>(null) }
    var menu by remember { mutableStateOf(false) }
    val picker =
        rememberLauncherForActivityResult(ActivityResultContracts.OpenMultipleDocuments()) { uris ->
            uris.forEach { uri ->
                scope.launch {
                    try {
                        val picked =
                            withContext(Dispatchers.IO) {
                                var name = "File"
                                context.contentResolver
                                    .query(
                                        uri,
                                        arrayOf(OpenableColumns.DISPLAY_NAME, OpenableColumns.SIZE),
                                        null,
                                        null,
                                        null,
                                    )
                                    ?.use { cursor ->
                                        if (cursor.moveToFirst()) {
                                            name = cursor.getString(0) ?: name
                                            if (!cursor.isNull(1))
                                                require(cursor.getLong(1) <= 50L * 1024 * 1024) {
                                                    "Files must be smaller than 50 MB"
                                                }
                                        }
                                    }
                                val bytes =
                                    context.contentResolver.openInputStream(uri)!!.use { input ->
                                        val output = java.io.ByteArrayOutputStream()
                                        val buffer = ByteArray(8192)
                                        while (true) {
                                            val n = input.read(buffer)
                                            if (n < 0) break
                                            require(output.size() + n <= 50 * 1024 * 1024) {
                                                "Files must be smaller than 50 MB"
                                            }
                                            output.write(buffer, 0, n)
                                        }
                                        output.toByteArray()
                                    }
                                Triple(
                                    name,
                                    context.contentResolver.getType(uri)
                                        ?: "application/octet-stream",
                                    bytes,
                                )
                            }
                        controller.attach(picked.first, picked.second, picked.third)
                    } catch (e: CancellationException) {
                        throw e
                    } catch (e: Exception) {
                        fileProblem = e.message
                    }
                }
            }
        }
    LifecycleEventEffect(Lifecycle.Event.ON_RESUME) { controller.start() }
    LifecycleEventEffect(Lifecycle.Event.ON_PAUSE) { controller.stopPolling() }
    DisposableEffect(controller) {
        controller.start()
        onDispose { controller.stopPolling() }
    }
    val visible =
        remember(state.messages, state.hidden) { ConversationRules.visible(state, controller.me) }
    val open = ConversationRules.openTask(controller.me, state.messages, state.traces)
    val waiting = ConversationRules.waiting(state.messages)
    val action = ConversationRules.action(controller.me, state)
    val canSend =
        !state.sending &&
            state.files.none { it.uploading || it.error != null } &&
            (action == ComposerAction.Stop || state.draft.isNotBlank() || state.files.isNotEmpty())
    Scaffold(
        topBar = {
            TopAppBar(
                title = {
                    Row(verticalAlignment = Alignment.CenterVertically) {
                        Mushroom(controller.agent, Modifier.size(30.dp))
                        Spacer(Modifier.width(10.dp))
                        Text(title, maxLines = 1)
                    }
                },
                navigationIcon = {
                    IconButton(onClick = onBack) {
                        Icon(Icons.AutoMirrored.Outlined.ArrowBack, words("返回", "Back"))
                    }
                },
                actions = {
                    Box {
                        IconButton(onClick = { menu = true }) {
                            Icon(Icons.Outlined.MoreHoriz, words("更多", "More"))
                        }
                        DropdownMenu(expanded = menu, onDismissRequest = { menu = false }) {
                            DropdownMenuItem(
                                text = { Text(words("刷新", "Refresh")) },
                                onClick = {
                                    menu = false
                                    scope.launch { controller.refresh(true) }
                                },
                            )
                            DropdownMenuItem(
                                text = { Text(words("导出对话", "Export conversation")) },
                                onClick = {
                                    menu = false
                                    export(
                                        "# $title\n\n" +
                                            visible.joinToString("\n\n") {
                                                "## ${if(it.from == controller.me) "You" else title}\n\n${it.readableBody}"
                                            }
                                    )
                                },
                            )
                            DropdownMenuItem(
                                text = { Text(words("成员详情", "Member details")) },
                                onClick = {
                                    menu = false
                                    settings()
                                },
                            )
                        }
                    }
                },
            )
        },
        snackbarHost = { SnackbarHost(snackbar) },
    ) { padding ->
        Column(Modifier.fillMaxSize().padding(padding).consumeWindowInsets(padding).imePadding()) {
            if (state.stale)
                Problem(
                    words("工作区暂时无法连接，正在显示之前的内容", "Workspace unavailable. Showing saved messages.")
                ) {
                    scope.launch { controller.refresh(true) }
                }
            if (state.failure != null || fileProblem != null)
                Problem(state.failure ?: fileProblem.orEmpty()) {
                    controller.clearFailure()
                    fileProblem = null
                    scope.launch { controller.refresh(true) }
                }
            Box(Modifier.weight(1f)) {
                if (!state.loaded)
                    Column(Modifier.padding(24.dp)) {
                        repeat(5) {
                            LinearProgressIndicator(
                                Modifier.fillMaxWidth(if (it % 2 == 0) .72f else .9f)
                                    .padding(vertical = 20.dp)
                            )
                        }
                    }
                else if (visible.isEmpty() && state.outgoing.isEmpty())
                    Empty(
                        words("和 $title 聊聊", "Talk with $title"),
                        words("把你想做的事情告诉它", "Tell it what you have in mind"),
                    )
                LazyColumn(
                    state = list,
                    reverseLayout = true,
                    modifier = Modifier.fillMaxSize(),
                    contentPadding = PaddingValues(horizontal = 18.dp, vertical = 20.dp),
                    verticalArrangement = Arrangement.spacedBy(16.dp),
                ) {
                    items(state.outgoing.reversed(), key = { "outgoing:${it.id}" }) { outgoing ->
                        Column(
                            horizontalAlignment = Alignment.End,
                            modifier = Modifier.fillMaxWidth(),
                        ) {
                            Surface(
                                color = MaterialTheme.colorScheme.secondaryContainer,
                                shape = RoundedCornerShape(18.dp, 18.dp, 4.dp, 18.dp),
                            ) {
                                Text(outgoing.body, Modifier.padding(14.dp))
                            }
                            TextButton(
                                enabled = outgoing.error != null,
                                onClick = { controller.recoverOutgoing(outgoing.id) },
                            ) {
                                Text(
                                    outgoing.error ?: words("发送中…", "Sending…"),
                                    style = MaterialTheme.typography.labelSmall,
                                )
                            }
                        }
                    }
                    if (
                        open?.status == "acked" &&
                            !ConversationRules.paused(state.traces[open.id].orEmpty())
                    )
                        item("working") {
                            Row(
                                verticalAlignment = Alignment.CenterVertically,
                                modifier = Modifier.padding(vertical = 4.dp),
                            ) {
                                Mushroom(controller.agent, Modifier.size(28.dp), working = true)
                                Spacer(Modifier.width(10.dp))
                                Text(
                                    words("正在处理", "Working"),
                                    style = MaterialTheme.typography.bodySmall,
                                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                                )
                            }
                        }
                    items(visible.asReversed(), key = { it.id }, contentType = { it.type }) {
                        message ->
                        MessageRow(
                            message,
                            message.from == controller.me,
                            state.traces[message.id].orEmpty(),
                            openFile,
                            copy = { copy(context, message.readableBody) },
                            hide = {
                                scope.launch {
                                    try {
                                        controller.hide(message.id, true)
                                        if (
                                            snackbar.showSnackbar(
                                                if (message.from == controller.me) "Message hidden"
                                                else "Reply hidden",
                                                "Undo",
                                                duration = SnackbarDuration.Short,
                                            ) == SnackbarResult.ActionPerformed
                                        )
                                            controller.hide(message.id, false)
                                    } catch (e: CancellationException) {
                                        throw e
                                    } catch (e: Exception) {
                                        fileProblem = e.message
                                    }
                                }
                            },
                            resend = {
                                scope.launch {
                                    try {
                                        controller.resend(message)
                                    } catch (e: Exception) {
                                        fileProblem = e.message
                                    }
                                }
                            },
                        )
                    }
                    if (state.more)
                        item("earlier") {
                            TextButton(
                                enabled = !state.loadingEarlier,
                                onClick = { scope.launch { controller.earlier() } },
                                modifier = Modifier.fillMaxWidth(),
                            ) {
                                Text(
                                    if (state.loadingEarlier) words("正在读取…", "Loading…")
                                    else words("更早的消息", "Earlier messages")
                                )
                            }
                        }
                }
                val nearEnd by remember { derivedStateOf { list.firstVisibleItemIndex <= 1 } }
                if (!nearEnd)
                    FilledTonalIconButton(
                        onClick = { scope.launch { list.animateScrollToItem(0) } },
                        modifier = Modifier.align(Alignment.BottomEnd).padding(12.dp),
                    ) {
                        Icon(Icons.Outlined.ArrowDownward, words("最新消息", "Latest messages"))
                    }
            }
            if (waiting.isNotEmpty())
                Column(
                    Modifier.fillMaxWidth()
                        .background(MaterialTheme.colorScheme.surfaceContainer)
                        .padding(horizontal = 16.dp)
                ) {
                    waiting.forEach { item ->
                        Row(verticalAlignment = Alignment.CenterVertically) {
                            Text(
                                item.readableBody,
                                Modifier.weight(1f),
                                maxLines = 1,
                                style = MaterialTheme.typography.bodySmall,
                            )
                            TextButton(
                                onClick = {
                                    scope.launch {
                                        try {
                                            controller.withdraw(item, true)
                                        } catch (e: Exception) {
                                            fileProblem = e.message
                                        }
                                    }
                                }
                            ) {
                                Text(words("编辑", "Edit"))
                            }
                            IconButton(
                                onClick = {
                                    scope.launch {
                                        try {
                                            controller.withdraw(item, false)
                                        } catch (e: Exception) {
                                            fileProblem = e.message
                                        }
                                    }
                                }
                            ) {
                                Icon(Icons.Outlined.Close, words("撤回", "Withdraw"))
                            }
                        }
                    }
                }
            val suggestion =
                if (open == null)
                    state.messages
                        .lastOrNull { it.from == controller.me && it.type == "task" }
                        ?.suggest
                else null
            if (state.draft.isBlank() && !suggestion.isNullOrBlank())
                TextButton(
                    onClick = { controller.setDraft(suggestion) },
                    modifier = Modifier.padding(horizontal = 12.dp),
                ) {
                    Text(suggestion, maxLines = 2, style = MaterialTheme.typography.bodySmall)
                }
            Surface(tonalElevation = 1.dp) {
                Column(Modifier.padding(horizontal = 12.dp, vertical = 8.dp)) {
                    if (state.files.isNotEmpty())
                        Row(Modifier.horizontalScroll(rememberScrollState())) {
                            state.files.forEach { file ->
                                InputChip(
                                    selected = false,
                                    onClick = {},
                                    label = {
                                        Text(
                                            if (file.uploading) "${file.name} …"
                                            else file.error ?: file.name,
                                            maxLines = 1,
                                        )
                                    },
                                    trailingIcon = {
                                        IconButton(
                                            onClick = { controller.removeFile(file.localId) },
                                            modifier = Modifier.size(24.dp),
                                        ) {
                                            Icon(
                                                Icons.Outlined.Close,
                                                words("移除附件", "Remove attachment"),
                                                Modifier.size(16.dp),
                                            )
                                        }
                                    },
                                    modifier = Modifier.padding(end = 6.dp),
                                )
                            }
                        }
                    OutlinedTextField(
                        value = state.draft,
                        onValueChange = controller::setDraft,
                        placeholder = {
                            Text(
                                if (action == ComposerAction.Steer) words("补充一下…", "Add to this…")
                                else words("发消息给 $title", "Message $title")
                            )
                        },
                        modifier = Modifier.fillMaxWidth(),
                        minLines = 1,
                        maxLines = 6,
                        shape = RoundedCornerShape(18.dp),
                    )
                    Row(Modifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically) {
                        IconButton(onClick = { picker.launch(arrayOf("*/*")) }) {
                            Icon(Icons.Outlined.Add, words("添加照片或文件", "Add photos or files"))
                        }
                        Spacer(Modifier.weight(1f))
                        if (action == ComposerAction.Steer && state.draft.isNotBlank())
                            TextButton(
                                onClick = { scope.launch { controller.submit(queue = true) } },
                                enabled = canSend,
                            ) {
                                Text(words("排队", "Queue"))
                            }
                        FilledIconButton(
                            onClick = {
                                scope.launch {
                                    controller.submit()
                                    list.animateScrollToItem(0)
                                }
                            },
                            enabled = canSend,
                        ) {
                            if (state.sending)
                                CircularProgressIndicator(Modifier.size(20.dp), strokeWidth = 2.dp)
                            else
                                Icon(
                                    if (action == ComposerAction.Stop) Icons.Outlined.Stop
                                    else Icons.AutoMirrored.Outlined.Send,
                                    if (action == ComposerAction.Stop) words("停止", "Stop")
                                    else words("发送", "Send"),
                                )
                        }
                    }
                }
            }
        }
    }
}

private fun copy(context: Context, text: String) {
    (context.getSystemService(Context.CLIPBOARD_SERVICE) as ClipboardManager).setPrimaryClip(
        ClipData.newPlainText("myco", text)
    )
}

@Composable
private fun MessageRow(
    message: Message,
    mine: Boolean,
    trace: List<TraceEvent>,
    openFile: (Attachment) -> Unit,
    copy: () -> Unit,
    hide: () -> Unit,
    resend: () -> Unit,
) {
    var menu by remember { mutableStateOf(false) }
    var details by remember { mutableStateOf(false) }
    Column(
        Modifier.fillMaxWidth(),
        horizontalAlignment = if (mine) Alignment.End else Alignment.Start,
    ) {
        Box(Modifier.combinedClickable(onClick = {}, onLongClick = { menu = true })) {
            if (mine)
                Surface(
                    color = MaterialTheme.colorScheme.secondaryContainer,
                    shape = RoundedCornerShape(18.dp, 18.dp, 4.dp, 18.dp),
                    modifier = Modifier.widthIn(max = 340.dp),
                ) {
                    SelectionContainer { Text(message.readableBody, Modifier.padding(14.dp)) }
                }
            else {
                if (message.type == "progress")
                    Text(
                        message.readableBody,
                        style = MaterialTheme.typography.bodyMedium,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                        modifier = Modifier.padding(vertical = 4.dp),
                    )
                else Markdown(message.readableBody)
            }
            DropdownMenu(menu, onDismissRequest = { menu = false }) {
                DropdownMenuItem(
                    text = { Text(words("复制", "Copy")) },
                    onClick = {
                        menu = false
                        copy()
                    },
                )
                DropdownMenuItem(
                    text = { Text(words("隐藏", "Hide")) },
                    onClick = {
                        menu = false
                        hide()
                    },
                )
            }
        }
        message.attachments.forEach { file ->
            AssistChip(
                onClick = { openFile(file) },
                label = { Text(file.name, maxLines = 1) },
                leadingIcon = { Icon(Icons.Outlined.AttachFile, null, Modifier.size(16.dp)) },
            )
        }
        if (mine && message.isOpen && message.status != "acked")
            Text(
                if (message.status == "held") words("稍后回复", "Will answer later")
                else words("尚未开始", "Not opened yet"),
                style = MaterialTheme.typography.labelSmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
                modifier = Modifier.padding(top = 4.dp),
            )
        if (message.status == "failed" && mine) {
            Text(
                message.error ?: words("未能完成", "Could not finish"),
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.error,
            )
            if (message.error != "stopped by the sender")
                TextButton(onClick = resend) { Text(words("重新发送", "Send again")) }
        }
        if (trace.isNotEmpty()) {
            TextButton(onClick = { details = !details }, contentPadding = PaddingValues(0.dp)) {
                Icon(
                    if (details) Icons.Outlined.ExpandLess else Icons.Outlined.ExpandMore,
                    null,
                    Modifier.size(16.dp),
                )
                Text(
                    words("${trace.size} 条步骤", "${trace.size} steps"),
                    style = MaterialTheme.typography.labelSmall,
                )
            }
            if (details)
                Column(
                    Modifier.animateContentSize()
                        .fillMaxWidth()
                        .background(
                            MaterialTheme.colorScheme.surfaceContainer,
                            RoundedCornerShape(12.dp),
                        )
                        .padding(12.dp)
                ) {
                    trace.takeLast(100).forEach { event ->
                        val step = event.step
                        Text(
                            step?.text ?: step?.tool ?: event.kind,
                            style = MaterialTheme.typography.bodySmall,
                        )
                        if (!step?.input.isNullOrBlank())
                            Text(
                                step!!.input!!.take(1200),
                                style = MaterialTheme.typography.labelSmall,
                                color = MaterialTheme.colorScheme.onSurfaceVariant,
                            )
                        if (!step?.output.isNullOrBlank())
                            Text(
                                step!!.output!!.take(1600),
                                style = MaterialTheme.typography.labelSmall,
                                color = MaterialTheme.colorScheme.onSurfaceVariant,
                            )
                        Spacer(Modifier.height(8.dp))
                    }
                }
        }
    }
}

@Composable
fun Markdown(text: String, modifier: Modifier = Modifier) {
    val context = LocalContext.current
    val renderer = remember(context) { Markwon.create(context) }
    val color = MaterialTheme.colorScheme.onSurface.toArgb()
    val density = androidx.compose.ui.platform.LocalDensity.current
    AndroidView(
        factory = {
            TextView(it).apply {
                textSize = 16f
                setTextIsSelectable(true)
                setLineSpacing(4f, 1f)
                setPadding(0, 0, 0, 0)
            }
        },
        update = { view ->
            view.setTextColor(color)
            if (view.tag != text) {
                renderer.setMarkdown(view, text)
                view.tag = text
            }
        },
        modifier = modifier.fillMaxWidth(),
    )
}
