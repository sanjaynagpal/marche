# Module Hierarchy

## Overview

Marche uses a three-level POM inheritance chain to separate three distinct kinds of modules:
**deployable services**, **shared libraries**, and **tools**.

```
marche-parent  (root)
├── marche-services-parent
│   ├── alpha-eight
│   ├── orion-eleven
│   ├── sirius-seventeen
│   ├── vega-twenty-one
│   ├── kepler-eleven
│   ├── kepler-twenty-one
│   └── polaris-eleven
│       ├── polaris-havok
│       └── polaris-wanda
├── marche-libraries-parent
│   └── http-lib-java
└── marche-tools-parent
    ├── health-probe-go    (Go, linux/windows amd64)
    └── health-probe-java  (Java 17, self-contained JAR)
```

## POM Roles

| POM | Packaging | Role |
|---|---|---|
| `marche-parent` | `pom` | Root aggregator. Owns all version properties, `pluginManagement`, `dependencyManagement`, and `distributionManagement`. Lists every module in the reactor. |
| `marche-services-parent` | `pom` | Intermediate parent for service modules. Runs `AssemblyVerifier` in the `verify` phase, which confirms every runtime JAR declared in `checksums.txt` is present in the binary distribution archive. |
| `marche-libraries-parent` | `pom` | Intermediate parent for library modules. No assembly or distribution packaging — library modules produce a plain JAR deployed directly to Nexus. |
| `marche-tools-parent` | `pom` | Intermediate parent for tool and utility modules. No JVM lifecycle — Maven delegates to language-native toolchains via `exec-maven-plugin`. Tools can be written in Go, Python, Bash, PowerShell, or any other language. |

## Inheritance Chain

### Service modules

```
alpha-eight  →  marche-services-parent  →  marche-parent
```

Service modules inherit:
- All version pins and dependency management from `marche-parent`
- `distributionManagement` (Nexus repos) from `marche-parent`
- `AssemblyVerifier` execution in the `verify` phase from `marche-services-parent`

### polaris sub-modules

```
polaris-havok  →  polaris-eleven  →  marche-services-parent  →  marche-parent
```

`polaris-eleven` is both an aggregator and an intermediate parent for its own sub-modules. It inherits the `AssemblyVerifier` execution from `marche-services-parent`, which runs on `polaris-havok` and `polaris-wanda` but skips gracefully on `polaris-eleven` itself (POM modules produce no JAR or `checksums.txt`).

### Library modules

```
http-lib-java  →  marche-libraries-parent  →  marche-parent
```

Library modules inherit version pins and Nexus deploy config but get no assembly, no dependency staging, and no `AssemblyVerifier`. They publish a plain JAR to Nexus and are consumed by service modules via a `<dependency>` declaration.

`http-lib-java` is the first library module. It provides shared HTTP utilities (`ConfigLoader`, `HttpResponder`, `MarcheServer`) for use by Java service modules. `alpha-eight` declares it as a dependency and delegates config loading and HTTP response handling to it.

### Tool modules

```
health-probe-go  →  marche-tools-parent  →  marche-parent
```

Tool modules use `<packaging>pom</packaging>` so Maven's JVM-specific lifecycle phases
(compiler, jar, surefire) do not run. All build steps are driven by explicit
`exec-maven-plugin` executions bound to Maven phases.

**Maven as a build orchestrator for Go**

Go supports native cross-compilation through environment variables: `GOOS` (target OS)
and `GOARCH` (target architecture). Setting `CGO_ENABLED=0` disables the C bridge,
which means the Go toolchain alone produces the final binary with no C compiler or
system libraries needed on the build machine. A single developer laptop running any OS
can therefore produce Linux and Windows binaries in the same Maven `package` run.

Maven phase mapping for Go tool modules:

| Maven phase | Go operation | Notes |
|---|---|---|
| `initialize` | `go mod download` | Warms the local module cache before compilation |
| `package` | `go build` (×2) | One execution per target platform; binaries land in `target/` |

The binaries are named `{artifactId}-linux-amd64` and `{artifactId}-windows-amd64.exe`.

**Constraint:** tool modules must use only the Go standard library or pure-Go modules.
Any dependency that links against C code (e.g. `mattn/go-sqlite3`) would require a
C cross-compiler for each target OS and breaks the `CGO_ENABLED=0` assumption.

**Java tool modules** (`health-probe-java`)

Java tools use standard `<packaging>jar</packaging>`. When the tool has no external
dependencies — as is the case for `health-probe-java`, which uses only `java.net.http`
from the JDK — the plain JAR produced by `maven-jar-plugin` is already self-contained.
The `Main-Class` manifest entry makes it runnable with `java -jar`. If external
dependencies are added later, replace `maven-jar-plugin` with `maven-shade-plugin`
to produce a fat JAR.

Java 17 features used in `health-probe-java`: records (`ProbeResult`), text blocks
(usage string), `var`, `HttpClient` with `BodyHandlers.ofString()`, and `.toList()`
on streams.

---

## Adding a New Library Module

1. Create the module directory under the repo root (standard Maven layout: `src/main/java`).
2. Set `<parent>` to `marche-libraries-parent` with `<relativePath>../marche-libraries-parent/pom.xml</relativePath>`.
3. Add the module to the `<modules>` list in the root `pom.xml` — place it **before** the service modules so the reactor builds it first.
4. Add a `<dependency>` entry in `<dependencyManagement>` in the root `pom.xml` so consuming modules can declare the dependency without specifying a version.
5. Declare the library as a `<dependency>` (no `<version>`) in any service module that needs it.

See `http-lib-java` and its use in `alpha-eight` as the reference example.

## Adding a New Tool Module

### Go tool

1. Create the module directory under the repo root. Place `go.mod` and source files at its root alongside `pom.xml`.
2. Set `<parent>` to `marche-tools-parent` with `<relativePath>../marche-tools-parent/pom.xml</relativePath>` and `<packaging>pom</packaging>`.
3. Add `exec-maven-plugin` executions for `go mod download` (phase `initialize`) and `go build` (phase `package`, once per target platform) with `GOOS`, `GOARCH`, and `CGO_ENABLED=0` set as `<environmentVariables>`.
4. Add the module to the `<modules>` list in the root `pom.xml`.

See `health-probe-go` as the reference example.

### Script-based tool (Bash, PowerShell, Python)

1. Create the module directory and place scripts under a `src/` subdirectory.
2. Use `<packaging>pom</packaging>` with no compiler plugin.
3. Use `maven-resources-plugin` or `maven-assembly-plugin` to package scripts into a distributable archive, and `exec-maven-plugin` for any validation step (e.g. `python -m py_compile`, `pwsh -Command "..."` syntax check).
4. Add the module to the `<modules>` list in the root `pom.xml`.

## Adding a New Service Module

1. Create the module directory under the repo root.
2. Set `<parent>` to `marche-services-parent`.
3. Add the module to the `<modules>` list in the root `pom.xml`.
4. Follow the existing service module layout (`binary/src/main/java`, `configuration/`).
