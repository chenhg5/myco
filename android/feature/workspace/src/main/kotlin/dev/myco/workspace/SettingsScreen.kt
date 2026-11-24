package dev.myco.workspace

import androidx.compose.foundation.layout.*
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.selection.selectable
import androidx.compose.foundation.selection.selectableGroup
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.unit.dp
import dev.myco.model.AppPreferences
import dev.myco.model.Workspace
import dev.myco.ui.words
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.launch

/** Values and actions only; storage and workspace credentials belong to the composition root. */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun SettingsScreen(
    workspace: Workspace?,
    preferences: AppPreferences,
    changePreferences: ((AppPreferences) -> AppPreferences) -> Unit,
    back: () -> Unit,
    join: () -> Unit,
    leave: () -> Unit,
    rename: suspend (String) -> Unit,
) {
    var confirm by remember { mutableStateOf(false) }
    var renaming by remember { mutableStateOf(false) }
    Scaffold(
        topBar = {
            TopAppBar(
                title = { Text(words("设置", "Settings")) },
                navigationIcon = { TextButton(onClick = back) { Text(words("返回", "Back")) } },
            )
        }
    ) { padding ->
        Column(
            Modifier.fillMaxSize()
                .padding(padding)
                .verticalScroll(rememberScrollState())
                .padding(24.dp),
            verticalArrangement = Arrangement.spacedBy(16.dp),
        ) {
            Text(words("外观", "Appearance"), style = MaterialTheme.typography.titleLarge)
            Text(words("主题", "Theme"), style = MaterialTheme.typography.titleSmall)
            Choices(
                "theme",
                preferences.appearance,
                listOf(
                    "system" to words("跟随系统", "System"),
                    "light" to words("浅色", "Light"),
                    "dark" to words("深色", "Dark"),
                ),
            ) { value ->
                changePreferences { it.copy(appearance = value) }
            }
            Text(words("语言", "Language"), style = MaterialTheme.typography.titleSmall)
            Choices(
                "language",
                preferences.language,
                listOf("system" to words("跟随系统", "System"), "zh" to "中文", "en" to "English"),
            ) { value ->
                changePreferences { it.copy(language = value) }
            }
            Text(
                words(
                    "立即生效，也用于应用里的网页。仅保存在这台设备上。",
                    "Applies immediately, including to apps you open. Saved only on this device.",
                ),
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
            if (workspace != null) {
                Spacer(Modifier.height(8.dp))
                Text(words("工作区", "Workspace"), style = MaterialTheme.typography.titleLarge)
                Text(workspace.name, style = MaterialTheme.typography.titleMedium)
                Text(
                    workspace.url,
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
                TextButton(
                    onClick = { renaming = true },
                    modifier = Modifier.testTag("workspace-rename"),
                ) {
                    Text(words("给这个工作区改名", "Rename this workspace"))
                }
                OutlinedButton(onClick = join) { Text(words("加入另一个工作区", "Join another workspace")) }
                OutlinedButton(onClick = { confirm = true }) {
                    Text(words("离开此工作区", "Leave this workspace"))
                }
            }
            Spacer(Modifier.height(8.dp))
            Text("myco for Android · 0.1.0", style = MaterialTheme.typography.bodySmall)
        }
    }
    if (renaming) RenameWorkspace(workspace?.name.orEmpty(), { renaming = false }, rename)
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

@Composable
private fun Choices(
    group: String,
    selected: String,
    options: List<Pair<String, String>>,
    choose: (String) -> Unit,
) {
    Column(Modifier.selectableGroup()) {
        options.forEach { (value, label) ->
            Row(
                Modifier.fillMaxWidth()
                    .heightIn(min = 48.dp)
                    .testTag("settings-$group-$value")
                    .selectable(
                        selected == value,
                        role = Role.RadioButton,
                        onClick = { choose(value) },
                    )
                    .padding(vertical = 10.dp),
                verticalAlignment = androidx.compose.ui.Alignment.CenterVertically,
            ) {
                RadioButton(selected == value, onClick = null)
                Spacer(Modifier.width(12.dp))
                Text(label, style = MaterialTheme.typography.bodyLarge)
            }
        }
    }
}

@Composable
private fun RenameWorkspace(original: String, dismiss: () -> Unit, save: suspend (String) -> Unit) {
    var text by rememberSaveable { mutableStateOf(original) }
    var busy by remember { mutableStateOf(false) }
    var failed by remember { mutableStateOf(false) }
    val scope = rememberCoroutineScope()
    val valid =
        text.trim().isNotEmpty() && text.trim().length <= 80 && text.none { it.isISOControl() }
    AlertDialog(
        onDismissRequest = { if (!busy) dismiss() },
        title = { Text(words("给工作区改名", "Rename workspace")) },
        text = {
            Column(verticalArrangement = Arrangement.spacedBy(12.dp)) {
                Text(words("只修改此设备上的名称，不影响其他成员。", "Only the name on this device changes, for you."))
                OutlinedTextField(
                    text,
                    {
                        text = it
                        failed = false
                    },
                    singleLine = true,
                    enabled = !busy,
                    label = { Text(words("名称", "Name")) },
                    isError = !valid,
                    modifier = Modifier.fillMaxWidth().testTag("workspace-name"),
                )
                if (!valid)
                    Text(
                        words("请输入 1–80 个字符。", "Use 1–80 characters."),
                        style = MaterialTheme.typography.bodySmall,
                    )
                if (failed)
                    Text(
                        words("名称没有保存，请重试。", "The name was not saved. Try again."),
                        color = MaterialTheme.colorScheme.error,
                    )
            }
        },
        confirmButton = {
            TextButton(
                enabled = valid && !busy,
                onClick = {
                    scope.launch {
                        busy = true
                        try {
                            save(text.trim())
                            dismiss()
                        } catch (e: CancellationException) {
                            throw e
                        } catch (_: Exception) {
                            failed = true
                        } finally {
                            busy = false
                        }
                    }
                },
            ) {
                Text(if (busy) words("保存中…", "Saving…") else words("保存", "Save"))
            }
        },
        dismissButton = {
            TextButton(enabled = !busy, onClick = dismiss) { Text(words("取消", "Cancel")) }
        },
    )
}
