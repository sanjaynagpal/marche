# Build

## Prerequisites

- **JDK 25** must be the active JDK. Maven uses it as the build compiler; each module cross-compiles to its own declared `<release>` target.
- **Maven 3.9+** — required for the multi-module lifecycle behaviour used here.
- Internet access for the first build (Maven resolves plugins and dependencies from Maven Central).

---

## Commands

```bash
# Build all modules
mvn clean package

# Build a single module
mvn clean package -pl alpha-eight

# Build a module and its full parent chain
mvn clean package -pl alpha-eight -am

# Compile without packaging (no tarballs produced)
mvn clean compile
```

---

## Output Artifacts

Each of the eight leaf modules produces two tarballs in `<module>/target/`:

| File | Description |
|---|---|
| `{artifactId}-{version}-bin.tar.gz` | Runnable distribution — scripts, JARs, checksums, and metadata |
| `{artifactId}-{version}-cfg.tar.gz` | Configuration distribution — all environment YAML files |

### Binary tar layout

```
bin/
├── run.sh                         Unix launch script (file mode 0755)
├── run.bat                        Windows launch script
└── lib/
    ├── {module}-{version}.jar     Module's own thin JAR
    ├── snakeyaml-2.3.jar          Runtime dependency JARs (each separately)
    ├── ...                        (Kotlin modules include kotlin-stdlib, ktor-*, logback, etc.)
    ├── checksums.txt              SHA-256 hash per JAR, sha256sum-compatible format
    ├── dependency-tree.txt        Maven dependency:tree output captured at build time
    └── lib-changes.txt            Diff report comparing this build to the previous one
```

### Configuration tar layout

```
cfg/
├── application-dev.yaml
├── application-prod.yaml
└── application-test.yaml
```

Every file found in `<module>/configuration/` is included automatically. Adding a new environment requires only placing a new YAML file there — no pom change needed.

---

## Maven Plugin Chain

The build does not use `maven-shade-plugin`. Instead, a chain of four plugins executes in order within the `package` phase. The default `jar:jar` lifecycle binding runs first (producing the thin module JAR), followed by the explicitly declared plugins in pom declaration order.

```
package phase execution order
──────────────────────────────────────────────────────────────────
1. jar:jar               (default lifecycle binding — thin module JAR)
2. dependency:copy-deps  copy runtime dependency JARs → target/lib/
3. dependency:dep-tree   write dependency tree      → target/lib/dependency-tree.txt
4. antrun:stage-jar      copy module JAR            → target/lib/
5. antrun:checksums      SHA-256 all *.jar files    → target/lib/checksums.txt
6. exec:generate-diff    diff vs. build-records/    → target/lib/lib-changes.txt
                           update build-records/
7. assembly:single       package binary tar + cfg tar
──────────────────────────────────────────────────────────────────
```

### Plugin 1 — `maven-compiler-plugin`: cross-compile to the target JDK

Each module declares a `<java.target.release>` property (8, 11, 17, or 21). The compiler plugin is configured with:

```xml
<release>${java.target.release}</release>
```

The `--release` flag sets both the source language level and the bytecode target, and additionally gates API usage: the compiler rejects any class or method not available in that release. This is stronger than the older `<source>` + `<target>` pair which only controlled syntax.

Kotlin modules additionally invoke `kotlin-maven-plugin` in the `compile` phase before `jar:jar` runs.

#### Bytecode major version and the runtime guarantee

Every Java `.class` file begins with a 4-byte magic number followed by a 2-byte minor version and a 2-byte major version. The major version encodes the minimum JVM required to load the file: `major = 44 + N`, where N is the target JDK version.

| Module | `java.target.release` | Class file major version | Minimum runtime JVM |
|---|---|---|---|
| `alpha-eight` | 8 | 52 | Java 8+ |
| `orion-eleven`, `polaris-havok`, `polaris-wanda` | 11 | 55 | Java 11+ |
| `sirius-seventeen` | 17 | 61 | Java 17+ |
| `vega-twenty-one` | 21 | 65 | Java 21+ |
| `kepler-eleven` | 11 | 55 | Java 11+ |
| `kepler-twenty-one` | 21 | 65 | Java 21+ |

Any JVM older than the target version refuses to load the class and throws `UnsupportedClassVersionError` before `main` is ever called. Because `--release` bakes this version into every compiled class in the module JAR, the guarantee is unconditional: a JVM meeting the minimum version above will load and execute the module's own code without error.

The API gating is a second guarantee: `--release` causes the compiler to resolve symbols against the API snapshot for that exact JDK version, not the JDK 25 build JVM. If a call site uses a method added in JDK 12 but the module targets JDK 11, the build fails at compile time rather than at runtime with `NoSuchMethodError`.

#### What `--release` does not cover

`--release` controls only the classes the compiler produces — the module's own source files. It has no authority over the dependency JARs that ship alongside the module in `bin/lib/`. A runtime dependency compiled against a newer JDK than the module target can silently pass the build and then throw `UnsupportedClassVersionError` at startup when the JVM attempts to load that dependency's classes. See the **Runtime Compatibility Verification** section below for how to catch this at build time.

