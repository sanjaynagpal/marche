# Marche

Marche is a Maven multi-module project that demonstrates language evolution across JDK 8, 11, 17, and 21, alongside Kotlin/Ktor and Go variants. Each service module is a minimal HTTP service exposing a `/health` endpoint that showcases language features appropriate to its target runtime. The project also serves as a reference for modern Maven build patterns: cross-compilation, distribution packaging, dependency auditing, and build-record diffing.

---

## Modules

### Services

| Module | Language | Runtime Target | Port | Features Demonstrated |
|---|---|---|---|---|
| `alpha-eight` | Java | 8 | 8080 | Lambdas, Stream API, `Optional`, method references |
| `orion-eleven` | Java | 11 | 8081 | `var`, `HttpClient`, `String.isBlank`, `String.repeat` |
| `sirius-seventeen` | Java | 17 | 8082 | Records, sealed interfaces, pattern `instanceof`, text blocks |
| `vega-twenty-one` | Java | 21 | 8083 | Virtual threads, pattern switch, record patterns |
| `kepler-eleven` | Kotlin + Ktor 2.x | 11 | 8084 | Data classes, coroutines, scope functions (`let`, `also`) |
| `kepler-twenty-one` | Kotlin + Ktor 3.x | 21 | 8085 | Sealed interfaces, exhaustive `when`, virtual thread probe |
| `gamma-go` | Go | 1.26+ | 8086 | Goroutines, channels, defer, graceful shutdown, `net/http` |
| `polaris-havok` | Java | 11 | 8090 | `ProcessHandle`, `Optional.isEmpty`, array generator |
| `polaris-wanda` | Java | 11 | 8091 | `String.strip`, `Set.of`, `Files.readString` |

`polaris-eleven` is a POM-only aggregator — it produces no artifact of its own and exists solely to group `polaris-havok` and `polaris-wanda` under a shared parent.

### Libraries

| Module | Language | JDK Target | Purpose |
|---|---|---|---|
| `http-lib-java` | Java | 8 | Shared config loading, response helpers, and `HttpServer` setup for JVM service modules |

### Tools

| Module | Language | Purpose |
|---|---|---|
| `health-probe-java` | Java | CLI that queries `/health` endpoints on running Marche services; produces a self-contained JAR (Java 17, no external dependencies) |
| `health-probe-go` | Go | CLI that queries `/health` endpoints; Maven cross-compiles to Linux amd64 and Windows amd64 native binaries |

---

## Architecture

### HTTP Layer

Java modules use `com.sun.net.httpserver.HttpServer`, which is built into the JDK and requires no external web framework. Kotlin modules use Ktor running on Netty. The Go service (`gamma-go`) uses the `net/http` standard library with graceful shutdown via `os/signal`. All service modules expose a single `/health` endpoint over HTTP on the port listed above.

### Shared Library

`http-lib-java` provides config loading (SnakeYAML-backed, dot-notation properties), response helpers, and `HttpServer` setup. It is Java 8 compatible and consumed by JVM service modules as a Maven dependency.

### Configuration

Each module loads configuration from a YAML file selected by the `MARCHE_ENV` environment variable (`dev`, `prod`, or `test`; defaults to `dev`). The config directory is controlled by `MARCHE_CONFIG_DIR` (defaults to `../cfg` relative to `bin/` when running from a distribution tar, or `configuration/` for the Go service). The YAML is flattened to dot-notation properties such as `server.port` and `logging.level`. All modules fall back to hardcoded defaults when no config file is present, so they start successfully with no external configuration.

### No Framework, No Database

There is no Spring Boot, no ORM, no messaging layer, and no test suite. The project is intentionally minimal — the focus is on language features and build tooling, not application complexity.

---

## Health Endpoint

Every service module responds to `GET /health` with a plain-text body that identifies itself:

```
OK | alpha-eight | Java 8 | env=dev | features: lambdas, streams, optional
```

The response includes the module name, language and target version, active environment, and a brief summary of the language features in use.

---

## Distribution Layout

### JVM service modules

Each JVM service module produces two tarballs in `<module>/target/`:

| File | Contents |
|---|---|
| `{artifactId}-{version}-bin.tar.gz` | Runnable distribution |
| `{artifactId}-{version}-cfg.tar.gz` | Configuration distribution |

