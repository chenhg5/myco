package dev.myco.workspace

import androidx.activity.compose.BackHandler
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.outlined.ArrowBack
import androidx.compose.material.icons.outlined.*
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalFocusManager
import androidx.compose.ui.platform.LocalSoftwareKeyboardController
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.compose.LocalLifecycleOwner
import androidx.lifecycle.repeatOnLifecycle
import dev.myco.chat.ChatScreen
import dev.myco.chat.Markdown
import dev.myco.conversation.ConversationController
import dev.myco.model.*
import dev.myco.network.HubClient
import dev.myco.network.PageLinks
import dev.myco.pages.PageNavigation
import dev.myco.pages.PageScreen
import dev.myco.ui.*
import kotlinx.coroutines.*
import kotlinx.serialization.json.*

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun WorkspaceScreen(
    workspace: Workspace,
    hub: Hub,
    conversations: ConversationStore,
    all: List<Workspace>,
    switch: (Workspace) -> Unit,
    addWorkspace: () -> Unit,
    settings: () -> Unit,
    openFile: (Hub, Attachment) -> Unit,
    export: (String) -> Unit,
) {
    var tab by rememberSaveable(workspace.id) { mutableIntStateOf(0) }
    var route by rememberSaveable(workspace.id) { mutableStateOf("") }
    var selectedProject by rememberSaveable(workspace.id) { mutableStateOf<String?>(null) }
    var who by remember(workspace.id) { mutableStateOf<Whoami?>(null) }
    var overview by remember(workspace.id) { mutableStateOf(Overview()) }
    var recent by remember(workspace.id) { mutableStateOf<List<Recent>>(emptyList()) }
    var projects by remember(workspace.id) { mutableStateOf<List<Project>>(emptyList()) }
    var error by remember(workspace.id) { mutableStateOf<String?>(null) }
    var loaded by remember(workspace.id) { mutableStateOf(false) }
    var workspaceMenu by remember { mutableStateOf(false) }
    var query by rememberSaveable { mutableStateOf("") }
    var refresh by remember { mutableIntStateOf(0) }
    var failures by remember { mutableIntStateOf(0) }
    val lifecycle = LocalLifecycleOwner.current.lifecycle
    val scope = rememberCoroutineScope()
    val focus = LocalFocusManager.current
    val keyboard = LocalSoftwareKeyboardController.current
    var openedPage by remember(workspace.id) { mutableStateOf<JsonElement?>(null) }
    var pageNavigation by remember(workspace.id) { mutableStateOf(PageNavigation()) }
    var pageLoad by remember(workspace.id) { mutableStateOf<Job?>(null) }
    var pageFailure by remember(workspace.id) { mutableStateOf<String?>(null) }
    var pageRequest by remember(workspace.id) { mutableLongStateOf(0) }
    fun closePage() {
        pageRequest++
        pageLoad?.cancel()
        openedPage = null
    }
    LaunchedEffect(route) { closePage() }
    val openPageLink: (String) -> Boolean = { link ->
        val target = PageLinks.parse(link, workspace.url)
        if (target == null || hub !is HubClient) false
        else {
            focus.clearFocus()
            keyboard?.hide()
            val request = ++pageRequest
            pageNavigation = PageNavigation(target.fragment, request)
            pageLoad?.cancel()
            if (openedPage?.jsonObject?.get("id")?.jsonPrimitive?.content != target.pageId) {
                openedPage = null
                pageLoad = scope.launch {
                    try {
                        val found = hub.request("GET", listOf("v1", "pages", target.pageId))
                        if (pageRequest == request) openedPage = found
                    } catch (e: CancellationException) {
                        throw e
                    } catch (e: Exception) {
                        if (pageRequest == request)
                            pageFailure = e.message ?: "Could not open the page"
                    }
                }
            }
            true
        }
    }
    LaunchedEffect(workspace.id, refresh) {
        lifecycle.repeatOnLifecycle(Lifecycle.State.STARTED) {
            while (isActive) {
                try {
                    supervisorScope {
                        val identity = async { who ?: hub.whoami() }
                        val nodes = async { hub.overview() }
                        val chats = async { hub.recent() }
                        val work = async { hub.projects() }
                        who = identity.await()
                        overview = nodes.await()
                        recent = chats.await()
                        projects = work.await()
                    }
                    error = null
                    failures = 0
                } catch (e: CancellationException) {
                    throw e
                } catch (e: Exception) {
                    failures++
                    if (failures > 1 || !loaded) error = e.message ?: "Workspace unavailable"
                }
                loaded = true
                delay(5_000)
            }
        }
    }
    BackHandler(route.isNotEmpty()) {
        route = ""
        selectedProject = null
    }
    val me = who?.sender
    if (route.startsWith("chat:") && me != null) {
        val name = route.removePrefix("chat:")
        val title = overview.agents.firstOrNull { it.agent.name == name }?.agent?.title ?: name
        Box(Modifier.fillMaxSize()) {
            ConversationRoute(
                hub,
                me,
                name,
                workspace.id,
                selectedProject,
                conversations,
                title,
                {
                    route = ""
                    selectedProject = null
                },
                { openFile(hub, it) },
                export,
                { route = "member:$name" },
                openPageLink,
            )
            val page = openedPage
            if (page != null && hub is HubClient) {
                BackHandler { closePage() }
                PageScreen(page, hub, { closePage() }, pageNavigation)
            }
        }
        pageFailure?.let { failure ->
            AlertDialog(
                onDismissRequest = { pageFailure = null },
                title = { Text(words("网页暂时无法打开", "Could not open the page")) },
                text = { Text(failure) },
                confirmButton = {
                    TextButton(onClick = { pageFailure = null }) { Text(words("关闭", "Close")) }
                },
            )
        }
        return
    }
    if (route.startsWith("member:")) {
        val agent = overview.agents.firstOrNull { it.agent.name == route.removePrefix("member:") }
        if (agent != null) {
            MemberScreen(
                agent,
                who,
                hub as? HubClient,
                { route = "" },
                { route = "chat:${agent.agent.name}" },
            )
            return
        }
    }
    if (route.startsWith("project:")) {
        val project = projects.firstOrNull { it.name == route.removePrefix("project:") }
        if (project != null) {
            ProjectScreen(
                project,
                overview.agents,
                { route = "" },
                { name ->
                    selectedProject = project.name
                    route = "chat:$name"
                },
            )
            return
        }
    }
    if (route in setOf("apps", "automations", "workflows") && hub is HubClient) {
        CollectionScreen(route, hub, { route = "" })
        return
    }
    val workEntries =
        listOf(
            Triple("apps", words("应用", "Apps"), Icons.Outlined.GridView),
            Triple("automations", words("自动化", "Automations"), Icons.Outlined.Schedule),
            Triple("workflows", words("工作流", "Workflows"), Icons.Outlined.AccountTree),
        )
    Scaffold(
        topBar = {
            TopAppBar(
                title = {
                    Box {
                        TextButton(onClick = { workspaceMenu = true }) {
                            Text(workspace.name)
                            Icon(Icons.Outlined.ExpandMore, null, Modifier.size(18.dp))
                        }
                        DropdownMenu(workspaceMenu, { workspaceMenu = false }) {
                            all.forEach { item ->
                                DropdownMenuItem(
                                    text = { Text(item.name) },
                                    onClick = {
                                        workspaceMenu = false
                                        route = ""
                                        switch(item)
                                    },
                                )
                            }
                            HorizontalDivider()
                            DropdownMenuItem(
                                text = { Text(words("加入工作区", "Join workspace")) },
                                onClick = {
                                    workspaceMenu = false
                                    addWorkspace()
                                },
                            )
                        }
                    }
                },
                actions = {
                    IconButton(onClick = { refresh++ }) {
                        Icon(Icons.Outlined.Refresh, words("刷新", "Refresh"))
                    }
                    IconButton(onClick = settings) {
                        Icon(Icons.Outlined.Settings, words("设置", "Settings"))
                    }
                },
            )
        },
        bottomBar = {
            NavigationBar(containerColor = MaterialTheme.colorScheme.surface) {
                listOf(
                        Triple(words("对话", "Chats"), Icons.Outlined.ChatBubbleOutline, 0),
                        Triple(words("项目", "Projects"), Icons.Outlined.FolderOpen, 1),
                        Triple(words("工作台", "Workbench"), Icons.Outlined.GridView, 2),
                    )
                    .forEach { (label, icon, index) ->
                        NavigationBarItem(
                            selected = tab == index,
                            onClick = {
                                tab = index
                                query = ""
                            },
                            icon = { Icon(icon, null) },
                            label = { Text(label) },
                        )
                    }
            }
        },
    ) { padding ->
        Column(Modifier.fillMaxSize().padding(padding)) {
            error?.let { Problem(it) { refresh++ } }
            OutlinedTextField(
                query,
                { query = it },
                placeholder = { Text(words("搜索", "Search")) },
                leadingIcon = { Icon(Icons.Outlined.Search, null) },
                modifier = Modifier.fillMaxWidth().padding(horizontal = 20.dp, vertical = 8.dp),
                singleLine = true,
                shape = androidx.compose.foundation.shape.RoundedCornerShape(24.dp),
            )
            if (!loaded) {
                LinearProgressIndicator(Modifier.fillMaxWidth())
                return@Column
            }
            when (tab) {
                0 -> {
                    val candidates = overview.agents.filter { it.agent.kind != "machine" }
                    val ordering = recent.mapNotNull { row ->
                        row.participants.firstOrNull { it != me }
                            ?: row.last.let { if (it.from == me) it.to else it.from }
                    }
                    val agents =
                        candidates
                            .sortedBy {
                                ordering.indexOf(it.agent.name).let { i ->
                                    if (i < 0) Int.MAX_VALUE else i
                                }
                            }
                            .filter {
                                query.isBlank() ||
                                    (it.agent.title + it.agent.description).contains(query, true)
                            }
                    if (agents.isEmpty())
                        Empty(
                            words("还没有对话", "No conversations yet"),
                            words("添加成员后，就可以开始交流", "Connect an agent to start talking"),
                        )
                    LazyColumn(
                        contentPadding = PaddingValues(horizontal = 12.dp, vertical = 4.dp)
                    ) {
                        items(agents, key = { it.agent.id }) { item ->
                            val last =
                                recent
                                    .firstOrNull { row ->
                                        item.agent.name in row.participants ||
                                            row.last.to == item.agent.name ||
                                            row.last.from == item.agent.name
                                    }
                                    ?.last
                            PersonRow(item, last?.readableBody ?: item.agent.description) {
                                if (me != null) route = "chat:${item.agent.name}"
                            }
                        }
                    }
                }
                1 -> {
                    val filtered = projects.filter {
                        (it.displayTitle + it.goal).contains(query, true)
                    }
                    if (filtered.isEmpty())
                        Empty(
                            words("还没有项目", "No projects yet"),
                            words("在电脑上创建的项目会显示在这里", "Projects from your workspace appear here"),
                        )
                    LazyColumn(contentPadding = PaddingValues(12.dp)) {
                        items(filtered, key = { it.name }) { item ->
                            ListItem(
                                headlineContent = { Text(item.displayTitle) },
                                supportingContent = {
                                    Text(item.goal, maxLines = 2, overflow = TextOverflow.Ellipsis)
                                },
                                leadingContent = { Icon(Icons.Outlined.FolderOpen, null) },
                                trailingContent = { Icon(Icons.Outlined.ChevronRight, null) },
                                modifier = Modifier.clickable { route = "project:${item.name}" },
                            )
                        }
                    }
                }
                else ->
                    LazyColumn(
                        contentPadding = PaddingValues(horizontal = 12.dp, vertical = 4.dp)
                    ) {
                        item { Section(words("工作", "Work")) }
                        workEntries.forEach { (id, label, icon) ->
                            item(id) {
                                ListItem(
                                    headlineContent = { Text(label) },
                                    leadingContent = { Icon(icon, null) },
                                    trailingContent = { Icon(Icons.Outlined.ChevronRight, null) },
                                    modifier = Modifier.clickable { route = id },
                                )
                            }
                        }
                        item { Section(words("成员", "Members")) }
                        items(
                            overview.agents.filter {
                                it.agent.kind != "machine" &&
                                    (it.agent.title + it.agent.description).contains(query, true)
                            },
                            key = { it.agent.id },
                        ) { item ->
                            PersonRow(item, item.agent.description) {
                                route = "member:${item.agent.name}"
                            }
                        }
                    }
            }
        }
    }
}

