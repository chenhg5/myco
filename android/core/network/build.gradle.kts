plugins {
    kotlin("jvm")
    kotlin("plugin.serialization")
}

kotlin { jvmToolchain(17) }

dependencies {
    api(project(":core:model"))
    implementation(libs.okhttp)
    implementation(libs.kotlinx.coroutines.core)
    testImplementation(libs.okhttp.mockwebserver)
    testImplementation(libs.kotlinx.coroutines.test)
    testImplementation(libs.junit)
}

tasks.withType<Test>().configureEach {
    if (providers.environmentVariable("MYCO_ANDROID_FIXTURE").isPresent) {
        outputs.upToDateWhen { false }
    }
}
