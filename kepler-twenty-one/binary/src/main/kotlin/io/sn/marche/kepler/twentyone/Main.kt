package io.sn.marche.kepler.twentyone

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
import java.util.concurrent.Executors

private const val MODULE = "kepler-twenty-one"
private const val COMPILED_FOR = 21

// Kotlin sealed interface mirrors a Java 17+ sealed interface — exhaustive when over.
private sealed interface Health {
    data class Up(val latencyMs: Int) : Health
    data class Down(val reason: String) : Health
}

fun main() {
    val config = loadConfig()
    val port = (config["server.port"] as? Number)?.toInt() ?: 8085

    // Java 21 feature surface: virtual threads. Probe a virtual thread to demonstrate.
    val vtName = probeVirtualThread()

    embeddedServer(Netty, port = port) {
        routing {
            get("/health") {
                val status: Health = Health.Up(latencyMs = 7)
                // Kotlin: exhaustive when over a sealed type — analogous to Java 21 pattern switch.
                val detail = when (status) {
                    is Health.Up -> "UP ${status.latencyMs}ms"
                    is Health.Down -> "DOWN: ${status.reason}"
                }
                val body = """
                    module: $MODULE
                    language: Kotlin ${KotlinVersion.CURRENT}
                    compiledFor: $COMPILED_FOR
                    runtimeJavaVersion: ${System.getProperty("java.version")}
                    env: ${config["marche.env"] ?: "dev"}
                    ktor: 3.x
                    status: $detail
                    virtualThreadProbe: $vtName
                    features: virtual threads (JDK 21), sealed interfaces, exhaustive when, coroutines
                """.trimIndent() + "\n"
                call.respondText(body, ContentType.Text.Plain)
            }
        }
    }.start(wait = true)
    println("$MODULE listening on $port")
}

private fun probeVirtualThread(): String {
    val vt = Executors.newVirtualThreadPerTaskExecutor()
    return try {
        vt.submit<String> {
            val t = Thread.currentThread()
            "${t.name} isVirtual=${t.isVirtual}"
        }.get()
    } finally {
        vt.shutdown()
    }
}

private fun loadConfig(): Map<String, Any> {
    val env = System.getProperty("marche.env") ?: System.getenv("MARCHE_ENV") ?: "dev"
    val dir = System.getProperty("marche.config.dir") ?: System.getenv("MARCHE_CONFIG_DIR") ?: "configuration"
    val file = Paths.get(dir, "application-$env.yaml")
    if (!Files.exists(file)) return mapOf("server.port" to 8085, "marche.env" to "dev")
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
