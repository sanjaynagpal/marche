import java.io.*;
import java.nio.file.*;
import java.time.*;
import java.time.format.*;
import java.util.*;
import java.util.stream.*;
import java.util.zip.*;

/**
 * Single-file Java program invoked by exec-maven-plugin in the verify phase.
 * Opens the binary distribution tar.gz and checks that every runtime JAR listed
 * in target/lib/checksums.txt is present inside the archive.  Exits non-zero to
 * fail the build if any expected artifact is absent.
 *
 * Args: <projectBasedir> <targetDir> <artifactId> <version>
 */
public class AssemblyVerifier {

    public static void main(String[] args) throws Exception {
        Path basedir    = Path.of(args[0]);
        Path targetDir  = Path.of(args[1]);
        String artifact = args[2];
        String version  = args[3];

        Path buildRecordsDir = basedir.resolve("build-records");
        Files.createDirectories(buildRecordsDir);

        Path checksums   = targetDir.resolve("lib").resolve("checksums.txt");
        Path archivePath = targetDir.resolve(artifact + "-" + version + "-bin.tar.gz");
        Path reportPath  = buildRecordsDir.resolve("assembly-verification.txt");

        String ts = LocalDateTime.now().format(DateTimeFormatter.ofPattern("yyyy-MM-dd'T'HH:mm:ss"));
        StringBuilder sb = new StringBuilder();
        sb.append("=== Assembly Verification Report ===\n");
        sb.append("Generated : ").append(ts).append("\n");
        sb.append("Module    : ").append(artifact).append(":").append(version).append("\n");
        sb.append("Archive   : ").append(archivePath.getFileName()).append("\n\n");

        Set<String> expected = parseExpectedJars(checksums);

        if (expected.isEmpty()) {
            sb.append("SKIPPED: checksums.txt not found or empty — no artifacts to verify.\n");
            Files.writeString(reportPath, sb.toString());
            System.out.println("[INFO] AssemblyVerifier: SKIPPED — checksums.txt absent for "
                + artifact + " (POM module or first build before staging).");
            return;
        }

        if (!Files.exists(archivePath)) {
            sb.append("ERROR: binary archive not found: ").append(archivePath).append("\n");
            Files.writeString(reportPath, sb.toString());
            System.err.println("[ERROR] AssemblyVerifier: archive not found: " + archivePath);
            System.exit(1);
        }

        Set<String> present = readTarGzJars(archivePath);

        List<String> missing = expected.stream()
            .filter(j -> !present.contains(j))
            .sorted()
            .collect(Collectors.toList());

        long matched = expected.stream().filter(present::contains).count();
        sb.append("Expected : ").append(expected.size()).append(" artifact(s)\n");
        sb.append("Present  : ").append(matched).append(" artifact(s)\n");
        sb.append("Missing  : ").append(missing.size()).append(" artifact(s)\n\n");

        if (missing.isEmpty()) {
            sb.append("RESULT: PASS — all runtime artifacts verified in distribution.\n");
            Files.writeString(reportPath, sb.toString());
            System.out.println("[INFO] AssemblyVerifier: PASS — all " + expected.size()
                + " artifact(s) present in " + archivePath.getFileName());
        } else {
            sb.append("RESULT: FAIL — missing artifact(s):\n");
            for (String m : missing) sb.append("  MISSING: ").append(m).append("\n");
            Files.writeString(reportPath, sb.toString());
            System.err.println("[ERROR] AssemblyVerifier: FAIL — " + missing.size()
                + " artifact(s) missing from " + archivePath.getFileName() + ":");
            for (String m : missing) System.err.println("  MISSING: " + m);
            System.exit(1);
        }
    }

    /** Parses checksums.txt and returns the set of bare JAR filenames expected in the archive. */
    static Set<String> parseExpectedJars(Path checksums) throws Exception {
        Set<String> result = new TreeSet<>();
        if (!Files.exists(checksums)) return result;
        for (String line : Files.readAllLines(checksums)) {
            String t = line.trim();
            if (t.isEmpty()) continue;
            String[] parts = t.split("\\s+", 2);
            if (parts.length == 2) {
                String name = parts[1].trim().replaceAll("^\\*", "");
                result.add(Path.of(name).getFileName().toString());
            }
        }
        return result;
    }

    /** Returns the set of bare .jar filenames found anywhere inside the tar.gz archive. */
    static Set<String> readTarGzJars(Path archive) throws Exception {
        Set<String> names = new TreeSet<>();
        byte[] header = new byte[512];
        String pendingLongName = null;

        try (InputStream fis = Files.newInputStream(archive);
             GZIPInputStream gis = new GZIPInputStream(fis)) {

            while (true) {
                if (readFully(gis, header, 512) < 512) break;
                if (isZeroBlock(header)) break;

                char typeflag  = (char) (header[156] & 0xFF);
                long size      = parseOctal(header, 124, 12);
                long padded    = size == 0 ? 0 : ((size + 511) / 512) * 512;

                if (typeflag == 'L') {
                    // GNU long-name extension: actual name stored as entry data
                    byte[] nameData = new byte[(int) size];
                    readFully(gis, nameData, (int) size);
                    skipFully(gis, padded - size);
                    int end = nameData.length;
                    while (end > 0 && nameData[end - 1] == 0) end--;
                    pendingLongName = new String(nameData, 0, end);
                    continue;
                }

                String entryName;
                if (pendingLongName != null) {
                    entryName = pendingLongName;
                    pendingLongName = null;
                } else {
                    String prefix = readNullTerminated(header, 345, 155);
                    String name   = readNullTerminated(header, 0, 100);
                    entryName = prefix.isEmpty() ? name : prefix + "/" + name;
                }

                if ((typeflag == '0' || typeflag == '\0') && entryName.endsWith(".jar")) {
                    int slash = entryName.lastIndexOf('/');
                    String basename = slash >= 0 ? entryName.substring(slash + 1) : entryName;
                    if (!basename.isEmpty()) names.add(basename);
                }

                skipFully(gis, padded);
            }
        }
        return names;
    }

    static int readFully(InputStream in, byte[] buf, int len) throws IOException {
        int total = 0;
        while (total < len) {
            int n = in.read(buf, total, len - total);
            if (n < 0) break;
            total += n;
        }
        return total;
    }

    /** Skips exactly n bytes by reading into a discard buffer (skip() may under-skip). */
    static void skipFully(InputStream in, long n) throws IOException {
        byte[] discard = new byte[4096];
        while (n > 0) {
            int got = in.read(discard, 0, (int) Math.min(n, discard.length));
            if (got < 0) break;
            n -= got;
        }
    }

    static boolean isZeroBlock(byte[] b) {
        for (byte v : b) if (v != 0) return false;
        return true;
    }

    static String readNullTerminated(byte[] b, int off, int len) {
        int end = off;
        while (end < off + len && b[end] != 0) end++;
        return new String(b, off, end - off).trim();
    }

    static long parseOctal(byte[] b, int off, int len) {
        // GNU binary encoding for sizes > 8 GB: high bit of first byte is set
        if ((b[off] & 0x80) != 0) {
            long val = 0;
            for (int i = off + 1; i < off + len; i++) val = (val << 8) | (b[i] & 0xFF);
            return val;
        }
        String s = readNullTerminated(b, off, len).trim();
        return s.isEmpty() ? 0 : Long.parseLong(s, 8);
    }
}
