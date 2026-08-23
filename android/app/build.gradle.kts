plugins {
    id("com.android.application")
    id("org.jetbrains.kotlin.android")
    id("org.jetbrains.kotlin.plugin.compose")
    id("org.jetbrains.kotlin.plugin.serialization")
}

val productApplicationIdentifier = "com.anyaicliremote.app"
val productDisplayName = "Any AI CLI Remote"
val productPairingScheme = "anyaicliremote"
val legacyPairingScheme = "grokremote"
val productAuthorizationHeader = "X-Any-AI-CLI-Remote-Key"
val legacyAuthorizationHeader = "X-Grok-Remote-Key"
val productPreferencesName = "any_aicli_remote_profile"
val legacyPreferencesName = "grok_remote_profile"
val productClientName = "any-aicli-remote-app-android"
val legacyDisplayName = "Grok Remote"

android {
    namespace = productApplicationIdentifier
    compileSdk = 35

    defaultConfig {
        applicationId = productApplicationIdentifier
        minSdk = 26
        targetSdk = 35
        versionCode = 1
        versionName = "0.1.0"

        manifestPlaceholders["productDisplayName"] = productDisplayName
        manifestPlaceholders["productPairingScheme"] = productPairingScheme
        manifestPlaceholders["legacyPairingScheme"] = legacyPairingScheme

        buildConfigField("String", "PRODUCT_DISPLAY_NAME", "\"$productDisplayName\"")
        buildConfigField("String", "PRODUCT_PAIRING_SCHEME", "\"$productPairingScheme\"")
        buildConfigField("String", "LEGACY_PAIRING_SCHEME", "\"$legacyPairingScheme\"")
        buildConfigField("String", "PRODUCT_AUTHORIZATION_HEADER", "\"$productAuthorizationHeader\"")
        buildConfigField("String", "LEGACY_AUTHORIZATION_HEADER", "\"$legacyAuthorizationHeader\"")
        buildConfigField("String", "PRODUCT_PREFERENCES_NAME", "\"$productPreferencesName\"")
        buildConfigField("String", "LEGACY_PREFERENCES_NAME", "\"$legacyPreferencesName\"")
        buildConfigField("String", "PRODUCT_CLIENT_NAME", "\"$productClientName\"")
        buildConfigField("String", "LEGACY_DISPLAY_NAME", "\"$legacyDisplayName\"")

        testInstrumentationRunner = "androidx.test.runner.AndroidJUnitRunner"
    }

    buildTypes {
        release {
            isMinifyEnabled = false
            proguardFiles(getDefaultProguardFile("proguard-android-optimize.txt"), "proguard-rules.pro")
        }
    }

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }
    kotlinOptions { jvmTarget = "17" }
    buildFeatures {
        compose = true
        buildConfig = true
    }
    lint {
        // These bundled detectors are binary-incompatible with Kotlin 2.1 UAST in AGP 8.7.
        disable += "NullSafeMutableLiveData"
        disable += "NonNullableMutableLiveData"
        disable += "FrequentlyChangingValue"
        disable += "RememberInComposition"
        disable += "AutoboxingStateCreation"
    }
}

dependencies {
    val composeBom = platform("androidx.compose:compose-bom:2025.09.00")
    implementation(composeBom)
    implementation("androidx.compose.material3:material3")
    implementation("androidx.compose.material:material-icons-extended")
    implementation("androidx.compose.ui:ui")
    implementation("androidx.compose.ui:ui-tooling-preview")
    debugImplementation("androidx.compose.ui:ui-tooling")

    implementation("androidx.core:core-ktx:1.15.0")
    implementation("androidx.activity:activity-compose:1.9.3")
    implementation("androidx.lifecycle:lifecycle-runtime-ktx:2.8.7")
    implementation("androidx.lifecycle:lifecycle-runtime-compose:2.8.7")
    implementation("androidx.lifecycle:lifecycle-viewmodel-compose:2.8.7")
    implementation("androidx.security:security-crypto:1.1.0")

    implementation("com.squareup.okhttp3:okhttp:4.12.0")
    implementation("com.google.android.gms:play-services-code-scanner:16.1.0")
    implementation("org.jetbrains.kotlinx:kotlinx-serialization-json:1.7.3")
    implementation("org.jetbrains.kotlinx:kotlinx-coroutines-android:1.9.0")
    implementation("com.agentclientprotocol:acp-model-jvm:0.28.1")
    implementation("com.mikepenz:multiplatform-markdown-renderer-android:0.38.1")
    implementation("com.mikepenz:multiplatform-markdown-renderer-m3-android:0.38.1")

    testImplementation("junit:junit:4.13.2")

    androidTestImplementation("junit:junit:4.13.2")
    androidTestImplementation(composeBom)
    androidTestImplementation("androidx.test:core-ktx:1.7.0")
    androidTestImplementation("androidx.test.ext:junit:1.3.0")
    androidTestImplementation("androidx.test:runner:1.7.0")
    androidTestImplementation("androidx.test.uiautomator:uiautomator:2.4.0")
    androidTestImplementation("androidx.compose.ui:ui-test-junit4")
    androidTestImplementation("com.squareup.okhttp3:mockwebserver:4.12.0")
    debugImplementation("androidx.compose.ui:ui-test-manifest")
}
