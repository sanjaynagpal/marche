# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Purpose

Marche is a Maven multi-module archetype demonstrating Java language evolution and build patterns across JDK 8, 11, 17, and 21, plus Kotlin/Ktor variants. Each module is a minimal HTTP service exposing a `/health` endpoint that showcases language features appropriate to its target JDK version.

## Build Commands

```bash
# Build all modules
mvn clean package

# Build a single module
mvn clean package -pl alpha-eight

# Compile without packaging
mvn clean compile

# Run a module's fat JAR
java -jar alpha-eight/target/alpha-eight-0.1.0-SNAPSHOT.jar
```

The build runtime is **JDK 25**; each module declares its own bytecode `<release>` target. All modules produce shade (uber) JARs via `maven-shade-plugin`.

There are no tests and no linting/code-quality plugins configured.

## Module Map

| Module | Language | JDK Target | Port | Focus |
|---|---|---|---|---|
| `alpha-eight` | Java | 8 | 8080 | Lambdas, Stream API, Optional |
| `orion-eleven` | Java | 11 | 8081 | `var`, `HttpClient`, `String.isBlank/repeat` |
| `sirius-seventeen` | Java | 17 | 8082 | Records, sealed interfaces, pattern `instanceof`, text blocks |
| `vega-twenty-one` | Java | 21 | 8083 | Virtual threads, pattern switch, record patterns |
| `kepler-eleven` | Kotlin + Ktor 2.x | 11 | 8084 | Data classes, coroutines, scope functions |
| `kepler-twenty-one` | Kotlin + Ktor 3.x | 21 | 8085 | Sealed interfaces, exhaustive `when`, virtual thread probe |
| `polaris-eleven` | POM (parent) | — | — | Aggregator for `polaris-havok` and `polaris-wanda` |
| `polaris-eleven/polaris-havok` | Java | 11 | 8090 | `ProcessHandle`, `Optional.isEmpty`, array generator |
| `polaris-eleven/polaris-wanda` | Java | 11 | 8091 | `String.strip`, `Set.of`, `Files.readString` |

## Module Structure

Every module follows this layout:

```
<module>/
├── pom.xml
├── binary/src/main/
│   ├── java/          # Java modules
│   └── kotlin/        # Kotlin modules (kepler-*)
└── configuration/
    ├── application-dev.yaml
    ├── application-prod.yaml
    └── application-test.yaml
```

## Configuration

Each module loads YAML config via custom logic (no Spring Boot). Key env vars:

- `MARCHE_ENV` — selects config file (`dev`, `prod`, `test`); defaults to `dev`
- `MARCHE_CONFIG_DIR` — path to config directory; defaults to `configuration/`

The YAML is flattened to dot-notation properties (e.g., `server.port`, `logging.level`). All modules fall back to hardcoded defaults if a config file is missing.

## Architecture Patterns

- **Java modules** use `com.sun.net.httpserver.HttpServer` (JDK built-in, no web framework).
- **Kotlin modules** use Ktor with Netty.
- No database, ORM, or messaging layer — this is intentional.
- All `/health` responses are plain text reporting module name, language, JDK version, active env, and demonstrated features.
