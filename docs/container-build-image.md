# Container Build Image

## Purpose

Marche requires three language runtimes to build the full reactor: a JDK for Java and
Kotlin modules, a Go toolchain for Go modules, and Maven to orchestrate the whole build.
Asking every developer and every CI agent to install and keep those runtimes in sync is
error-prone. A developer on JDK 21 will silently compile vega-twenty-one differently
than a CI agent on JDK 25, and a mismatch between Go versions can produce binaries with
different behaviour.

The build image solves this by packaging every required runtime into a single, versioned
OCI image. Anyone who can run Podman or Docker can produce an identical build, regardless
of what is installed on the host machine.

**What the image provides:**

| Tool | Purpose in the Marche build |
|---|---|
| JDK 25 | Build runtime declared in `CLAUDE.md`. Compiles Java modules targeting bytecode levels 8, 11, 17, and 21. JDK ≥ 21 is mandatory — `vega-twenty-one` and `kepler-twenty-one` both declare `<release>21</release>` and use Java 21 APIs (virtual threads). |
| Go 1.26.3 | Matches the `go` directive in `gamma-go/go.mod` and `health-probe-go/go.mod`. Maven delegates all Go compilation to the Go toolchain via `exec-maven-plugin`. |
| Maven 3.9.9 | The reactor orchestrator. Drives JVM compilation, Go cross-compilation, assembly packaging, and dependency staging across all modules. |
| Git 2.43.x | Installed from UBI 8 repos. Required by some Maven plugins (e.g. `maven-release-plugin`), and needed if a CI pipeline clones the repository inside the container. |

---

## Requirements

### Host prerequisites

- **Podman 4+** (preferred) or **Docker 24+** — any host OS (Linux, macOS, Windows with
  Podman Desktop or Docker Desktop).
- Network access to the following during the image build:
  - `registry.access.redhat.com` — base UBI 8 image
  - `cdn.redhat.com` / `ubi-8-*.rpm` repositories — yum packages (Git, curl, etc.)
  - `api.adoptium.net` — Eclipse Temurin JDK 25 binary (~135 MB)
  - `go.dev/dl` — Go 1.26.3 tarball (~64 MB)
  - `archive.apache.org/dist/maven` — Maven 3.9.9 binary (~9 MB)

### Runtime prerequisites

When running a container built from this image, the following must be supplied at
runtime. Nothing sensitive is baked into the image.

| What | How to supply |
|---|---|
| Nexus publish credentials | `-e NEXUS_USER=<user> -e NEXUS_PASSWORD=<password>` |
| Maven artifact cache | `-v /host/m2-cache:/root/.m2/repository:Z` (optional but strongly recommended — avoids re-downloading hundreds of MB on every build) |
| Go module cache | `-v /host/go-cache:/root/go/pkg/mod:Z` (optional, same reason) |
| Source tree | `-v /path/to/marche:/workspace:Z` and `-w /workspace` |

The `:Z` suffix on volume mounts sets the correct SELinux label. It is required when
running on RHEL 8/9 hosts (the production CI environment) and is harmless on
non-SELinux hosts.

---

## Image Naming Convention

Build images follow a structured tag that encodes the key toolchain versions:

```
ubi8-jdk{java-major}-go{go-minor}-mvn
```

Examples:

| Image name | JDK | Go | Notes |
|---|---|---|---|
| `ubi8-jdk25-go126-mvn` | 25 | 1.26.x | **Current image** — matches repo requirements |
| `ubi8-jdk21-go126-mvn` | 21 | 1.26.x | Hypothetical LTS-pinned variant |
| `ubi8-jdk25-go127-mvn` | 25 | 1.27.x | Future — when go.mod bumps Go version |

The Maven version does not appear in the folder name because it changes less often and is
tracked via the image tag (e.g. `ubi8-jdk25-go126-mvn:3.9.9`).

Each distinct combination lives in its own subdirectory under `container/build/`:

```
container/
└── build/
    └── ubi8-jdk25-go126-mvn/
        ├── Containerfile    ← Podman-native build file
        ├── Dockerfile       ← identical content; Docker/buildah compatibility
        └── settings.xml     ← Maven server credentials via ${env.*} references
```

