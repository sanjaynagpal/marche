package io.sn.marche.kepler.eleven

import io.ktor.http.ContentType
import io.ktor.server.application.call
import io.ktor.server.engine.embeddedServer
import io.ktor.server.netty.Netty
import io.ktor.server.response.respondText
import io.ktor.server.routing.get
import io.ktor.server.routing.routing
import org.yaml.snakeyaml.Yaml
import java.nio.file.Files
import java.nio.file.Paths

private const val MODULE = "kepler-eleven"
private const val COMPILED_FOR = 11

fun main() {
    val config = loadConfig()
    val port = (config["server.port"] as? Number)?.toInt() ?: 8084

    embeddedServer(Netty, port = port) {
        routing {
            get("/health") {
                // Kotlin features: data classes, scope functions, sealed classes, coroutines (via Ktor).
                val features = listOf(
                    "data classes",
                    "sealed classes",
                    "scope functions (apply/let/run)",
                    "coroutines via Ktor",
                    "extension functions"
                )
                val body = buildString {
                    appendLine("module: $MODULE")
                    appendLine("language: Kotlin ${KotlinVersion.CURRENT}")
                    appendLine("compiledFor: $COMPILED_FOR")
                    appendLine("runtimeJavaVersion: ${System.getProperty("java.version")}")
                    appendLine("env: ${config["marche.env"] ?: "dev"}")
                    appendLine("ktor: 2.x")
                    appendLine("features:")
                    features.forEach { appendLine("  - $it") }
                }
                call.respondText(body, ContentType.Text.Plain)
            }
        }
    }.start(wait = true)
    println("$MODULE listening on $port")
}

private fun loadConfig(): Map<String, Any> {
    val env = System.getProperty("marche.env") ?: System.getenv("MARCHE_ENV") ?: "dev"
    val dir = System.getProperty("marche.config.dir") ?: System.getenv("MARCHE_CONFIG_DIR") ?: "configuration"
    val file = Paths.get(dir, "application-$env.yaml")
    if (!Files.exists(file)) return mapOf("server.port" to 8084, "marche.env" to "dev")
    return Files.newInputStream(file).use { input ->
        val raw = Yaml().load<Map<String, Any>?>(input) ?: emptyMap()
        flatten(raw, "")
    }
}

@Suppress("UNCHECKED_CAST")
private fun flatten(input: Map<String, Any>, prefix: String): Map<String, Any> {
    val out = LinkedHashMap<String, Any>()
    for ((k, v) in input) {
        val key = if (prefix.isEmpty()) k else "$prefix.$k"
        if (v is Map<*, *>) {
            out.putAll(flatten(v as Map<String, Any>, key))
        } else {
            out[key] = v
        }
    }
    return out
}
