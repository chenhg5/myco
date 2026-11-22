package dev.myco.android

import android.content.Intent
import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.compose.BackHandler
import androidx.activity.compose.setContent
import androidx.activity.enableEdgeToEdge
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.input.PasswordVisualTransformation
import androidx.compose.ui.unit.dp
import androidx.core.content.FileProvider
import dev.myco.model.*
import dev.myco.network.HubClient
import dev.myco.storage.DeviceStore
import dev.myco.ui.*
import dev.myco.workspace.WorkspaceScreen
import java.io.File
import kotlinx.coroutines.*

class MainActivity : ComponentActivity() {
    private val incoming = mutableStateOf<String?>(null)

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        enableEdgeToEdge()
        incoming.value = intent.dataString
        val store = DeviceStore(applicationContext)
        setContent {
            MycoTheme {
                MycoApp(store, incoming.value, { incoming.value = null }, ::shareText, ::openFile)
            }
        }
    }

    override fun onNewIntent(intent: Intent) {
        super.onNewIntent(intent)
        incoming.value = intent.dataString
    }

    private fun shareText(text: String) {
        startActivity(
            Intent.createChooser(
                Intent(Intent.ACTION_SEND).apply {
                    type = "text/markdown"
                    putExtra(Intent.EXTRA_TEXT, text)
                },
                "myco",
            )
        )
    }

    private suspend fun openFile(name: String, type: String, data: ByteArray) {
        val uri =
            withContext(Dispatchers.IO) {
                val folder =
                    File(cacheDir, "shared/${java.util.UUID.randomUUID()}").apply { mkdirs() }
                val safe =
                    name
                        .substringAfterLast('/')
                        .substringAfterLast('\\')
                        .replace(Regex("""[^\p{L}\p{N}._ -]"""), "_")
                        .take(120)
                        .trimStart('.')
                        .ifBlank { "File" }
                val file = File(folder, safe).apply { writeBytes(data) }
                FileProvider.getUriForFile(this@MainActivity, "$packageName.files", file)
            }
        val intent =
            Intent(Intent.ACTION_VIEW)
                .setDataAndType(uri, type)
                .addFlags(Intent.FLAG_GRANT_READ_URI_PERMISSION)
        try {
            startActivity(Intent.createChooser(intent, "myco"))
        } catch (_: android.content.ActivityNotFoundException) {
            startActivity(
                Intent.createChooser(
                    Intent(Intent.ACTION_SEND).apply {
                        this.type = type
                        putExtra(Intent.EXTRA_STREAM, uri)
                        addFlags(Intent.FLAG_GRANT_READ_URI_PERMISSION)
                    },
                    "myco",
                )
            )
        }
    }
}