---

## Implementation

### Base image

```
registry.access.redhat.com/ubi8/ubi:8.10-1778062799
```

Red Hat Universal Base Image 8 (UBI 8) is a freely redistributable, enterprise-grade
base layer. The specific tag `8.10-1778062799` pins to an exact build of the UBI 8.10
minor release, making image rebuilds reproducible without relying on floating `latest`
tags. It is the same lineage as RHEL 8, so binaries built inside this image run without
modification on RHEL 8/9 production hosts.

### Layer structure and caching

Each major tool installation occupies its own `RUN` layer. This lets Podman/Docker cache
completed layers and skip them on rebuild if only a later layer changes (e.g. updating
`settings.xml` does not retrigger the JDK download).

```
Layer 1  FROM ubi8/ubi:8.10-1778062799
Layer 2  LABEL metadata
Layer 3  ARG version defaults
Layer 4  RUN yum install git curl tar ...      ← changes rarely
Layer 5  RUN curl JDK 25 + install             ← changes on JDK update
Layer 6  ENV JAVA_HOME / PATH
Layer 7  RUN curl Go 1.26.3 + install          ← changes on Go update
Layer 8  ENV GOROOT / GOPATH / PATH
Layer 9  RUN curl Maven 3.9.9 + install        ← changes on Maven update
Layer 10 ENV MAVEN_HOME / PATH / MAVEN_OPTS
Layer 11 COPY settings.xml                     ← changes on settings update
Layer 12 RUN mkdir cache directories
Layer 13 RUN sanity check (java/go/mvn/git)    ← fails the build if a tool is missing
Layer 14 WORKDIR /workspace
Layer 15 CMD ["/bin/bash"]
```

### Version arguments

All three tool versions are declared as build-time `ARG`s with defaults matching the
current repo requirements. They can be overridden at `podman build` time without editing
the file:

```
ARG JAVA_MAJOR=25
ARG GO_VERSION=1.26.3
ARG MAVEN_VERSION=3.9.9
```

### JDK installation

JDK 25 is downloaded from the Eclipse Temurin (Adoptium) API:

```
https://api.adoptium.net/v3/binary/latest/{JAVA_MAJOR}/ga/linux/x64/jdk/hotspot/normal/eclipse
```

This URL always resolves to the latest GA patch release for the requested major version.
It uses HTTP redirects to the GitHub release asset, so no build number needs to be
hard-coded. When a stricter reproducibility policy is required, replace this URL with a
direct GitHub release asset URL that includes the exact build number (e.g.
`temurin25-binaries/releases/download/jdk-25.0.3+9/...`).

The tarball is extracted to `/opt/java` with `--strip-components=1` so that
`/opt/java/bin/java` is the executable. `JAVA_HOME=/opt/java` is then exported to
`PATH`.

### Go installation

Go 1.26.3 is downloaded from the official Go distribution page:

```
https://go.dev/dl/go{GO_VERSION}.linux-amd64.tar.gz
```

The tarball is extracted to `/usr/local`, placing the Go toolchain at `/usr/local/go`.
This is the layout expected by the standard Go installation instructions. `GOROOT`,
`GOPATH`, and both bin directories are exported.

### Maven installation

Maven 3.9.9 is downloaded from the Apache permanent archive:

```
https://archive.apache.org/dist/maven/maven-3/{MAVEN_VERSION}/binaries/apache-maven-{MAVEN_VERSION}-bin.tar.gz
```

`archive.apache.org` (not `downloads.apache.org`) is used deliberately: the archive
never removes old releases, so the URL remains stable even after newer Maven versions
are published. The tarball is extracted to `/opt` and symlinked to `/opt/maven` so that
upgrading Maven only requires updating the `ARG` default and removing the old symlink.

`MAVEN_OPTS` is set to `-Xms256m -Xmx2g -XX:+UseG1GC`. Increase `-Xmx` if the CI
agent has more than 4 GB available.

### Maven settings and credentials

