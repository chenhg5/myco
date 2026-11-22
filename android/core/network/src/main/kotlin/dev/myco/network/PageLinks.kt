package dev.myco.network

import java.net.URI
import okhttp3.HttpUrl.Companion.toHttpUrlOrNull

/** A validated workspace page location. The fragment never enters a Hub request. */
data class PageLink(val pageId: String, val fragment: String?)

object PageLinks {
    const val maxFragmentLength = 1024
    private val pageId = Regex("pg_[a-z0-9]+", RegexOption.IGNORE_CASE)

    fun parse(link: String, workspace: String): PageLink? = runCatching {
        val address = link.substringBefore('#')
        val fragment = if ('#' in link) link.substringAfter('#') else null
        if (
            fragment != null &&
                (fragment.length > maxFragmentLength || fragment.any { it < ' ' || it == '\u007f' })
        )
            return null
        val raw = URI(address)
        val baseRaw = URI(workspace)
        val target = address.toHttpUrlOrNull() ?: return null
        val base = workspace.toHttpUrlOrNull() ?: return null
        if (raw.rawUserInfo != null || baseRaw.rawUserInfo != null || raw.rawQuery != null)
            return null
        if (target.scheme != base.scheme || target.host != base.host || target.port != base.port)
            return null
        val prefix = baseRaw.rawPath.orEmpty().trimEnd('/') + "/pages/"
        val path = raw.rawPath.orEmpty()
        if (!path.startsWith(prefix)) return null
        val id = path.removePrefix(prefix).removeSuffix("/")
        if (!pageId.matches(id)) return null
        PageLink(id, fragment)
    }
        .getOrNull()
}
