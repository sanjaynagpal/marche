package io.sn.marche.alpha;

import io.sn.marche.http.ConfigLoader;
import io.sn.marche.http.HttpResponder;
import io.sn.marche.http.MarcheServer;

import com.sun.net.httpserver.HttpExchange;
import com.sun.net.httpserver.HttpHandler;

import java.io.IOException;
import java.util.Arrays;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;
import java.util.stream.Collectors;

public final class Main {

    private static final String MODULE = "alpha-eight";
    private static final int COMPILED_FOR = 8;
    private static final int DEFAULT_PORT = 8080;

    private Main() {}

    public static void main(String[] args) throws IOException {
        Map<String, Object> config = ConfigLoader.load(defaults());
        MarcheServer.create(MODULE, config, DEFAULT_PORT)
                .context("/health", new HealthHandler(config))
                .start();
    }

    private static Map<String, Object> defaults() {
        Map<String, Object> d = new LinkedHashMap<String, Object>();
        d.put("server.port", DEFAULT_PORT);
        d.put("marche.env", "dev");
        return d;
    }

    static final class HealthHandler implements HttpHandler {
        private final Map<String, Object> config;

        HealthHandler(Map<String, Object> config) {
            this.config = config;
        }

        public void handle(HttpExchange exchange) throws IOException {
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

            HttpResponder.ok(exchange, body);
        }
    }
}
