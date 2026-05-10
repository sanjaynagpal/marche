package io.sn.marche.http;

import com.sun.net.httpserver.HttpHandler;
import com.sun.net.httpserver.HttpServer;

import java.io.IOException;
import java.net.InetSocketAddress;
import java.util.Map;

public final class MarcheServer {

    private final HttpServer server;
    private final String moduleName;
    private final int port;

    private MarcheServer(HttpServer server, String moduleName, int port) {
        this.server = server;
        this.moduleName = moduleName;
        this.port = port;
    }

    public static MarcheServer create(String moduleName, Map<String, Object> config, int defaultPort)
            throws IOException {
        int port = ((Number) config.getOrDefault("server.port", defaultPort)).intValue();
        HttpServer server = HttpServer.create(new InetSocketAddress(port), 0);
        server.setExecutor(null);
        return new MarcheServer(server, moduleName, port);
    }

    public MarcheServer context(String path, HttpHandler handler) {
        server.createContext(path, handler);
        return this;
    }

    public void start() {
        server.start();
        System.out.println(moduleName + " listening on " + port);
    }
}
