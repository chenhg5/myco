package dev.myco.network

import kotlinx.serialization.json.*
import org.junit.Assert.*
import org.junit.Test

class PagePermissionsTest {
    private fun page(template: String?) =
        Json.parseToJsonElement(if (template == null) """{"id":"pg_a"}""" else """{"id":"pg_a","template":$template}""")

    @Test
    fun anAppMayNotUseWhatItDoesNotDeclare() {
        val strict = page("""{"id":"tp_a","permissions":["send"],"declares":true}""")
        assertTrue(pageAllows(strict, "send"))
        assertFalse(pageAllows(strict, "download"))
        val refusal = assertThrows(DownloadRefusal::class.java) { requireDeclared(strict, "download") }
        assertEquals("permission_denied", refusal.code)
        // No app, an older hub, a page of myco's own apps kept from before declarations
        // (the hub does not say declares): as before.
        assertTrue(pageAllows(page(null), "download"))
        assertTrue(pageAllows(page("""{"id":"tp_a"}"""), "download"))
        assertTrue(pageAllows(page("""{"id":"tp_myco_doc","latest":"2.2.0","official":true}"""), "download"))
        // myco's own apps are held to what they declare.
        assertFalse(pageAllows(page("""{"id":"tp_myco_doc","declares":true,"official":true}"""), "download"))
        assertFalse(pageAllows(page("""{"id":"tp_myco_disk","os":["files"],"declares":true,"official":true}"""), "download"))
    }
}
