package io.sn.marche.polaris.wanda;

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
import java.util.Set;

public final class Main {

    private static final String MODULE = "polaris-wanda";
    private static final int COMPILED_FOR = 11;

    private Main() {}

    public static void main(String[] args) throws IOException {
        var config = loadConfig();
        var port = ((Number) config.getOrDefault("server.port", 8091)).intValue();

        var server = HttpServer.create(new InetSocketAddress(port), 0);
        server.createContext("/health", exchange -> {
            // Java 11 features: var, Set.of (Java 9), String.strip (Java 11), Files.readString (Java 11) — demonstrate strip.
            var raw = "  wanda  ";
            var stripped = raw.strip();
            Set<String> caps = Set.of("var", "String.strip", "Set.of", "Files.readString");

            var body = String.join("\n",
                    "module: " + MODULE,
                    "language: Java",
                    "compiledFor: " + COMPILED_FOR,
                    "runtimeJavaVersion: " + System.getProperty("java.version"),
                    "env: " + config.getOrDefault("marche.env", "dev"),
                    "stripDemo: '" + stripped + "' (was '" + raw + "')",
                    "features: " + String.join(", ", caps)
            );
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
            return Map.of("server.port", 8091, "marche.env", "dev");
        }
        try (InputStream in = Files.newInputStream(file)) {
            Map<String, Object> raw = new Yaml().load(in);
            return flatten(raw == null ? Map.of() : raw, "");
        } catch (IOException e) {
            System.err.println("Could not read " + file + ": " + e.getMessage());
            return Map.of("server.port", 8091, "marche.env", "dev");
        }
    }

    @SuppressWarnings("unchecked")
    private static Map<String, Object> flatten(Map<String, Object> in, String prefix) {
        var out = new LinkedHashMap<String, Object>();
        in.forEach((k, v) -> {
            var key = prefix.isBlank() ? k : prefix + "." + k;
            if (v instanceof Map) {
                out.putAll(flatten((Map<String, Object>) v, key));
            } else {
                out.put(key, v);
            }
        });
        return out;
    }
}
