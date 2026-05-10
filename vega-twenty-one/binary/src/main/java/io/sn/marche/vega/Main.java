package io.sn.marche.vega;

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
import java.util.concurrent.Executors;

public final class Main {

    private static final String MODULE = "vega-twenty-one";
    private static final int COMPILED_FOR = 21;

    // Java 17+ sealed types with record patterns demonstrated in switch (Java 21).
    sealed interface Health permits Up, Down {}
    record Up(int latencyMs) implements Health {}
    record Down(String reason) implements Health {}

    private Main() {}

    public static void main(String[] args) throws IOException {
        var config = loadConfig();
        var port = ((Number) config.getOrDefault("server.port", 8083)).intValue();

        var server = HttpServer.create(new InetSocketAddress(port), 0);
        // Java 21 feature: virtual threads.
        server.setExecutor(Executors.newVirtualThreadPerTaskExecutor());
        server.createContext("/health", exchange -> {
            Health h = new Up(7);

            // Java 21: pattern switch with record patterns + exhaustiveness over sealed type.
            String detail = switch (h) {
                case Up(int ms) -> "UP " + ms + "ms";
                case Down(String reason) -> "DOWN: " + reason;
            };

            var body = """
                    module: %s
                    language: Java
                    compiledFor: %d
                    runtimeJavaVersion: %s
                    env: %s
                    status: %s
                    thread: %s
                    isVirtual: %s
                    features: virtual threads, pattern switch, record patterns, sequenced collections
                    """.formatted(
                    MODULE,
                    COMPILED_FOR,
                    System.getProperty("java.version"),
                    config.getOrDefault("marche.env", "dev"),
                    detail,
                    Thread.currentThread(),
                    Thread.currentThread().isVirtual());

            var bytes = body.getBytes(StandardCharsets.UTF_8);
            exchange.getResponseHeaders().set("Content-Type", "text/plain; charset=utf-8");
            exchange.sendResponseHeaders(200, bytes.length);
            try (OutputStream os = exchange.getResponseBody()) {
                os.write(bytes);
            }
        });
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
            return Map.of("server.port", 8083, "marche.env", "dev");
        }
        try (InputStream in = Files.newInputStream(file)) {
            Map<String, Object> raw = new Yaml().load(in);
            return flatten(raw == null ? Map.of() : raw, "");
        } catch (IOException e) {
            System.err.println("Could not read " + file + ": " + e.getMessage());
            return Map.of("server.port", 8083, "marche.env", "dev");
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