@Composable
private fun ConversationRoute(
    hub: Hub,
    me: String,
    agent: String,
    workspace: String,
    project: String?,
    store: ConversationStore,
    title: String,
    back: () -> Unit,
    file: (Attachment) -> Unit,
    export: (String) -> Unit,
    settings: () -> Unit,
    openLink: (String) -> Boolean,
) {
    val scope = rememberCoroutineScope()
    val controller =
        remember(hub, me, agent, project) {
            ConversationController(
                hub,
                me,
                agent,
                "$workspace:$agent:${project.orEmpty()}",
                store,
                scope,
                project,
            )
        }
    ChatScreen(controller, title, back, file, export, settings, openLink)
}

@Composable
private fun Section(title: String) {
    Text(
        title,
        style = MaterialTheme.typography.labelLarge,
        color = MaterialTheme.colorScheme.onSurfaceVariant,
        modifier = Modifier.padding(start = 16.dp, top = 24.dp, bottom = 10.dp),
    )
}

@Composable
private fun PersonRow(item: AgentStatus, subtitle: String, click: () -> Unit) {
    ListItem(
        headlineContent = { Text(item.agent.title, maxLines = 1) },
        supportingContent = {
            Text(subtitle.take(200), maxLines = 2, overflow = TextOverflow.Ellipsis)
        },
        leadingContent = { Mushroom(item.agent.name, working = item.state == "working") },
        trailingContent = {
            Text(
                if (item.agent.online) words("在线", "Online") else words("离线", "Offline"),
                style = MaterialTheme.typography.labelSmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        },
        modifier = Modifier.clickable(onClick = click).padding(vertical = 4.dp),
    )
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun ProjectScreen(
    project: Project,
    agents: List<AgentStatus>,
    back: () -> Unit,
    chat: (String) -> Unit,
) {
    Scaffold(
        topBar = {
            TopAppBar(
                title = { Text(project.displayTitle) },
                navigationIcon = {
                    IconButton(onClick = back) {
                        Icon(Icons.AutoMirrored.Outlined.ArrowBack, words("返回", "Back"))
                    }
                },
            )
        }
    ) { padding ->
        LazyColumn(Modifier.padding(padding), contentPadding = PaddingValues(20.dp)) {
            item {
                Text(project.goal, style = MaterialTheme.typography.titleLarge)
                Spacer(Modifier.height(20.dp))
                Markdown(project.brief)
            }
            item { Section(words("一起做这件事", "Working on this")) }
            items(project.members, key = { it.name }) { member ->
                agents
                    .firstOrNull { it.agent.name == member.name }
                    ?.let {
                        PersonRow(it, member.note.ifBlank { it.agent.description }) {
                            chat(member.name)
                        }
                    }
            }
        }
    }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun MemberScreen(
    status: AgentStatus,
    who: Whoami?,
    hub: HubClient?,
    back: () -> Unit,
    chat: () -> Unit,
) {
    val agent = status.agent
    var model by remember { mutableStateOf(agent.runtime?.model.orEmpty()) }
    var effort by remember { mutableStateOf(agent.runtime?.effort.orEmpty()) }
    var failure by remember { mutableStateOf<String?>(null) }
    var busy by remember { mutableStateOf(false) }
    var saved by remember { mutableStateOf(false) }
    val scope = rememberCoroutineScope()
    Scaffold(
        topBar = {
            TopAppBar(
                title = { Text(agent.title) },
                navigationIcon = {
                    IconButton(onClick = back) {
                        Icon(Icons.AutoMirrored.Outlined.ArrowBack, words("返回", "Back"))
                    }
                },
            )
        }
    ) { padding ->
        Column(
            Modifier.fillMaxSize().padding(padding).padding(24.dp),
            verticalArrangement = Arrangement.spacedBy(16.dp),
        ) {
            Mushroom(agent.name, Modifier.size(72.dp))
            Text(agent.description, style = MaterialTheme.typography.bodyLarge)
            Text(agent.statusText.orEmpty(), color = MaterialTheme.colorScheme.onSurfaceVariant)
            Button(onClick = chat) { Text(words("发消息", "Message")) }
            Text(
                "${agent.binding.environment} · ${agent.runtime?.cli ?: agent.binding.harness}",
                style = MaterialTheme.typography.bodySmall,
            )
            if (
                hub != null &&
                    (who?.canManage == true || who?.member?.id == agent.ownerId) &&
                    status.receiving == "runner"
            ) {
                OutlinedTextField(
                    model,
                    {
                        model = it
                        saved = false
                    },
                    label = { Text(words("模型", "Model")) },
                    modifier = Modifier.fillMaxWidth(),
                )
                OutlinedTextField(
                    effort,
                    {
                        effort = it
                        saved = false
                    },
                    label = { Text(words("推理强度", "Reasoning effort")) },
                    modifier = Modifier.fillMaxWidth(),
                )
                Button(
                    enabled = !busy,
                    onClick = {
                        scope.launch {
                            busy = true
                            try {
                                hub.request(
                                    "PUT",
                                    listOf("v1", "agents", agent.name, "run-settings"),
                                    body =
                                        buildJsonObject {
                                            put("model", model)
                                            put("effort", effort)
                                        },
                                )
                                saved = true
                                failure = null
                            } catch (e: CancellationException) {
                                throw e
                            } catch (e: Exception) {
                                failure = e.message
                            } finally {
                                busy = false
                            }
                        }
                    },
                ) {
                    Text(words("保存", "Save"))
                }
                if (saved)
                    Text(
                        words("下次开始处理任务时生效", "Takes effect on the next task"),
                        style = MaterialTheme.typography.bodySmall,
                    )
            } else
                Text(
                    "${agent.runtime?.model.orEmpty()} ${agent.runtime?.effort.orEmpty()}",
                    style = MaterialTheme.typography.bodySmall,
                )
            failure?.let { Problem(it) }
        }
    }
}
