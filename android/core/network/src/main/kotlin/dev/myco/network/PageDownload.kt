package dev.myco.network

import java.text.Normalizer
import java.util.Base64
import kotlinx.serialization.json.*

class DownloadRefusal(val code: String, message: String, val chinese: String) :
    IllegalArgumentException(message)

/** A bounded page export, not an arbitrary path or network request. */
class PageDownload private constructor(val name: String, val type: String, val bytes: ByteArray) {
    override fun toString() = "PageDownload(name=$name, bytes=${bytes.size})"

    companion object {
        const val maxBytes = 20 * 1024 * 1024
        private val types =
            mapOf(
                "txt" to "text/plain",
                "md" to "text/markdown",
                "csv" to "text/csv",
                "tsv" to "text/tab-separated-values",
                "json" to "application/json",
                "pdf" to "application/pdf",
                "zip" to "application/zip",
                "ics" to "text/calendar",
                "html" to "text/html",
                "xlsx" to "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
                "xls" to "application/vnd.ms-excel",
                "docx" to "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
                "pptx" to
                    "application/vnd.openxmlformats-officedocument.presentationml.presentation",
                "png" to "image/png",
                "jpg" to "image/jpeg",
                "jpeg" to "image/jpeg",
                "svg" to "image/svg+xml",
                "gif" to "image/gif",
                "webp" to "image/webp",
                "mp3" to "audio/mpeg",
                "wav" to "audio/wav",
                "m4a" to "audio/mp4",
                "mp4" to "video/mp4",
                "mov" to "video/quicktime",
                "webm" to "video/webm",
            )

        fun decode(args: JsonObject): PageDownload {
            val rawName =
                args["name"]?.jsonPrimitive?.contentOrNull.orEmpty().ifBlank { "download.txt" }
            val leaf =
                Normalizer.normalize(
                        rawName.substringAfterLast('/').substringAfterLast('\\'),
                        Normalizer.Form.NFC,
                    )
                    .filterNot {
                        it.isISOControl() || Character.getType(it) == Character.FORMAT.toInt()
                    }
                    .replace(Regex("[<>:\"|?*]"), "_")
                    .trim()
                    .trimStart('.')
                    .trimEnd('.', ' ')
            val extension = leaf.substringAfterLast('.', "").lowercase()
            val type =
                types[extension]
                    ?: throw DownloadRefusal(
                        "invalid_argument",
                        "Choose a document, picture, audio or video file name",
                        "请使用文档、图片、音频或视频的文件名",
                    )
            val base =
                leaf.substringBeforeLast('.').take(120 - extension.length - 1).ifBlank {
                    "download"
                }
            val name = "$base.$extension"
            val encoded =
                args["base64"]?.jsonPrimitive?.contentOrNull
                    ?: throw DownloadRefusal(
                        "invalid_argument",
                        "The file could not be read",
                        "文件内容读不出来",
                    )
            if (encoded.length > ((maxBytes + 2) / 3) * 4) tooLarge()
            val bytes =
                try {
                    Base64.getDecoder().decode(encoded)
                } catch (_: IllegalArgumentException) {
                    throw DownloadRefusal(
                        "invalid_argument",
                        "The file could not be read",
                        "文件内容读不出来",
                    )
                }
            if (bytes.size > maxBytes) tooLarge()
            return PageDownload(name, type, bytes)
        }

        private fun tooLarge(): Nothing =
            throw DownloadRefusal("too_large", "Files can be up to 20 MiB", "文件不能超过 20 MiB")
    }
}