### Plugin 2 — `maven-dependency-plugin`: populate `target/lib/`

Two executions, both bound to the `package` phase:

**`copy-deps`** — copies all `runtime`-scoped transitive dependency JARs (not the module's own JAR) into `target/lib/`.

```xml
<goal>copy-dependencies</goal>
<configuration>
    <outputDirectory>${project.build.directory}/lib</outputDirectory>
    <includeScope>runtime</includeScope>
</configuration>
```

**`dep-tree`** — invokes `dependency:tree` with file output, writing the full resolution graph to `target/lib/dependency-tree.txt`. This gives a build-time record of exactly which transitive dependencies were resolved, captured inside every distribution tarball for auditing.

### Plugin 3 — `maven-antrun-plugin`: stage the module JAR and generate checksums

**`stage-jar`** — uses Ant's `<copy>` task to move the thin module JAR from `target/` into `target/lib/`. This runs after the dependency JARs are already in place, so all JARs are co-located before checksums are computed.

**`generate-checksums`** — uses Ant's built-in `<checksum>` task:

```xml
<checksum algorithm="SHA-256" format="MD5SUM" fileext=".sha256">
    <fileset dir="${project.build.directory}/lib" includes="*.jar"/>
</checksum>
```

`format="MD5SUM"` produces one line per file in the form `hash  *filename`, matching the output format of the Unix `sha256sum` utility. Ant writes a `.sha256` sidecar file alongside each JAR; these are concatenated into a single `checksums.txt` with `<concat>` and the sidecars are deleted. The resulting file can be verified directly:

```bash
cd bin/lib && sha256sum -c checksums.txt
```

### Plugin 4 — `exec-maven-plugin`: build-record diff

Runs `java --source 11 src/build-support/LibDiff.java` as a forked process, passing four arguments: the module basedir, the target directory, the artifact ID, and the version.

`LibDiff.java` is a single-file Java program (`java --source` syntax, no prior compilation step) that:

1. Reads the previous build's `checksums.txt` and `dependency-tree.txt` from `<module>/build-records/`.  On the very first build, or when `build-records/` is absent, all JARs are treated as newly added.
2. Reads the just-generated versions from `target/lib/`.
3. Computes the set difference for checksums:
   - JARs in the new build but not the previous → **ADDED**
   - JARs in the previous build but not the new → **REMOVED**
   - JARs in both but with different SHA-256 hashes → **CHANGED** (e.g. a SNAPSHOT rebuilt)
   - Remaining JARs → **UNCHANGED** (count only)
4. Computes line-level differences for the dependency tree (lines added, lines removed).
5. Writes `target/lib/lib-changes.txt` — a timestamped, structured report.
6. Overwrites `build-records/checksums.txt` and `build-records/dependency-tree.txt` with the new state, ready to be diffed against on the next build.

**Example `lib-changes.txt` on a subsequent build:**

```
=== Library Changes Report ===
Generated : 2026-05-09T23:22:00
Module    : alpha-eight:0.1.0-SNAPSHOT

=== Checksum Changes (compared to previous build) ===
  CHANGED:  *alpha-eight-0.1.0-SNAPSHOT.jar   [was: d53913... -> now: 21c1bd...]
  UNCHANGED: 1 file(s)

=== Dependency Tree Changes (compared to previous build) ===
  (no changes)
```

> **Why a Java program instead of Groovy or a shell script?**
>
> Groovy's ASM library (used internally to parse class files) does not yet support JDK 25 bytecode (class file major version 69). Invoking GMavenPlus on a JDK 25 build raises `Unsupported class file major version 69`. A plain `java --source 11` invocation avoids this entirely — the `java` launcher compiles and runs the source file directly, with no third-party scripting engine in the path. Shell scripts were ruled out because the project must build correctly on both Unix and Windows without requiring separate script variants.

### Plugin 5 — `maven-assembly-plugin`: package the two tarballs

A single execution (`id: dist`, phase `package`, goal `single`) reads two shared assembly descriptors located at the root of the multi-module project under `src/assembly/`:

**`binary.xml`**

- Pulls run scripts from `${project.basedir}/binary/src/main/scripts/` into `bin/`; `run.sh` is assigned Unix file mode `0755`.
- Pulls everything in `${project.build.directory}/lib/` into `bin/lib/` — the JARs, `checksums.txt`, `dependency-tree.txt`, and `lib-changes.txt` are all present by this point.
- Sets `<includeBaseDirectory>false</includeBaseDirectory>` so `bin/` is the root of the archive, not a version-prefixed wrapper.

**`cfg.xml`**

- Pulls everything in `${project.basedir}/configuration/` into `cfg/`.
- No include filter — any YAML file added to that directory is packaged automatically.
- Sets `<includeBaseDirectory>false</includeBaseDirectory>` so `cfg/` is the root.

The descriptors are shared across all eight modules via `${maven.multiModuleProjectDirectory}`, the Maven property that resolves to the root project directory regardless of which submodule is currently being built. This includes the nested `polaris-havok` and `polaris-wanda` modules, where `${project.basedir}` is two levels deep.

---

## Audit Trail

Three files shipped inside every binary tarball form an artifact-level audit trail:

| File | Purpose |
|---|---|
| `bin/lib/checksums.txt` | Exact SHA-256 hash of every shipped JAR — use for integrity verification at deployment |
| `bin/lib/dependency-tree.txt` | Full Maven dependency resolution graph at build time — shows transitive dependency versions |
| `bin/lib/lib-changes.txt` | Structured diff vs. the previous build — highlights new, removed, and hash-changed JARs |

The `build-records/` directory inside each module's source tree is the running ledger. It persists the previous build's checksums and dependency tree across `mvn clean`, enabling every subsequent build to produce a meaningful diff. Committing `build-records/` to version control extends the audit trail across releases.

---

## Runtime Compatibility Verification

The `--release` flag guarantees the module's own classes are compatible with the declared JDK target. It does not scan the dependency JARs shipped in `bin/lib/`. The `enforceBytecodeVersion` rule from the `extra-enforcer-rules` extension fills this gap: it inspects every `.class` file in the compiled output **and** every dependency JAR on the classpath, failing the build if any class file major version exceeds the declared maximum.

### Why this matters

A future dependency upgrade could silently introduce bytecode compiled for a newer JDK. For example, if a new version of SnakeYAML were compiled for Java 17 and `polaris-wanda` targets Java 11, the module's own classes would still pass compilation — but the JVM would throw `UnsupportedClassVersionError` when loading SnakeYAML at startup. Without build-time enforcement this is only discovered at deployment.

### How it is configured

`maven-enforcer-plugin` is declared in root `pom.xml` `<pluginManagement>` with `extra-enforcer-rules` as a plugin dependency, so the version is pinned centrally. Each leaf module adds one execution:

```xml
<plugin>
    <groupId>org.apache.maven.plugins</groupId>
    <artifactId>maven-enforcer-plugin</artifactId>
    <executions>
        <execution>
            <id>enforce-bytecode-version</id>
            <goals><goal>enforce</goal></goals>
            <configuration>
                <rules>
                    <enforceBytecodeVersion>
                        <maxJdkVersion>${java.target.release}</maxJdkVersion>
                        <ignoredScopes>test</ignoredScopes>
                    </enforceBytecodeVersion>
                </rules>
            </configuration>
        </execution>
    </executions>
</plugin>
```

`maxJdkVersion` reuses the `${java.target.release}` property already declared in each module — no additional per-module configuration is needed. The rule binds to the `validate` phase, which runs before `compile`, so a bytecode violation fails the build immediately before any compilation or packaging work begins.

### What this checks

| Source | Checked by `--release` | Checked by `enforceBytecodeVersion` |
|---|---|---|
| Module's own compiled classes | Yes — rejects incompatible API calls at compile time | Yes — scans every `.class` file in `target/classes/` |
| Runtime dependency JARs in `bin/lib/` | No | Yes — inspects every `.class` inside each JAR |
| Transitive dependencies | No | Yes — follows the full runtime dependency graph |

Together, `--release` and `enforceBytecodeVersion` provide end-to-end assurance: the module compiles only compatible code, and no shipped dependency can introduce a newer bytecode version that would fail on the deployment JVM.

---

## Plugin Version Registry

All plugin versions are pinned centrally in the root `pom.xml` `<pluginManagement>` block.

| Plugin | Version |
|---|---|
| `maven-compiler-plugin` | 3.15.0 |
| `maven-jar-plugin` | 3.5.0 |
| `maven-resources-plugin` | 3.5.0 |
| `maven-dependency-plugin` | 3.8.1 |
| `maven-antrun-plugin` | 3.1.0 |
| `exec-maven-plugin` | 3.4.1 |
| `maven-assembly-plugin` | 3.8.0 |
| `kotlin-maven-plugin` | 2.3.21 |
| `maven-enforcer-plugin` | 3.5.0 |
| `extra-enforcer-rules` | 1.9.0 |

---

## Module Hierarchy

```
marche-parent (pom)
├── alpha-eight      (jar)  → bin + cfg tars
├── orion-eleven     (jar)  → bin + cfg tars
├── sirius-seventeen (jar)  → bin + cfg tars
├── vega-twenty-one  (jar)  → bin + cfg tars
├── kepler-eleven    (jar)  → bin + cfg tars
├── kepler-twenty-one(jar)  → bin + cfg tars
└── polaris-eleven   (pom — aggregator, no tars)
    ├── polaris-havok (jar) → bin + cfg tars
    └── polaris-wanda (jar) → bin + cfg tars
```

`polaris-eleven` inherits from `marche-parent` and passes that inheritance to its children. The `${maven.multiModuleProjectDirectory}` property ensures that the shared assembly descriptors and `LibDiff.java` are always resolved relative to the root, even for `polaris-havok` and `polaris-wanda`.
