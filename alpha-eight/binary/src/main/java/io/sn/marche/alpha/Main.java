package io.sn.marche.alpha;

import com.sun.net.httpserver.HttpExchange;
import com.sun.net.httpserver.HttpHandler;
import com.sun.net.httpserver.HttpServer;
import org.yaml.snakeyaml.Yaml;

import java.io.IOException;
import java.io.InputStream;
import java.io.OutputStream;
import java.net.InetSocketAddress;
import java.nio.charset.Charset;
import java.nio.file.Files;
import java.nio.file.Path;
import java.nio.file.Paths;
import java.util.Arrays;
import java.util.Collections;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;
import java.util.stream.Collectors;

public final class Main {

    private static final Charset UTF8 = Charset.forName("UTF-8");
    private static final String MODULE = "alpha-eight";
    private static final int COMPILED_FOR = 8;

    private Main() {}

    public static void main(String[] args) throws IOException {
        Map<String, Object> config = loadConfig();
        int port = ((Number) config.getOrDefault("server.port", 8080)).intValue();

        HttpServer server = HttpServer.create(new InetSocketAddress(port), 0);
        server.createContext("/health", new HealthHandler(config));
        server.setExecutor(null);
        server.start();
        System.out.println(MODULE + " listening on " + port);
    }

    private static Map<String, Object> loadConfig() {
        String env = System.getProperty("marche.env", System.getenv().getOrDefault("MARCHE_ENV", "dev"));
        String dir = System.getProperty("marche.config.dir",
                System.getenv().getOrDefault("MARCHE_CONFIG_DIR", "configuration"));
        Path file = Paths.get(dir, "application-" + env + ".yaml");
        if (!Files.exists(file)) {
            return defaults();
        }
        try (InputStream in = Files.newInputStream(file)) {
            Map<String, Object> raw = new Yaml().load(in);
            return flatten(raw == null ? Collections.<String, Object>emptyMap() : raw, "");
        } catch (IOException e) {
            System.err.println("Could not read " + file + ": " + e.getMessage());
            return defaults();
        }
    }

    private static Map<String, Object> defaults() {
        Map<String, Object> d = new LinkedHashMap<String, Object>();
        d.put("server.port", 8080);
        d.put("marche.env", "dev");
        return d;
    }

    @SuppressWarnings("unchecked")
    private static Map<String, Object> flatten(Map<String, Object> in, String prefix) {
        Map<String, Object> out = new LinkedHashMap<String, Object>();
        for (Map.Entry<String, Object> e : in.entrySet()) {
            String key = prefix.isEmpty() ? e.getKey() : prefix + "." + e.getKey();
            Object value = e.getValue();
            if (value instanceof Map) {
                out.putAll(flatten((Map<String, Object>) value, key));
            } else {
                out.put(key, value);
            }
        }
        return out;
    }

    static final class HealthHandler implements HttpHandler {
        private final Map<String, Object> config;

        HealthHandler(Map<String, Object> config) {
            this.config = config;
        }

        public void handle(HttpExchange exchange) throws IOException {
            // Java 8 features in use: lambdas, Stream API, Collectors.joining, default methods.
            List<String> features = Arrays.asList(
                    "lambdas",
                    "stream-api",
                    "default-methods",
                    "Optional",
                    "java.time"
            );
            String featureBlock = features.stream()
                    .map(new java.util.function.Function<String, String>() {
                        public String apply(String s) { return "  - " + s; }
                    })
                    .collect(Collectors.joining("\n"));

            String body = "module: " + MODULE + "\n"
                    + "language: Java\n"
                    + "compiledFor: " + COMPILED_FOR + "\n"
                    + "runtimeJavaVersion: " + System.getProperty("java.version") + "\n"
                    + "env: " + config.getOrDefault("marche.env", "dev") + "\n"
                    + "features:\n" + featureBlock + "\n";

            byte[] bytes = body.getBytes(UTF8);
            exchange.getResponseHeaders().set("Content-Type", "text/plain; charset=utf-8");
            exchange.sendResponseHeaders(200, bytes.length);
            OutputStream os = exchange.getResponseBody();
            try {
                os.write(bytes);
            } finally {
                os.close();
            }
        }
    }
}