@Composable
private fun MycoApp(
    store: DeviceStore,
    incoming: String?,
    consumed: () -> Unit,
    share: (String) -> Unit,
    file: suspend (String, String, ByteArray) -> Unit,
) {
    val scope = rememberCoroutineScope()
    var all by remember { mutableStateOf<List<Workspace>>(emptyList()) }
    var current by remember { mutableStateOf<Workspace?>(null) }
    var hub by remember { mutableStateOf<Hub?>(null) }
    var started by remember { mutableStateOf(false) }
    var join by rememberSaveable { mutableStateOf(false) }
    var settings by rememberSaveable { mutableStateOf(false) }
    var problem by remember { mutableStateOf<String?>(null) }
    val demo = remember { DemoHub() }
    val demoStore = remember { DemoStore() }
    suspend fun select(workspace: Workspace) {
        val credentials = store.credentials(workspace.id) ?: return
        hub = HubClient(credentials.workspace.url, credentials.token)
        current = credentials.workspace
        store.select(workspace.id)
    }
    LaunchedEffect(Unit) {
        try {
            all = store.workspaces()
            val id = store.selected()
            all.firstOrNull { it.id == id }?.let { select(it) }
                ?: all.firstOrNull()?.let { select(it) }
        } catch (_: Exception) {
            problem = "Could not open saved workspaces"
        }
        started = true
    }
    LaunchedEffect(incoming) { if (incoming != null) join = true }
    BackHandler(join || settings) {
        join = false
        settings = false
        consumed()
    }
    Surface(Modifier.fillMaxSize(), color = MaterialTheme.colorScheme.background) {
        when {
            !started ->
                Box(Modifier.fillMaxSize(), contentAlignment = Alignment.Center) {
                    CircularProgressIndicator()
                }
            join ->
                JoinScreen(
                    incoming.orEmpty(),
                    back = {
                        join = false
                        consumed()
                    },
                    joined = { credentials ->
                        scope.launch {
                            try {
                                val old = all.firstOrNull {
                                    it.url.trimEnd('/') == credentials.workspace.url.trimEnd('/')
                                }
                                val saved =
                                    if (old == null) credentials
                                    else credentials.copy(workspace = old)
                                store.save(saved)
                                all = store.workspaces()
                                select(saved.workspace)
                                join = false
                                consumed()
                            } catch (_: Exception) {
                                problem = "Could not save the workspace on this device"
                            }
                        }
                    },
                )
            settings ->
                SettingsScreen(
                    current,
                    back = { settings = false },
                    join = {
                        settings = false
                        join = true
                    },
                    leave = {
                        scope.launch {
                            current?.let { if (it.id != "demo") store.remove(it.id) }
                            all = store.workspaces()
                            current = null
                            hub = null
                            settings = false
                            all.firstOrNull()?.let { select(it) }
                        }
                    },
                )
            current != null && hub != null ->
                key(current!!.id) {
                    WorkspaceScreen(
                        current!!,
                        hub!!,
                        if (current!!.id == "demo") demoStore else store,
                        all,
                        switch = { workspace -> scope.launch { select(workspace) } },
                        addWorkspace = { join = true },
                        settings = { settings = true },
                        openFile = { client, attachment ->
                            scope.launch {
                                try {
                                    val data =
                                        withContext(Dispatchers.IO) {
                                            client.file(attachment.fileId)
                                        }
                                    file(attachment.name, attachment.contentType, data)
                                } catch (_: Exception) {
                                    problem = "Could not open the file. Try again."
                                }
                            }
                        },
                        export = share,
                    )
                }
            else ->
                Welcome(
                    join = { join = true },
                    demo = {
                        current = Workspace("demo", "Demo", "https://demo.invalid")
                        hub = demo
                    },
                )
        }
        problem?.let { text ->
            AlertDialog(
                onDismissRequest = { problem = null },
                title = { Text(words("暂时无法完成", "Could not finish")) },
                text = { Text(text) },
                confirmButton = {
                    TextButton(onClick = { problem = null }) { Text(words("好", "OK")) }
                },
            )
        }
    }
}

