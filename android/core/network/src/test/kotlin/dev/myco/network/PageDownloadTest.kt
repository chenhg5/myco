package dev.myco.network

import java.util.Base64
import kotlinx.serialization.json.*
import org.junit.Assert.*
import org.junit.Test

class PageDownloadTest {
    private fun request(name: String, text: String = "hello") = buildJsonObject {
        put("name", name)
        put("type", "application/x-executable")
        put("base64", Base64.getEncoder().encodeToString(text.toByteArray()))
    }

    @Test
    fun namesAreLeafNamesAndContentsAndMimeArePreserved() {
        val file = PageDownload.decode(request("../private/报告.PDF", "中文"))
        assertEquals("报告.pdf", file.name)
        assertEquals("application/pdf", file.type)
        assertArrayEquals("中文".toByteArray(), file.bytes)
        assertEquals("图表.csv", PageDownload.decode(request("C:\\folder\\图表.csv")).name)
        assertEquals(
            "noteshidden.txt",
            PageDownload.decode(request(".notes\u202Ehidden\u0000.txt")).name,
        )
        assertTrue(PageDownload.decode(request("a".repeat(200) + ".xlsx")).name.endsWith(".xlsx"))
        assertTrue(PageDownload.decode(request("a".repeat(200) + ".xlsx")).name.length <= 120)
    }

    @Test
    fun unsupportedFilesAndInvalidBase64AreRejectedBeforeOpeningAPicker() {
        listOf("install.apk", "run.exe", "command.sh", "../", ".", "file").forEach { name ->
            try {
                PageDownload.decode(request(name))
                fail(name)
            } catch (e: DownloadRefusal) {
                assertEquals("invalid_argument", e.code)
            }
        }
        try {
            PageDownload.decode(
                buildJsonObject {
                    put("name", "notes.txt")
                    put("base64", "%%%")
                }
            )
            fail()
        } catch (e: DownloadRefusal) {
            assertEquals("invalid_argument", e.code)
        }
    }

    @Test
    fun decodedSizeIsCheckedEvenWhenAnExtraByteFitsInTheBase64LengthLimit() {
        val encoded = "A".repeat(((PageDownload.maxBytes + 2) / 3) * 4)
        try {
            PageDownload.decode(
                buildJsonObject {
                    put("name", "large.pdf")
                    put("base64", encoded)
                }
            )
            fail()
        } catch (e: DownloadRefusal) {
            assertEquals("too_large", e.code)
        }
        try {
            PageDownload.decode(
                buildJsonObject {
                    put("name", "large.pdf")
                    put("base64", encoded + "AAAA")
                }
            )
            fail()
        } catch (e: DownloadRefusal) {
            assertEquals("too_large", e.code)
        }
    }
}
