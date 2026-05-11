import java.io.*;
import java.nio.file.*;
import java.time.*;
import java.time.format.*;
import java.util.*;
import java.util.stream.*;
import java.util.zip.*;

/**
 * Single-file Java program invoked by exec-maven-plugin in the verify phase.
 * Reads every JAR in target/lib/, extracts the bytecode major version from the
 * first class file in each JAR, and writes build-records/bytecode-versions.txt.
 * Exits non-zero if any JAR's bytecode version exceeds the module's declared target.
 * Skips gracefully when target/lib/ is absent (Go modules, POM-only modules).
 *
 * Args: <projectBasedir> <targetDir> <artifactId> <version> <declaredJavaTarget>
 */
public class BytecodeReport {

    public static void main(String[] args) throws Exception {
        Path basedir      = Path.of(args[0]);
        Path targetDir    = Path.of(args[1]);
        String artifact   = args[2];
        String version    = args[3];
        String declared   = args.length > 4 ? args[4] : "";

        Path buildRecordsDir = basedir.resolve("build-records");
        Files.createDirectories(buildRecordsDir);

        Path libDir    = targetDir.resolve("lib");
        Path reportPath = buildRecordsDir.resolve("bytecode-versions.txt");

        int declaredMajor = toMajorVersion(declared);
        String ts = LocalDateTime.now().format(DateTimeFormatter.ofPattern("yyyy-MM-dd'T'HH:mm:ss"));

        StringBuilder sb = new StringBuilder();
        sb.append("=== Bytecode Version Report ===\n");
        sb.append("Generated       : ").append(ts).append("\n");
        sb.append("Module          : ").append(artifact).append(":").append(version).append("\n");
        if (declaredMajor > 0) {
            sb.append("Declared target : Java ").append(declared)
              .append(" (class file major version ").append(declaredMajor).append(")\n\n");
        } else {
            sb.append("Declared target : (not set)\n\n");
        }

        if (!Files.isDirectory(libDir)) {
            sb.append("SKIPPED: target/lib/ not found — no JAR artifacts for this module.\n");
            Files.writeString(reportPath, sb.toString());
            System.out.println("[INFO] BytecodeReport: SKIPPED — no target/lib/ for " + artifact);
            return;
        }

        List<Path> jars = Files.list(libDir)
            .filter(p -> p.getFileName().toString().endsWith(".jar"))
            .sorted(Comparator.comparing(p -> p.getFileName().toString()))
            .collect(Collectors.toList());

        if (jars.isEmpty()) {
            sb.append("SKIPPED: no JAR files found in target/lib/.\n");
            Files.writeString(reportPath, sb.toString());
            System.out.println("[INFO] BytecodeReport: SKIPPED — no JARs in target/lib/ for " + artifact);
            return;
        }

        List<String> violations = new ArrayList<>();

        sb.append(String.format("%-55s  %6s  %-18s  %s\n", "JAR", "Major", "Java Version", "Status"));
        sb.append("-".repeat(100)).append("\n");

        for (Path jar : jars) {
            String name  = jar.getFileName().toString();
            int major    = readMajorVersion(jar);
            String label = majorToJavaName(major);
            String status;
            if (major < 0) {
                status = "UNREADABLE (no .class found)";
            } else if (declaredMajor > 0 && major > declaredMajor) {
                status = "VIOLATION: major " + major + " > declared " + declaredMajor;
                violations.add(name + " (major=" + major + ", " + label + " > declared Java " + declared + ")");
            } else {
                status = "OK";
            }
            sb.append(String.format("%-55s  %6s  %-18s  %s\n",
                name, major < 0 ? "?" : major, label, status));
        }

        sb.append("\n");
        if (violations.isEmpty()) {
            sb.append("RESULT: PASS — all ").append(jars.size())
              .append(" JAR(s) are within the declared bytecode target");
            if (declaredMajor > 0) sb.append(" (Java ").append(declared).append(")");
            sb.append(".\n");
            Files.writeString(reportPath, sb.toString());
            System.out.println("[INFO] BytecodeReport: PASS — " + jars.size()
                + " JAR(s) verified for " + artifact);
        } else {
            sb.append("RESULT: FAIL — ").append(violations.size())
              .append(" JAR(s) exceed the declared bytecode target:\n");
            for (String v : violations) sb.append("  VIOLATION: ").append(v).append("\n");
            Files.writeString(reportPath, sb.toString());
            System.err.println("[ERROR] BytecodeReport: FAIL — bytecode violations in " + artifact + ":");
            for (String v : violations) System.err.println("  VIOLATION: " + v);
            System.exit(1);
        }
    }

    /**
     * Opens the JAR as a ZIP, finds the first .class entry, reads bytes 6-7
     * (big-endian major version in the class file header), and returns it.
     * Returns -1 if no class entry is found or the file cannot be read.
     *
     * Class file header layout (each field is 2 bytes, big-endian):
     *   0xCAFEBABE  magic
     *   minor_version
     *   major_version  ← bytes [6..7]
     */
    static int readMajorVersion(Path jar) {
        try (ZipFile zf = new ZipFile(jar.toFile())) {
            return zf.stream()
                .filter(e -> !e.isDirectory() && e.getName().endsWith(".class"))
                .findFirst()
                .map(entry -> {
                    try (InputStream is = zf.getInputStream(entry)) {
                        byte[] header = new byte[8];
                        int read = 0;
                        while (read < 8) {
                            int n = is.read(header, read, 8 - read);
                            if (n < 0) return -1;
                            read += n;
                        }
                        return ((header[6] & 0xFF) << 8) | (header[7] & 0xFF);
                    } catch (IOException e) {
                        return -1;
                    }
                })
                .orElse(-1);
        } catch (Exception e) {
            return -1;
        }
    }

    /** Converts a Java version string ("8", "11", "17", "21") to a class file major version. */
    static int toMajorVersion(String javaVersion) {
        try {
            int v = Integer.parseInt(javaVersion.trim());
            // Class file major version = 44 + java version number (e.g. Java 8 → 52, Java 21 → 65)
            return 44 + v;
        } catch (NumberFormatException e) {
            return -1;
        }
    }

    /** Maps a class file major version to a human-readable Java version string. */
    static String majorToJavaName(int major) {
        if (major < 45) return major < 0 ? "unknown" : "pre-Java-1";
        if (major <= 48) return "Java 1." + (major - 44);  // 1.1 – 1.4
        return "Java " + (major - 44);                      // Java 5 and above
    }
}
