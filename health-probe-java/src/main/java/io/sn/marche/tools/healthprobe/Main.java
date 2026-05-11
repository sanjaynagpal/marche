package io.sn.marche.tools.healthprobe;

import java.net.URI;
import java.net.http.HttpClient;
import java.net.http.HttpRequest;
import java.net.http.HttpResponse;
import java.time.Duration;
import java.util.List;

public final class Main {

    private static final String USAGE = """
            health-probe-java — Marche service health checker (Java 17)

            Usage:
              health-probe-java <url> [url ...]

            Each URL is queried with a 5-second timeout. The response body is printed for
            every endpoint. Exits 0 only when all endpoints return HTTP 200.

            Examples:
              health-probe-java http://localhost:8080/health
              health-probe-java http://localhost:8080/health http://localhost:8081/health
            """;

    record ProbeResult(String url, int status, String body, String error) {

        static ProbeResult ok(String url, int status, String body) {
            return new ProbeResult(url, status, body, null);
        }

        static ProbeResult failed(String url, String error) {
            return new ProbeResult(url, -1, null, error);
        }

        boolean isOk() {
            return error == null && status == 200;
        }
    }

    public static void main(String[] args) {
        if (args.length == 0) {
            System.err.print(USAGE);
            System.exit(1);
        }

        var client = HttpClient.newBuilder()
                .connectTimeout(Duration.ofSeconds(5))
                .build();

        var results = List.of(args).stream()
                .map(url -> probe(client, url))
                .toList();

        results.forEach(Main::print);

        if (results.stream().anyMatch(r -> !r.isOk())) {
            System.exit(1);
        }
    }

    private static ProbeResult probe(HttpClient client, String url) {
        var request = HttpRequest.newBuilder()
                .uri(URI.create(url))
                .timeout(Duration.ofSeconds(5))
                .GET()
                .build();
        try {
            var response = client.send(request, HttpResponse.BodyHandlers.ofString());
            return ProbeResult.ok(url, response.statusCode(), response.body());
        } catch (Exception e) {
            return ProbeResult.failed(url, e.getMessage());
        }
    }

    private static void print(ProbeResult result) {
        if (result.isOk()) {
            System.out.printf("OK    %s%n%s%n", result.url(), result.body());
        } else if (result.error() != null) {
            System.err.printf("FAIL  %s%n      %s%n%n", result.url(), result.error());
        } else {
            System.err.printf("FAIL  %s  (HTTP %d)%n%s%n%n", result.url(), result.status(), result.body());
        }
    }
}
