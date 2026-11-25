package dev.myco.network

import kotlinx.serialization.json.*

/**
 * Whether a page's app may use a permission ("send", "download"). From hubs that say its
 * declarations are all it declares (template.declares, hubs from v0.22.9), only what it lists in
 * "permissions". Pages with no app, older hubs, and pages of myco's own apps kept from before
 * declarations (the hub does not say it for them until they are upgraded): as before.
 */
fun pageAllows(page: JsonElement, permission: String): Boolean {
    val template = (page as? JsonObject)?.get("template") as? JsonObject ?: return true
    if ((template["declares"] as? JsonPrimitive)?.booleanOrNull != true) return true
    val permissions =
        (template["permissions"] as? JsonArray)?.mapNotNull { (it as? JsonPrimitive)?.contentOrNull }.orEmpty()
    return permission in permissions
}

/** Refuses (permission_denied) what the page's app does not declare. */
fun requireDeclared(page: JsonElement, permission: String) {
    if (!pageAllows(page, permission))
        throw DownloadRefusal(
            "permission_denied",
            "This app does not declare \"$permission\" in myco.json's \"permissions\"",
            "这个应用没有在 myco.json 的 \"permissions\" 里写 \"$permission\"，不能用",
        )
}
