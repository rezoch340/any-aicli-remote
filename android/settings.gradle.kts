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

rootProject.name = "AnyAICLIRemote"
include(":app", ":feature:ui", ":core:model", ":core:remote", ":core:storage", ":core:session", ":core:chat")