@Composable
private fun Welcome(join: () -> Unit, demo: () -> Unit) {
    Column(
        Modifier.fillMaxSize().safeDrawingPadding().padding(32.dp),
        verticalArrangement = Arrangement.Center,
    ) {
        Mushroom("myco", Modifier.size(72.dp))
        Spacer(Modifier.height(32.dp))
        Text(
            words("你的 AI 伙伴，\n随时在身边", "Your AI colleagues,\nwherever you are"),
            style = MaterialTheme.typography.headlineLarge,
        )
        Spacer(Modifier.height(16.dp))
        Text(
            words(
                "和电脑上的智能体交流，跟进项目，把事情做成。",
                "Talk with your agents, follow your projects, and keep things moving.",
            ),
            style = MaterialTheme.typography.bodyLarge,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
        Spacer(Modifier.height(40.dp))
        Button(onClick = join, modifier = Modifier.fillMaxWidth().height(52.dp)) {
            Text(words("加入工作区", "Join a workspace"))
        }
        TextButton(onClick = demo, modifier = Modifier.fillMaxWidth()) {
            Text(words("先看看", "Look around first"))
        }
    }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun JoinScreen(initial: String, back: () -> Unit, joined: (Credentials) -> Unit) {
    var link by rememberSaveable { mutableStateOf(initial) }
    var busy by remember { mutableStateOf(false) }
    var error by remember { mutableStateOf<String?>(null) }
    val scope = rememberCoroutineScope()
    Scaffold(
        topBar = {
            TopAppBar(
                title = { Text(words("加入工作区", "Join workspace")) },
                navigationIcon = { TextButton(onClick = back) { Text(words("返回", "Back")) } },
            )
        }
    ) { padding ->
        Column(
            Modifier.padding(padding).padding(24.dp),
            verticalArrangement = Arrangement.spacedBy(20.dp),
        ) {
            Text(
                words(
                    "在电脑的设置中打开「我的设备」，添加设备，然后把链接粘贴到这里。",
                    "On your computer, open Settings → My devices → Add another device, then paste the link here.",
                )
            )
            OutlinedTextField(
                link,
                { link = it },
                label = { Text(words("添加设备链接", "Add-device link")) },
                modifier = Modifier.fillMaxWidth(),
                visualTransformation = PasswordVisualTransformation(),
                maxLines = 4,
            )
            error?.let { Problem(it) }
            Button(
                enabled = link.isNotBlank() && !busy,
                onClick = {
                    scope.launch {
                        busy = true
                        try {
                            joined(HubClient.join(link, android.os.Build.MODEL))
                        } catch (e: CancellationException) {
                            throw e
                        } catch (e: Exception) {
                            error = e.message ?: "Could not join"
                        } finally {
                            busy = false
                        }
                    }
                },
                modifier = Modifier.fillMaxWidth().height(50.dp),
            ) {
                if (busy) CircularProgressIndicator(Modifier.size(20.dp), strokeWidth = 2.dp)
                else Text(words("加入", "Join"))
            }
        }
    }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun SettingsScreen(
    workspace: Workspace?,
    back: () -> Unit,
    join: () -> Unit,
    leave: () -> Unit,
) {
    var confirm by remember { mutableStateOf(false) }
    Scaffold(
        topBar = {
            TopAppBar(
                title = { Text(words("设置", "Settings")) },
                navigationIcon = { TextButton(onClick = back) { Text(words("返回", "Back")) } },
            )
        }
    ) { padding ->
        Column(
            Modifier.padding(padding).padding(24.dp).verticalScroll(rememberScrollState()),
            verticalArrangement = Arrangement.spacedBy(16.dp),
        ) {
            Text(workspace?.name.orEmpty(), style = MaterialTheme.typography.titleLarge)
            Text(workspace?.url.orEmpty(), style = MaterialTheme.typography.bodySmall)
            OutlinedButton(onClick = join) { Text(words("加入另一个工作区", "Join another workspace")) }
            OutlinedButton(onClick = { confirm = true }) {
                Text(words("离开此工作区", "Leave this workspace"))
            }
            Spacer(Modifier.height(20.dp))
            Text("myco for Android · 0.1.0", style = MaterialTheme.typography.bodySmall)
        }
    }
    if (confirm)
        AlertDialog(
            onDismissRequest = { confirm = false },
            title = { Text(words("离开工作区？", "Leave workspace?")) },
            text = {
                Text(
                    words(
                        "此设备上保存的登录和缓存将被移除。",
                        "This device’s sign-in and saved conversations will be removed.",
                    )
                )
            },
            confirmButton = {
                TextButton(
                    onClick = {
                        confirm = false
                        leave()
                    }
                ) {
                    Text(words("离开", "Leave"))
                }
            },
            dismissButton = {
                TextButton(onClick = { confirm = false }) { Text(words("取消", "Cancel")) }
            },
        )
}
