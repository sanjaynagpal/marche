# Build Integrity: Preventing Silent Runtime Dependency Loss

## Problem Statement

A runtime dependency can silently disappear from the distribution package without
failing the build. This was observed after upgrading to **JDK 8 Update 492**, but
the underlying gap exists independently of JDK version.

### Root Cause Analysis

The build pipeline has two compounding weaknesses:

**1. The assembly descriptor delegated dependency resolution to `copy-dependencies`.**

`binary.xml` used a `<fileSet>` pointing at `${project.build.directory}/lib`, which
is pre-populated by `maven-dependency-plugin:copy-dependencies`. This means the
assembly blindly copies whatever landed in `target/lib/`. If `copy-dependencies`
silently skipped a JAR — for any reason — the distribution shipped without it and
no step raised an error.

**2. `LibDiff.java` reported removals but did not fail the build.**

`LibDiff` compares the current `checksums.txt` against the previous build record and
flags any `REMOVED` artifact. However, it exits with code 0 regardless, so Maven
treats the build as successful even when artifacts have disappeared from the staging
directory.

### Why JDK 8u361+ / 8u492+ Exposed This

Starting with JDK 8 Update 361, Oracle tightened TLS (disabling TLS 1.0/1.1) and
made incremental changes to internal security and resolver behaviour in subsequent
updates. These changes affected how older versions of `maven-dependency-plugin`
resolved and copied transitive dependencies:

- Resolver inconsistencies with updated TLS constraints could cause artifact
  downloads to silently fail during a plugin's resolution phase, leaving the JAR
  absent from `target/lib/` with no build error.
- Changes to the `javax.*` and `com.sun.*` namespace availability affected annotation
  processors and code generators, causing some JARs to be skipped at resolution time.
- The endorsed-standards override mechanism (`java.endorsed.dirs`) was effectively
  disabled, so JARs previously picked up via that path vanished silently.

The Marche project already uses plugin versions above the minimums recommended for
8u492+ compatibility (`maven-dependency-plugin` 3.8.1, `maven-assembly-plugin` 3.8.0,
`maven-compiler-plugin` 3.15.0). The architectural gap — no post-assembly verification
— is what allowed failures to go undetected regardless of plugin versions.

---

## Prescribed Solution

Two complementary changes close both gaps:

### 1. Switch the assembly descriptor to `<dependencySets>`

Replace the `<fileSet>` that copied `target/lib/` with a `<dependencySets>` block.
This makes the assembly plugin directly responsible for resolving and staging runtime
dependencies, independent of whether `copy-dependencies` produced a complete
`target/lib/`. Key assembly descriptor settings:

- `<scope>runtime</scope>` — only runtime-scoped dependencies, no test or provided leakage
- `<useTransitiveDependencies>true</useTransitiveDependencies>` — full transitive closure
- `<useTransitiveFiltering>true</useTransitiveFiltering>` — exclusion rules propagate
  correctly through the dependency graph (required for correct behaviour in
  assembly-plugin 3.x with schema 2.2.0)
- `<useProjectArtifact>false</useProjectArtifact>` — the module JAR is included via a
  separate targeted `<fileSet>` to keep concerns separate

The `copy-dependencies` execution and checksum/diff pipeline (`LibDiff`) are retained
unchanged — they continue to serve as the audit trail.

### 2. Add `AssemblyVerifier` — post-assembly integrity gate

A new single-file build-support tool, `AssemblyVerifier.java`, runs in the Maven
`verify` phase (after `package`, so after the assembly tar.gz is fully built). It:

1. Reads `target/lib/checksums.txt` as the authoritative manifest of every runtime
   JAR that must be present in the distribution.
2. Opens the binary tar.gz using a pure-JDK TAR reader (no external dependencies).
3. Compares expected JARs against JARs found in `bin/lib/` inside the archive.
4. **Exits with code 1 to fail the build** if any expected artifact is absent.
5. Writes a `build-records/assembly-verification.txt` report for the audit trail.

The exec-maven-plugin execution is declared once in the root `pom.xml` under
`<build><plugins>` (not `pluginManagement`), bound to `verify`. Maven inherits this
to every module. POM-packaging aggregator modules (e.g., `polaris-eleven`) hit a
graceful `SKIPPED` path because they never produce `target/lib/checksums.txt`.

### What each gap catches

| Failure mode | Detected by |
|---|---|
| JAR copied to `target/lib/` but dropped by assembly | `AssemblyVerifier` |
| JAR missing from `target/lib/` (resolver skipped it) | `LibDiff` REMOVED entry + `AssemblyVerifier` mismatch |
| Assembly descriptor typo/exclusion added later | `AssemblyVerifier` |
| JAR disappears between builds (dependency removed) | `LibDiff` REMOVED entry |

---

## Build Commands

```bash
# Full integrity-checked build (recommended)
mvn clean verify

# Fast iteration build — skips assembly verification
mvn clean package

# Single module, fully verified
mvn clean verify -pl alpha-eight
```

`mvn clean verify` is the build command that guarantees no runtime artifact is
silently missing from the distribution. `mvn clean package` still works for rapid
development cycles but does not run `AssemblyVerifier`.

---

## Audit Trail

Each module's `build-records/` directory accumulates:

| File | Written by | Purpose |
|---|---|---|
| `checksums.txt` | `LibDiff` | SHA-256 of every runtime JAR in `target/lib/` |
| `dependency-tree.txt` | `LibDiff` | Maven dependency tree snapshot |
| `lib-changes.txt` | `LibDiff` | Diff vs. previous build (ADDED / REMOVED / CHANGED) |
| `assembly-verification.txt` | `AssemblyVerifier` | PASS / FAIL record for the tar.gz archive |
