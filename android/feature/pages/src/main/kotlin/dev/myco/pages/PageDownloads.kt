package dev.myco.pages

import android.content.Context
import android.content.Intent
import android.net.Uri
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.contract.ActivityResultContract
import androidx.compose.runtime.*
import androidx.compose.ui.platform.LocalContext
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.compose.LocalLifecycleOwner
import dev.myco.network.DownloadRefusal
import dev.myco.network.PageDownload
import java.io.IOException
import kotlin.coroutines.resume
import kotlin.coroutines.resumeWithException
import kotlinx.coroutines.*
import kotlinx.coroutines.flow.first
import kotlinx.serialization.json.JsonObject

private class SavePageDocument : ActivityResultContract<PageDownload, Uri?>() {
    override fun createIntent(context: Context, input: PageDownload) =
        Intent(Intent.ACTION_CREATE_DOCUMENT)
            .addCategory(Intent.CATEGORY_OPENABLE)
            .setType(input.type)
            .putExtra(Intent.EXTRA_TITLE, input.name)

    override fun parseResult(resultCode: Int, intent: Intent?): Uri? =
        if (resultCode == android.app.Activity.RESULT_OK) intent?.data else null
}

private data class PendingDownload(
    val file: PageDownload,
    val answer: CancellableContinuation<Boolean>,
)

/** The system picker owns the destination. No storage permission or page-provided path is used. */
@Composable
internal fun rememberPageDownload(failed: () -> Unit): suspend (JsonObject) -> Boolean {
    val context = LocalContext.current
    val scope = rememberCoroutineScope()
    val lifecycle = LocalLifecycleOwner.current.lifecycle
    val currentFailed by rememberUpdatedState(failed)
    var pending by remember { mutableStateOf<PendingDownload?>(null) }
    var busy by remember { mutableStateOf(false) }
    val launcher =
        rememberLauncherForActivityResult(SavePageDocument()) { uri ->
            val request = pending
            pending = null
            if (request != null && request.answer.isActive)
                scope.launch {
                    try {
                        if (uri != null)
                            withContext(Dispatchers.IO) {
                                if (uri.scheme != "content")
                                    throw IOException("Unsupported document destination")
                                context.contentResolver.openOutputStream(uri, "wt")?.use {
                                    it.write(request.file.bytes)
                                } ?: throw IOException("The document could not be opened")
                            }
                        if (request.answer.isActive) request.answer.resume(uri != null)
                    } catch (e: CancellationException) {
                        throw e
                    } catch (_: Exception) {
                        currentFailed()
                        if (request.answer.isActive)
                            request.answer.resumeWithException(
                                DownloadRefusal(
                                    "save_failed",
                                    "The file was not saved. Try again.",
                                    "文件没有保存，请重试。",
                                )
                            )
                    }
                }
        }
    LaunchedEffect(pending) {
        val request = pending ?: return@LaunchedEffect
        try {
            lifecycle.currentStateFlow.first { it.isAtLeast(Lifecycle.State.RESUMED) }
            launcher.launch(request.file)
        } catch (e: CancellationException) {
            throw e
        } catch (_: Exception) {
            pending = null
            currentFailed()
            if (request.answer.isActive)
                request.answer.resumeWithException(
                    DownloadRefusal(
                        "save_failed",
                        "No document saver is available",
                        "此设备无法打开文件保存窗口",
                    )
                )
        }
    }
    DisposableEffect(Unit) {
        onDispose {
            pending?.answer?.cancel()
            pending = null
        }
    }
    return { args ->
        if (busy)
            throw DownloadRefusal("busy", "Finish saving the current file first", "请先完成当前文件的保存")
        busy = true
        try {
            val file = withContext(Dispatchers.IO) { PageDownload.decode(args) }
            withTimeout(600_000) {
                suspendCancellableCoroutine { c ->
                    val request = PendingDownload(file, c)
                    pending = request
                    c.invokeOnCancellation {
                        scope.launch { if (pending === request) pending = null }
                    }
                }
            }
        } finally {
            busy = false
        }
    }
}
