plugins {
    id("com.android.library")
    id("org.jetbrains.kotlin.android")
    id("org.jetbrains.kotlin.plugin.serialization")
    id("io.gitlab.arturbosch.detekt")
}
detekt { config.setFrom(rootProject.file("config/detekt/detekt.yml")); buildUponDefaultConfig = true; parallel = true; autoCorrect = false }
android { namespace = "com.anyaicliremote.core.storage"; compileSdk = 35; defaultConfig { minSdk = 26 }; compileOptions { sourceCompatibility = JavaVersion.VERSION_17; targetCompatibility = JavaVersion.VERSION_17 }; kotlinOptions { jvmTarget = "17" } }
dependencies { api(project(":core:model")); implementation("androidx.security:security-crypto:1.1.0"); implementation("org.jetbrains.kotlinx:kotlinx-serialization-json:1.7.3"); testImplementation("junit:junit:4.13.2") }
