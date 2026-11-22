package dev.myco.network

import org.junit.Assert.*
import org.junit.Test

class PageLinksTest {
    private val workspace = "https://hub.example/w/team"

    @Test
    fun oldPageLinksAndWorkspacePrefixesRemainSupported() {
        assertEquals(
            PageLink("pg_movie", null),
            PageLinks.parse("$workspace/pages/pg_movie", workspace),
        )
        assertEquals(
            PageLink("pg_movie", null),
            PageLinks.parse("$workspace/pages/pg_movie/", "$workspace/"),
        )
        assertEquals(
            PageLink("pg_123", ""),
            PageLinks.parse("https://HUB.example:443/pages/pg_123#", "https://hub.example"),
        )
    }

    @Test
    fun fragmentsAreOpaqueAndPreserveChineseEncoding() {
        val encoded =
            "watch?source=r1&movie=m1&line=%E4%B8%AD%E6%96%87&episode=%E7%AC%AC%E4%B8%80%E9%9B%86"
        assertEquals(
            PageLink("pg_movie", encoded),
            PageLinks.parse("$workspace/pages/pg_movie#$encoded", workspace),
        )
        assertEquals(
            "bad%xx",
            PageLinks.parse("$workspace/pages/pg_movie#bad%xx", workspace)?.fragment,
        )
        val unicode = "watch?line=中文&episode=第一集"
        assertEquals(
            unicode,
            PageLinks.parse("$workspace/pages/pg_movie#$unicode", workspace)?.fragment,
        )
        assertEquals(
            "a#b?x=y&unknown=z",
            PageLinks.parse("$workspace/pages/pg_movie#a#b?x=y&unknown=z", workspace)?.fragment,
        )
    }

    @Test
    fun fragmentLimitIsEnforcedBeforeOpeningTheHost() {
        assertNotNull(PageLinks.parse("$workspace/pages/pg_movie#" + "a".repeat(1024), workspace))
        assertNull(PageLinks.parse("$workspace/pages/pg_movie#" + "a".repeat(1025), workspace))
    }

    @Test
    fun otherWorkspacesOriginsAndOrdinaryLinksStayExternal() {
        listOf(
                "https://other.example/w/team/pages/pg_movie",
                "http://hub.example/w/team/pages/pg_movie",
                "https://hub.example:8443/w/team/pages/pg_movie",
                "https://hub.example/w/other/pages/pg_movie",
                "https://hub.example/pages/pg_movie",
                "$workspace/help",
                "mailto:person@example.com",
            )
            .forEach {
                assertNull(it, PageLinks.parse(it, workspace))
            }
    }

    @Test
    fun credentialsQueriesInvalidIdsAndAmbiguousPathsStayExternal() {
        listOf(
                "https://person:secret@hub.example/w/team/pages/pg_movie",
                "$workspace/pages/pg_movie?source=r1#watch",
                "$workspace/pages/movie",
                "$workspace/pages/pg_bad-id",
                "$workspace/pages/pg_movie/file",
                "$workspace/pages/pg_movie//",
                "$workspace/pages/%70g_movie",
                "$workspace/a/../pages/pg_movie",
                "$workspace/pages/pg_movie#bad\nroute",
            )
            .forEach {
                assertNull(it, PageLinks.parse(it, workspace))
            }
    }
}
