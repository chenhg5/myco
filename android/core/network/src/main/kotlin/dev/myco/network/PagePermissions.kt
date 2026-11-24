package dev.myco.network

import kotlinx.serialization.json.*

/**
 * Whether a page's app may use a permission ("send", "download"). From hubs that say its
 * declarations are all it declares (template.declares, hubs from v0.22.8), only what it lists in
 * "permissions"; myco's own apps that declare nothing yet ("os" empty) may, for now. Pages with
 * no app and older hubs: as before.
 */
fun pageAllows(page: JsonElement, permission: String): Boolean {
    val template = (page as? JsonObject)?.get("template") as? JsonObject ?: return true
    if ((template["declares"] as? JsonPrimitive)?.booleanOrNull != true) return true
    fun list(key: String) =
        (template[key] as? JsonArray)?.mapNotNull { (it as? JsonPrimitive)?.contentOrNull }.orEmpty()
    if (permission in list("permissions")) return true
    return (template["official"] as? JsonPrimitive)?.booleanOrNull == true && list("os").isEmpty()
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