`settings.xml` is copied into the image at `/root/.m2/settings.xml`. It configures two
Nexus server entries matching the `<id>` values in the root `pom.xml`
`<distributionManagement>` block:

```xml
<server>
    <id>nexus-releases</id>
    <username>${env.NEXUS_USER}</username>
    <password>${env.NEXUS_PASSWORD}</password>
</server>
<server>
    <id>nexus-snapshots</id>
    <username>${env.NEXUS_USER}</username>
    <password>${env.NEXUS_PASSWORD}</password>
</server>
```

Maven resolves `${env.VAR}` natively at runtime — no shell preprocessing or entrypoint
templating is required. The values come from environment variables passed to the
container with `-e`. If the variables are absent, Maven substitutes the literal string
`${env.NEXUS_USER}`, which causes a 401 from Nexus during `deploy` — a clear signal
that credentials were not provided rather than a silent failure.

`settings.xml` also declares the local repository path explicitly
(`/root/.m2/repository`) so it is predictable when mounting a host cache volume.

### Sanity check

The final `RUN` step before `WORKDIR` executes:

```bash
java -version && go version && mvn --version && git --version
```

This runs at **image build time**, not at container startup. If any tool failed to
install correctly — wrong path, corrupt download, missing symlink — the image build
fails immediately with a clear error. This prevents silently distributing an image where
`go` or `mvn` is not on `PATH`.

---

## Building the Image

### Standard build

Run from the repository root. The build context is the image directory; only the three
files in that directory (`Containerfile`, `Dockerfile`, `settings.xml`) are sent to the
daemon.

**Podman (preferred):**
```bash
podman build \
  -t ubi8-jdk25-go126-mvn:3.9.9 \
  -f container/build/ubi8-jdk25-go126-mvn/Containerfile \
  container/build/ubi8-jdk25-go126-mvn/
```

**Docker:**
```bash
docker build \
  -t ubi8-jdk25-go126-mvn:3.9.9 \
  container/build/ubi8-jdk25-go126-mvn/
```

Docker uses `Dockerfile` by default; no `-f` flag is needed.

### Overriding tool versions

Pass `--build-arg` to substitute a different version without editing the file. This is
how future image variants are created — build with a different JDK, tag accordingly, and
keep the `Containerfile` unchanged.

```bash
# Build a JDK 21 variant
podman build \
  --build-arg JAVA_MAJOR=21 \
  -t ubi8-jdk21-go126-mvn:3.9.9 \
  -f container/build/ubi8-jdk25-go126-mvn/Containerfile \
  container/build/ubi8-jdk25-go126-mvn/

# Build with a newer Maven
podman build \
  --build-arg MAVEN_VERSION=3.9.10 \
  -t ubi8-jdk25-go126-mvn:3.9.10 \
  -f container/build/ubi8-jdk25-go126-mvn/Containerfile \
  container/build/ubi8-jdk25-go126-mvn/
```

When producing a new JDK/Go combination for a genuinely different toolchain, create a
new subdirectory under `container/build/` with the matching name and copy the three
files into it, updating the `ARG` defaults. This preserves the previous image definition
in source control.

### Windows (Podman Desktop / Podman machine)

On Windows, use the full path to the Podman executable if it is not on `PATH`:

```powershell
$podman = "C:\Users\<username>\AppData\Local\Programs\Podman\podman.exe"
& $podman build `
  -t ubi8-jdk25-go126-mvn:3.9.9 `
  -f "container\build\ubi8-jdk25-go126-mvn\Containerfile" `
  "container\build\ubi8-jdk25-go126-mvn\"
```

---

## Testing the Image

### Step 1 — Verify tool versions

Confirm that each tool is present, on `PATH`, and at the expected version:

```bash
podman run --rm ubi8-jdk25-go126-mvn:3.9.9 \
  bash -c "java -version && go version && mvn --version && git --version"
```

Expected output (versions may show a later patch):

```
openjdk version "25.0.3" 2026-04-21 LTS
OpenJDK Runtime Environment Temurin-25.0.3+9 ...
go version go1.26.3 linux/amd64
Apache Maven 3.9.9 ...
  Java version: 25.0.3, vendor: Eclipse Adoptium ...