#### Binary tar (`bin/` is the top-level directory)

```
bin/
├── run.sh                          Unix launch script (executable, mode 0755)
├── run.bat                         Windows launch script
└── lib/
    ├── {module}-{version}.jar      Thin module JAR
    ├── snakeyaml-2.3.jar           Runtime dependency JARs (each separately)
    ├── ...
    ├── checksums.txt               SHA-256 hash per JAR (sha256sum-compatible format)
    ├── dependency-tree.txt         Full Maven dependency resolution graph
    └── lib-changes.txt             Diff vs. previous build (added, removed, changed JARs)
```

#### Config tar (`cfg/` is the top-level directory)

```
cfg/
├── application-dev.yaml
├── application-prod.yaml
└── application-test.yaml
```

Any YAML file placed in `<module>/configuration/` is automatically included — no build change required to add a new environment.

### Go modules

`gamma-go` and `health-probe-go` produce native binaries cross-compiled by Maven via `exec-maven-plugin`. No tarballs are generated; outputs land directly in `<module>/target/`:

```
target/
├── gamma-go-linux-amd64        Linux binary
└── gamma-go-windows-amd64.exe  Windows binary
```

---

## Running a Module

### Prerequisites

| Requirement | Version |
|---|---|
| JDK (build) | 25 |
| JDK (runtime) | Determined by each module's declared target — see table below |
| Maven | 3.9+ |
| Go | 1.26+ (only required to build `gamma-go` and Go tools) |

The `--release` flag passed to `javac` bakes the target version into every compiled class file as a bytecode major version number (`major = 44 + N`). Any JVM older than the target refuses to load the class and throws `UnsupportedClassVersionError` before the application starts. The runtime JVM requirement per module follows directly:

| Module | Minimum Runtime JVM |
|---|---|
| `alpha-eight` | Java 8+ |
| `orion-eleven`, `polaris-havok`, `polaris-wanda`, `kepler-eleven` | Java 11+ |
| `sirius-seventeen`, `health-probe-java` | Java 17+ |
| `vega-twenty-one`, `kepler-twenty-one` | Java 21+ |
| `gamma-go`, `health-probe-go` | Go 1.26+ (build-time only — output is a native binary) |

### Build

A Maven wrapper is included — no Maven installation required. It downloads Maven 3.9.15 on first use.

```bash
# Build everything (Unix)
./mvnw clean package

# Build everything (Windows)
mvnw.cmd clean package

# Build one module
./mvnw clean package -pl alpha-eight
```

See [Build.md](Build.md) for the full plugin chain and output artifact details.

### Extract and Run (Unix — JVM services)

```bash
# Extract both tarballs into the same directory
mkdir -p /opt/marche/alpha-eight
cd /opt/marche/alpha-eight
tar -xzf /path/to/alpha-eight-0.1.0-SNAPSHOT-bin.tar.gz
tar -xzf /path/to/alpha-eight-0.1.0-SNAPSHOT-cfg.tar.gz

# Run with the default (dev) environment
./bin/run.sh

# Run with a different environment
MARCHE_ENV=prod ./bin/run.sh

# Run with config from a non-standard location
MARCHE_CONFIG_DIR=/etc/marche/config ./bin/run.sh
```

### Extract and Run (Windows — JVM services)

```cmd
mkdir C:\marche\alpha-eight
cd C:\marche\alpha-eight
tar -xzf alpha-eight-0.1.0-SNAPSHOT-bin.tar.gz
tar -xzf alpha-eight-0.1.0-SNAPSHOT-cfg.tar.gz

bin\run.bat
```

Set `MARCHE_ENV` or `MARCHE_CONFIG_DIR` as environment variables before invoking the script to override the defaults.

### Run the Go service

After building, the native binary is in `gamma-go/target/`. No JVM or classpath required.

```bash
# Linux
MARCHE_ENV=prod ./gamma-go/target/gamma-go-linux-amd64

# Windows
gamma-go\target\gamma-go-windows-amd64.exe
```

### Verify

```bash
curl http://localhost:8080/health   # alpha-eight
curl http://localhost:8086/health   # gamma-go
```

---

## Audit Trail

The `build-records/` directory inside each module's source tree is the persistent audit store. It survives `mvn clean` and is committed to version control so the trail extends across releases.

