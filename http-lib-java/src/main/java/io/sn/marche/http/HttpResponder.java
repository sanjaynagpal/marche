package io.sn.marche.http;

import com.sun.net.httpserver.HttpExchange;

import java.io.IOException;
import java.io.OutputStream;
import java.nio.charset.Charset;

public final class HttpResponder {

    private static final Charset UTF8 = Charset.forName("UTF-8");

    private HttpResponder() {}

    public static void sendText(HttpExchange exchange, int status, String body) throws IOException {
        byte[] bytes = body.getBytes(UTF8);
        exchange.getResponseHeaders().set("Content-Type", "text/plain; charset=utf-8");
        exchange.sendResponseHeaders(status, bytes.length);
        OutputStream os = exchange.getResponseBody();
        try {
            os.write(bytes);
        } finally {
            os.close();
        }
    }

    public static void ok(HttpExchange exchange, String body) throws IOException {
        sendText(exchange, 200, body);
    }

    public static void notFound(HttpExchange exchange) throws IOException {
        sendText(exchange, 404, "Not Found\n");
    }

    public static void methodNotAllowed(HttpExchange exchange) throws IOException {
        sendText(exchange, 405, "Method Not Allowed\n");
    }
}
