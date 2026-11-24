package dev.myco.model

import kotlinx.serialization.Serializable

/** Device choices, separate from workspace credentials and shared page state. */
@Serializable
data class AppPreferences(val appearance: String = "system", val language: String = "system") {
    fun normalized() =
        AppPreferences(
            appearance.takeIf { it in setOf("system", "light", "dark") } ?: "system",
            language.takeIf { it in setOf("system", "zh", "en") } ?: "system",
        )
}
