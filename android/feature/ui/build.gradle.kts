plugins {
    id("com.android.library")
    id("org.jetbrains.kotlin.android")
    id("org.jetbrains.kotlin.plugin.compose")
    id("org.jetbrains.kotlin.plugin.serialization")
    id("io.gitlab.arturbosch.detekt")
}

android {
    namespace = "com.anyaicliremote.feature.ui"
    compileSdk = 35
    defaultConfig { minSdk = 26 }
    compileOptions { sourceCompatibility = JavaVersion.VERSION_17; targetCompatibility = JavaVersion.VERSION_17 }
    kotlinOptions { jvmTarget = "17" }
    buildFeatures { compose = true }
    lint { disable += "NullSafeMutableLiveData"; disable += "NonNullableMutableLiveData"; disable += "FrequentlyChangingValue"; disable += "RememberInComposition"; disable += "AutoboxingStateCreation" }
}
detekt { config.setFrom(rootProject.file("config/detekt/detekt.yml")); buildUponDefaultConfig = true; parallel = true; autoCorrect = false }

dependencies {
    implementation(project(":core:model")); implementation(project(":core:remote")); implementation(project(":core:storage")); implementation(project(":core:session")); implementation(project(":core:chat"))
    val composeBom = platform("androidx.compose:compose-bom:2025.09.00")
    implementation(composeBom); implementation("androidx.compose.material3:material3"); implementation("androidx.compose.material:material-icons-extended"); implementation("androidx.compose.ui:ui"); implementation("androidx.compose.ui:ui-tooling-preview"); debugImplementation("androidx.compose.ui:ui-tooling")
    implementation("androidx.lifecycle:lifecycle-runtime-ktx:2.8.7"); implementation("androidx.lifecycle:lifecycle-runtime-compose:2.8.7"); implementation("androidx.lifecycle:lifecycle-viewmodel-compose:2.8.7")
    implementation("org.jetbrains.kotlinx:kotlinx-serialization-json:1.7.3"); implementation("org.jetbrains.kotlinx:kotlinx-coroutines-android:1.9.0"); implementation("com.mikepenz:multiplatform-markdown-renderer-android:0.38.1"); implementation("com.mikepenz:multiplatform-markdown-renderer-m3-android:0.38.1"); implementation("com.mikepenz:multiplatform-markdown-renderer-code-android:0.38.1")
    testImplementation("junit:junit:4.13.2")
}
