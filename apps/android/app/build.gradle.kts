plugins {
    id("com.android.application")
    id("org.jetbrains.kotlin.android")
}

val mobileBridgeAar = file("libs/mobilebridge.aar")

android {
    namespace = "com.example.vibecoding"
    compileSdk = 35

    defaultConfig {
        applicationId = "com.example.vibecoding"
        minSdk = 26
        targetSdk = 35
        versionCode = 1
        versionName = "0.1.0"
    }

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_1_8
        targetCompatibility = JavaVersion.VERSION_1_8
    }

    kotlinOptions {
        jvmTarget = "1.8"
    }
}

dependencies {
    implementation(files("libs/mobilebridge.aar"))
}

tasks.named("preBuild") {
    doFirst {
        if (!mobileBridgeAar.exists()) {
            throw GradleException(
                "Missing gomobile AAR at ${mobileBridgeAar.path}. Run ./scripts/android/build-aar.sh from the repo root first."
            )
        }
    }
}