git version 2.43.7
```

### Step 2 — Build a Go module (quick smoke test)

Mount the repository and build `health-probe-go`, which has no Java dependencies and
completes in under a minute. This confirms that Go cross-compilation works inside the
container and that Maven can resolve plugins from central.

```bash
podman run --rm \
  -v "$(pwd)":/workspace:Z \
  -w /workspace \
  ubi8-jdk25-go126-mvn:3.9.9 \
  mvn clean package -pl health-probe-go --also-make
```

Expected: `BUILD SUCCESS`. Two binaries appear in `health-probe-go/target/`:
`health-probe-go-linux-amd64` and `health-probe-go-windows-amd64.exe`.

### Step 3 — Build the full reactor

Mount Maven and Go caches to avoid re-downloading on repeat runs. Supply placeholder
credentials (not needed for `package`; only `deploy` contacts Nexus):

```bash
podman run --rm \
  -v "$(pwd)":/workspace:Z \
  -v "${HOME}/.m2/repository":/root/.m2/repository:Z \
  -v "${HOME}/go/pkg/mod":/root/go/pkg/mod:Z \
  -e NEXUS_USER=unused \
  -e NEXUS_PASSWORD=unused \
  -w /workspace \
  ubi8-jdk25-go126-mvn:3.9.9 \
  mvn clean package
```

**Windows PowerShell:**
```powershell
& $podman run --rm `
  -v "c:\src\github\marche:/workspace:Z" `
  -v "$env:USERPROFILE\.m2\repository:/root/.m2/repository:Z" `
  -v "$env:USERPROFILE\go\pkg\mod:/root/go/pkg/mod:Z" `
  -e NEXUS_USER=unused `
  -e NEXUS_PASSWORD=unused `
  -w /workspace `
  ubi8-jdk25-go126-mvn:3.9.9 `
  mvn clean package
```

Expected: all modules reach `BUILD SUCCESS`. On the first run without warm caches, Maven
downloads plugins from Maven Central (~200–400 MB depending on which modules have never
been built before). Subsequent runs using the mounted cache complete significantly faster.

### Step 4 — Interactive shell (debugging)

Drop into an interactive shell to inspect the environment or debug a failing build step:

```bash
podman run --rm -it \
  -v "$(pwd)":/workspace:Z \
  -e NEXUS_USER=ci \
  -e NEXUS_PASSWORD=secret \
  -w /workspace \
  ubi8-jdk25-go126-mvn:3.9.9
```

Inside the shell, standard diagnostics:

```bash
# Confirm all tools are on PATH
which java go mvn git

# Check environment variables set by the image
env | grep -E 'JAVA|GO|MAVEN'

# Inspect Maven's resolved settings
mvn help:effective-settings

# Verify Maven sees the correct JDK
mvn --version

# Check Go can cross-compile to Linux and Windows
GOOS=linux  GOARCH=amd64 CGO_ENABLED=0 go build -o /tmp/test-linux  ./gamma-go
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -o /tmp/test-win.exe ./gamma-go
```

---

## Known Warnings

### Maven / JDK 25 restricted method warnings

```
WARNING: A restricted method in java.lang.System has been called
WARNING: java.lang.System::load has been called by org.fusesource.jansi.internal.JansiLoader
```

These warnings are emitted by Maven's JANSI console-coloring library when it loads a
native library via `System.load()`. JDK 25 tightened access to this method. The warnings
are informational — Maven functions correctly. They will disappear when JANSI releases a
version that uses the `--enable-native-access` mechanism. To suppress them today, add
the following to `MAVEN_OPTS`:

```
--enable-native-access=ALL-UNNAMED
```

This is not set by default in the image because the flag did not exist in older JDKs and
would break the image if `ARG JAVA_MAJOR` is overridden to a JDK version below 17.

### UBI subscription warnings

```
This system is not registered with an entitlement server.
```

UBI 8 images can use the Red Hat Universal Base Image repositories without an active
subscription. The warning is cosmetic and does not affect package installation.
