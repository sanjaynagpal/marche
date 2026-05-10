# Module Hierarchy

## Overview

Marche uses a three-level POM inheritance chain to separate two distinct kinds of modules:
**deployable services** and **shared libraries**.

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
└── marche-libraries-parent
    └── http-lib-java
```

## POM Roles

| POM | Packaging | Role |
|---|---|---|
| `marche-parent` | `pom` | Root aggregator. Owns all version properties, `pluginManagement`, `dependencyManagement`, and `distributionManagement`. Lists every module in the reactor. |
| `marche-services-parent` | `pom` | Intermediate parent for service modules. Runs `AssemblyVerifier` in the `verify` phase, which confirms every runtime JAR declared in `checksums.txt` is present in the binary distribution archive. |
| `marche-libraries-parent` | `pom` | Intermediate parent for library modules. No assembly or distribution packaging — library modules produce a plain JAR deployed directly to Nexus. |

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

## Adding a New Library Module

1. Create the module directory under the repo root (standard Maven layout: `src/main/java`).
2. Set `<parent>` to `marche-libraries-parent` with `<relativePath>../marche-libraries-parent/pom.xml</relativePath>`.
3. Add the module to the `<modules>` list in the root `pom.xml` — place it **before** the service modules so the reactor builds it first.
4. Add a `<dependency>` entry in `<dependencyManagement>` in the root `pom.xml` so consuming modules can declare the dependency without specifying a version.
5. Declare the library as a `<dependency>` (no `<version>`) in any service module that needs it.

See `http-lib-java` and its use in `alpha-eight` as the reference example.

## Adding a New Service Module

1. Create the module directory under the repo root.
2. Set `<parent>` to `marche-services-parent`.
3. Add the module to the `<modules>` list in the root `pom.xml`.
4. Follow the existing service module layout (`binary/src/main/java`, `configuration/`).
