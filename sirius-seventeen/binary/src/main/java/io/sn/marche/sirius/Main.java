package io.sn.marche.sirius;

import com.sun.net.httpserver.HttpServer;
import org.yaml.snakeyaml.Yaml;

import java.io.IOException;
import java.io.InputStream;
import java.io.OutputStream;
import java.net.InetSocketAddress;
import java.nio.charset.StandardCharsets;
import java.nio.file.Files;
import java.nio.file.Path;
import java.util.LinkedHashMap;
import java.util.Map;

public final class Main {

    private static final String MODULE = "sirius-seventeen";
    private static final int COMPILED_FOR = 17;

    // Java 17 features: sealed interface + records.
    sealed interface Health permits Up, Down {}
    record Up(String detail) implements Health {}
    record Down(String reason) implements Health {}

    private Main() {}

    public static void main(String[] args) throws IOException {
        var config = loadConfig();
        var port = ((Number) config.getOrDefault("server.port", 8082)).intValue();

        var server = HttpServer.create(new InetSocketAddress(port), 0);
        server.createContext("/health", exchange -> {
            Health status = new Up("ready");

            // Java 17: pattern matching for instanceof.
            String detail;
            if (status instanceof Up up) {
                detail = "UP: " + up.detail();
            } else if (status instanceof Down down) {
                detail = "DOWN: " + down.reason();
            } else {
                detail = "UNKNOWN";
            }

            // Java 15+ text block.
            var body = """
                    module: %s
                    language: Java
                    compiledFor: %d
                    runtimeJavaVersion: %s
                    env: %s
                    status: %s
                    features: records, sealed interfaces, pattern matching for instanceof, text blocks
                    """.formatted(
                    MODULE,
                    COMPILED_FOR,
                    System.getProperty("java.version"),
                    config.getOrDefault("marche.env", "dev"),
                    detail);

            var bytes = body.getBytes(StandardCharsets.UTF_8);
            exchange.getResponseHeaders().set("Content-Type", "text/plain; charset=utf-8");
            exchange.sendResponseHeaders(200, bytes.length);
            try (OutputStream os = exchange.getResponseBody()) {
                os.write(bytes);
            }
        });
        server.setExecutor(null);
        server.start();
        System.out.println(MODULE + " listening on " + port);
    }

    private static Map<String, Object> loadConfig() {
        var env = System.getProperty("marche.env",
                System.getenv().getOrDefault("MARCHE_ENV", "dev"));
        var dir = System.getProperty("marche.config.dir",
                System.getenv().getOrDefault("MARCHE_CONFIG_DIR", "configuration"));
        var file = Path.of(dir, "application-" + env + ".yaml");
        if (!Files.exists(file)) {
            return Map.of("server.port", 8082, "marche.env", "dev");
        }
        try (InputStream in = Files.newInputStream(file)) {
            Map<String, Object> raw = new Yaml().load(in);
            return flatten(raw == null ? Map.of() : raw, "");
        } catch (IOException e) {
            System.err.println("Could not read " + file + ": " + e.getMessage());
            return Map.of("server.port", 8082, "marche.env", "dev");
        }
    }

    @SuppressWarnings("unchecked")
    private static Map<String, Object> flatten(Map<String, Object> in, String prefix) {
        var out = new LinkedHashMap<String, Object>();
        in.forEach((k, v) -> {
            var key = prefix.isBlank() ? k : prefix + "." + k;
            if (v instanceof Map<?, ?> m) {
                out.putAll(flatten((Map<String, Object>) m, key));
            } else {
                out.put(key, v);
            }
        });
        return out;
    }
}