| File | Written by | Purpose |
|---|---|---|
| `checksums.txt` | `LibDiff.java` (`package`) | SHA-256 hash of every runtime JAR — verifiable with `sha256sum -c checksums.txt` |
| `dependency-tree.txt` | `LibDiff.java` (`package`) | Full Maven dependency resolution graph captured at build time |
| `assembly-verification.txt` | `AssemblyVerifier.java` (`verify`) | Confirms every JAR in `checksums.txt` is present inside the binary tar |
| `bytecode-versions.txt` | `BytecodeReport.java` (`verify`) | Class file major version of every runtime JAR, checked against the module's declared Java target |

### Bytecode version verification

Every JVM service module declares a `java.target.release` property (e.g., `8` for `alpha-eight`). The Maven compiler plugin enforces this at compile time via `--release`. `BytecodeReport` runs in the `verify` phase, re-reads the actual class file headers baked into every JAR in `target/lib/`, and confirms the bytecode major version matches — closing the loop between what the compiler was told and what was actually shipped.

The class file major version is encoded in bytes 6–7 of every `.class` file header (after the `0xCAFEBABE` magic and 2-byte minor version). The mapping is `major = 44 + java_version` — Java 8 compiles to major 52, Java 11 to 55, Java 17 to 61, Java 21 to 65.

If any JAR in `target/lib/` has a higher major version than declared, the build fails immediately and the violation is recorded in the report.

**Sample `build-records/bytecode-versions.txt` for `alpha-eight` (declared target: Java 8)**

```
=== Bytecode Version Report ===
Generated       : 2026-05-10T14:32:01
Module          : alpha-eight:0.1.0-SNAPSHOT
Declared target : Java 8 (class file major version 52)

JAR                                                      Major  Java Version        Status
----------------------------------------------------------------------------------------------------
alpha-eight-0.1.0-SNAPSHOT.jar                              52  Java 8              OK
http-lib-java-0.1.0-SNAPSHOT.jar                            52  Java 8              OK
snakeyaml-2.3.jar                                           52  Java 8              OK

RESULT: PASS — all 3 JAR(s) are within the declared bytecode target (Java 8).
```

**Sample when a violation is detected**

```
=== Bytecode Version Report ===
Generated       : 2026-05-10T14:32:01
Module          : alpha-eight:0.1.0-SNAPSHOT
Declared target : Java 8 (class file major version 52)

JAR                                                      Major  Java Version        Status
----------------------------------------------------------------------------------------------------
alpha-eight-0.1.0-SNAPSHOT.jar                              52  Java 8              OK
http-lib-java-0.1.0-SNAPSHOT.jar                            52  Java 8              OK
snakeyaml-2.3.jar                                           55  Java 11             VIOLATION: major 55 > declared 52

RESULT: FAIL — 1 JAR(s) exceed the declared bytecode target:
  VIOLATION: snakeyaml-2.3.jar (major=55, Java 11 > declared Java 8)
```

Go modules (`gamma-go`) skip this step entirely — they produce native binaries, not JARs, so there is no `target/lib/` directory and the tool exits early with a `SKIPPED` note.

---

## Module Hierarchy

```
marche-parent (pom)
├── marche-services-parent  (pom — intermediate parent for all service modules)
├── marche-libraries-parent (pom — intermediate parent for library modules)
├── marche-tools-parent     (pom — intermediate parent for tool modules)
│
├── [libraries]
│   └── http-lib-java       (jar) → published to Nexus; consumed by JVM services
│
├── [tools]
│   ├── health-probe-java   (jar) → self-contained executable JAR
│   └── health-probe-go     (pom) → native binaries (Linux + Windows)
│
└── [services]
    ├── alpha-eight         (jar) → bin + cfg tars
    ├── orion-eleven        (jar) → bin + cfg tars
    ├── sirius-seventeen    (jar) → bin + cfg tars
    ├── vega-twenty-one     (jar) → bin + cfg tars
    ├── kepler-eleven       (jar) → bin + cfg tars
    ├── kepler-twenty-one   (jar) → bin + cfg tars
    ├── gamma-go            (pom) → native binaries (Linux + Windows)
    └── polaris-eleven      (pom — aggregator, no tars)
        ├── polaris-havok   (jar) → bin + cfg tars
        └── polaris-wanda   (jar) → bin + cfg tars
```
