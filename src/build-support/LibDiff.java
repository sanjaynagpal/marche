import java.io.*;
import java.nio.file.*;
import java.time.*;
import java.time.format.*;
import java.util.*;
import java.util.stream.*;

/**
 * Single-file Java program invoked by exec-maven-plugin to generate lib-changes.txt
 * and update build-records/ snapshots for audit-trail tracking.
 *
 * Args: <projectBasedir> <targetDir> <artifactId> <version>
 */
public class LibDiff {

    public static void main(String[] args) throws Exception {
        Path basedir   = Path.of(args[0]);
        Path targetDir = Path.of(args[1]);
        String artifact = args[2];
        String version  = args[3];

        Path buildRecordsDir = basedir.resolve("build-records");
        Files.createDirectories(buildRecordsDir);

        Path libDir        = targetDir.resolve("lib");
        Path newChecksums  = libDir.resolve("checksums.txt");
        Path newTree       = libDir.resolve("dependency-tree.txt");
        Path prevChecksums = buildRecordsDir.resolve("checksums.txt");
        Path prevTree      = buildRecordsDir.resolve("dependency-tree.txt");
        Path changesFile   = libDir.resolve("lib-changes.txt");

        Map<String, String> prev = parseChecksums(prevChecksums);
        Map<String, String> curr = parseChecksums(newChecksums);

        Set<String> allNames = new TreeSet<>();
        allNames.addAll(prev.keySet());
        allNames.addAll(curr.keySet());

        List<String[]> added = new ArrayList<>(), removed = new ArrayList<>(), changed = new ArrayList<>();
        long unchanged = 0;

        for (String name : allNames) {
            String p = prev.get(name), c = curr.get(name);
            if      (p == null)     added.add(new String[]{name, c});
            else if (c == null)     removed.add(new String[]{name, p});
            else if (!p.equals(c)) changed.add(new String[]{name, p, c});
            else                   unchanged++;
        }

        Set<String> prevLines    = readLines(prevTree);
        Set<String> newLines     = readLines(newTree);
        List<String> addedLines   = newLines.stream().filter(l -> !prevLines.contains(l)).sorted().collect(Collectors.toList());
        List<String> removedLines = prevLines.stream().filter(l -> !newLines.contains(l)).sorted().collect(Collectors.toList());

        boolean firstBuild = !Files.exists(prevChecksums);
        String ts = LocalDateTime.now().format(DateTimeFormatter.ofPattern("yyyy-MM-dd'T'HH:mm:ss"));

        StringBuilder sb = new StringBuilder();
        sb.append("=== Library Changes Report ===\n");
        sb.append("Generated : ").append(ts).append("\n");
        sb.append("Module    : ").append(artifact).append(":").append(version).append("\n\n");

        sb.append("=== Checksum Changes");
        sb.append(firstBuild ? " (first build - no previous record)" : " (compared to previous build)");
        sb.append(" ===\n");
        for (String[] e : added)   sb.append(String.format("  ADDED:    %-55s [%s]\n", e[0], e[1]));
        for (String[] e : removed) sb.append(String.format("  REMOVED:  %-55s [%s]\n", e[0], e[1]));
        for (String[] e : changed) sb.append(String.format("  CHANGED:  %-55s [was: %s -> now: %s]\n", e[0], e[1], e[2]));
        if (unchanged > 0) sb.append("  UNCHANGED: ").append(unchanged).append(" file(s)\n");
        if (added.isEmpty() && removed.isEmpty() && changed.isEmpty() && !firstBuild)
            sb.append("  (no changes)\n");

        sb.append("\n=== Dependency Tree Changes");
        sb.append(Files.exists(prevTree) ? " (compared to previous build)" : " (first build - no previous record)");
        sb.append(" ===\n");
        if (!addedLines.isEmpty()) {
            sb.append("  ADDED lines:\n");
            for (String l : addedLines) sb.append("    + ").append(l).append("\n");
        }
        if (!removedLines.isEmpty()) {
            sb.append("  REMOVED lines:\n");
            for (String l : removedLines) sb.append("    - ").append(l).append("\n");
        }
        if (addedLines.isEmpty() && removedLines.isEmpty() && Files.exists(prevTree))
            sb.append("  (no changes)\n");

        Files.writeString(changesFile, sb.toString());
        System.out.println("[INFO] lib-changes.txt written to: " + changesFile);

        Files.copy(newChecksums, prevChecksums, StandardCopyOption.REPLACE_EXISTING);
        Files.copy(newTree,      prevTree,      StandardCopyOption.REPLACE_EXISTING);
        System.out.println("[INFO] build-records updated in: " + buildRecordsDir);
    }

    static Map<String, String> parseChecksums(Path f) throws Exception {
        if (!Files.exists(f)) return new LinkedHashMap<>();
        Map<String, String> map = new LinkedHashMap<>();
        for (String line : Files.readAllLines(f)) {
            String t = line.trim();
            if (t.isEmpty()) continue;
            String[] parts = t.split("\\s+", 2);
            if (parts.length == 2) map.put(parts[1].trim(), parts[0].trim());
        }
        return map;
    }

    static Set<String> readLines(Path f) throws Exception {
        if (!Files.exists(f)) return new LinkedHashSet<>();
        return Files.readAllLines(f).stream()
            .map(String::trim)
            .filter(l -> !l.isEmpty())
            .collect(Collectors.toCollection(LinkedHashSet::new));
    }
}
