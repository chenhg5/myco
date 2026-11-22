pluginManagement {
    repositories {
        google()
        mavenCentral()
        gradlePluginPortal()
    }
}

dependencyResolutionManagement {
    repositoriesMode.set(RepositoriesMode.FAIL_ON_PROJECT_REPOS)
    repositories {
        google()
        mavenCentral()
    }
}

rootProject.name = "myco-android"

include(
    ":app",
    ":core:model",
    ":core:network",
    ":core:conversation",
    ":core:storage",
    ":core:ui",
    ":feature:chat",
    ":feature:workspace",
    ":feature:pages",
)
