package io.sn.marche.http;

import org.yaml.snakeyaml.Yaml;

import java.io.IOException;
import java.io.InputStream;
import java.nio.file.Files;
import java.nio.file.Path;
import java.nio.file.Paths;
import java.util.Collections;
import java.util.LinkedHashMap;
import java.util.Map;

public final class ConfigLoader {

    private ConfigLoader() {}

    /**
     * Loads application-{env}.yaml from the config directory and returns it as
     * a flat dot-notation map merged on top of the supplied defaults. Falls back
     * to the defaults when the file is absent or unreadable.
     */
    public static Map<String, Object> load(Map<String, Object> defaults) {
        String env = System.getProperty("marche.env",
                System.getenv().getOrDefault("MARCHE_ENV", "dev"));
        String dir = System.getProperty("marche.config.dir",
                System.getenv().getOrDefault("MARCHE_CONFIG_DIR", "configuration"));
        Path file = Paths.get(dir, "application-" + env + ".yaml");
        if (!Files.exists(file)) {
            return defaults;
        }
        try (InputStream in = Files.newInputStream(file)) {
            Map<String, Object> raw = new Yaml().load(in);
            return flatten(raw == null ? Collections.<String, Object>emptyMap() : raw, "");
        } catch (IOException e) {
            System.err.println("Could not read " + file + ": " + e.getMessage());
            return defaults;
        }
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
}
