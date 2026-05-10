# Marche

Marche is a Maven multi-module project that demonstrates Java language evolution across JDK 8, 11, 17, and 21, alongside Kotlin/Ktor equivalents. Each module is a minimal HTTP service exposing a `/health` endpoint that showcases language features appropriate to its target JDK version. The project also serves as a reference for modern Maven build patterns: cross-compilation, distribution packaging, dependency auditing, and build-record diffing.

---

## Modules

| Module | Language | JDK Target | Port | Features Demonstrated |
|---|---|---|---|---|
| `alpha-eight` | Java | 8 | 8080 | Lambdas, Stream API, `Optional`, method references |
| `orion-eleven` | Java | 11 | 8081 | `var`, `HttpClient`, `String.isBlank`, `String.repeat` |
| `sirius-seventeen` | Java | 17 | 8082 | Records, sealed interfaces, pattern `instanceof`, text blocks |
| `vega-twenty-one` | Java | 21 | 8083 | Virtual threads, pattern switch, record patterns |
| `kepler-eleven` | Kotlin + Ktor 2.x | 11 | 8084 | Data classes, coroutines, scope functions (`let`, `also`) |
| `kepler-twenty-one` | Kotlin + Ktor 3.x | 21 | 8085 | Sealed interfaces, exhaustive `when`, virtual thread probe |
| `polaris-havok` | Java | 11 | 8090 | `ProcessHandle`, `Optional.isEmpty`, array generator |
| `polaris-wanda` | Java | 11 | 8091 | `String.strip`, `Set.of`, `Files.readString` |

`polaris-eleven` is a POM-only aggregator — it produces no artifact of its own and exists solely to group `polaris-havok` and `polaris-wanda` under a shared parent.

---

## Architecture

### HTTP Layer

Java modules use `com.sun.net.httpserver.HttpServer`, which is built into the JDK and requires no external web framework. Kotlin modules use Ktor running on Netty. Both approaches expose a single `/health` endpoint over HTTP on the port listed above.

### Configuration

Each module loads configuration from a YAML file selected by the `MARCHE_ENV` environment variable (`dev`, `prod`, or `test`; defaults to `dev`). The config directory is controlled by `MARCHE_CONFIG_DIR` (defaults to `../cfg` relative to `bin/` when running from a distribution tar). The YAML is parsed by SnakeYAML and flattened to dot-notation properties such as `server.port` and `logging.level`. All modules fall back to hardcoded defaults when no config file is present, so they start successfully with no external configuration.

### No Framework, No Database

There is no Spring Boot, no ORM, no messaging layer, and no test suite. The project is intentionally minimal — the focus is on language features and build tooling, not application complexity.

---

## Health Endpoint

Every module responds to `GET /health` with a plain-text body that identifies itself:

```
OK | alpha-eight | Java 8 | env=dev | features: lambdas, streams, optional
```

The response includes the module name, language and target version, active environment, and a brief summary of the language features in use.

---

## Distribution Layout

Each module produces two tarballs in `<module>/target/`:

| File | Contents |
|---|---|
| `{artifactId}-{version}-bin.tar.gz` | Runnable distribution |
| `{artifactId}-{version}-cfg.tar.gz` | Configuration distribution |

### Binary tar (`bin/` is the top-level directory)

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

### Config tar (`cfg/` is the top-level directory)

```
cfg/
├── application-dev.yaml
├── application-prod.yaml
└── application-test.yaml
```

Any YAML file placed in `<module>/configuration/` is automatically included — no build change required to add a new environment.

---

## Running a Module

### Prerequisites

| Requirement | Version |
|---|---|
| JDK (build) | 25 |
| JDK (runtime) | Determined by each module's declared target — see table below |
| Maven | 3.9+ |

The `--release` flag passed to `javac` bakes the target version into every compiled class file as a bytecode major version number (`major = 44 + N`). Any JVM older than the target refuses to load the class and throws `UnsupportedClassVersionError` before the application starts. The runtime JVM requirement per module follows directly:

| Module | Minimum Runtime JVM |
|---|---|
| `alpha-eight` | Java 8+ |
| `orion-eleven`, `polaris-havok`, `polaris-wanda`, `kepler-eleven` | Java 11+ |
| `sirius-seventeen` | Java 17+ |
| `vega-twenty-one`, `kepler-twenty-one` | Java 21+ |

### Build

```bash
# Build everything
mvn clean package

# Build one module
mvn clean package -pl alpha-eight
```

See [Build.md](Build.md) for the full plugin chain and output artifact details.

### Extract and Run (Unix)

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

### Extract and Run (Windows)

```cmd
mkdir C:\marche\alpha-eight
cd C:\marche\alpha-eight
tar -xzf alpha-eight-0.1.0-SNAPSHOT-bin.tar.gz
tar -xzf alpha-eight-0.1.0-SNAPSHOT-cfg.tar.gz

bin\run.bat
```

Set `MARCHE_ENV` or `MARCHE_CONFIG_DIR` as environment variables before invoking the script to override the defaults.

### Verify

```bash
curl http://localhost:8080/health
```

---

## Audit Trail

Every binary tar ships three audit files inside `bin/lib/`:

| File | Purpose |
|---|---|
| `checksums.txt` | SHA-256 hash of every shipped JAR — verifiable with `sha256sum -c checksums.txt` |
| `dependency-tree.txt` | Full Maven dependency resolution graph captured at build time |
| `lib-changes.txt` | Structured diff vs. the previous build — highlights new, removed, and changed JARs |

The `build-records/` directory inside each module's source tree persists `checksums.txt` and `dependency-tree.txt` across `mvn clean` runs. Committing `build-records/` to version control extends the audit trail across releases.

---

## Module Hierarchy

```
marche-parent (pom)
├── alpha-eight         (jar) → bin + cfg tars
├── orion-eleven        (jar) → bin + cfg tars
├── sirius-seventeen    (jar) → bin + cfg tars
├── vega-twenty-one     (jar) → bin + cfg tars
├── kepler-eleven       (jar) → bin + cfg tars
├── kepler-twenty-one   (jar) → bin + cfg tars
└── polaris-eleven      (pom — aggregator, no tars)
    ├── polaris-havok   (jar) → bin + cfg tars
    └── polaris-wanda   (jar) → bin + cfg tars
```
