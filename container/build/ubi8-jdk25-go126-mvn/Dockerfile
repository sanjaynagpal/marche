# =============================================================================
# Marche build image
#
# Base   : RHEL 8 UBI 8.10-1778062799
# JDK    : 25 (Eclipse Temurin)   — build runtime; cross-compiles to targets 8–21
# Go     : 1.26.3                 — gamma-go service + health-probe-go tool
# Maven  : 3.9.9                  — orchestrates the full reactor
# Git    : from UBI 8 yum repos
#
# Image naming convention:  ubi8-jdk{major}-go{minor}-mvn
# This image:               ubi8-jdk25-go126-mvn
#
# Build (Docker):
#   docker build -t ubi8-jdk25-go126-mvn:3.9.9 \
#     container/build/ubi8-jdk25-go126-mvn/
#
# Build (Podman — use Containerfile instead for native support):
#   podman build -t ubi8-jdk25-go126-mvn:3.9.9 \
#     -f Containerfile \
#     container/build/ubi8-jdk25-go126-mvn/
#
# Run (mount repo, cache volumes, pass Nexus credentials):
#   podman run --rm -it \
#     -v "$(pwd)":/workspace:Z \
#     -v /host/m2-cache:/root/.m2/repository:Z \
#     -v /host/go-cache:/root/go/pkg/mod:Z \
#     -e NEXUS_USER=ci \
#     -e NEXUS_PASSWORD=secret \
#     -w /workspace \
#     ubi8-jdk25-go126-mvn:3.9.9 \
#     mvn clean package
# =============================================================================

FROM registry.access.redhat.com/ubi8/ubi:8.10-1778062799

LABEL org.opencontainers.image.title="ubi8-jdk25-go126-mvn" \
      org.opencontainers.image.description="Marche CI build image: UBI 8 + JDK 25 + Go 1.26.3 + Maven 3.9.9 + Git" \
      org.opencontainers.image.vendor="marche" \
      java.version="25" \
      go.version="1.26.3" \
      maven.version="3.9.9"

# ── Overridable at build time ─────────────────────────────────────────────────
ARG JAVA_MAJOR=25
ARG GO_VERSION=1.26.3
ARG MAVEN_VERSION=3.9.9

# ── System packages ───────────────────────────────────────────────────────────
# findutils provides 'find'; which is used by Maven toolchain probing.
RUN yum install -y \
        curl \
        tar \
        gzip \
        git \
        findutils \
        which \
    && yum clean all \
    && rm -rf /var/cache/yum

# ── JDK 25 — Eclipse Temurin (Adoptium) ──────────────────────────────────────
# The Adoptium API returns the latest GA binary for the requested major version.
# Pin to a specific release by replacing the URL with a versioned GitHub release
# asset URL when the build must be fully reproducible.
RUN curl -fL \
        "https://api.adoptium.net/v3/binary/latest/${JAVA_MAJOR}/ga/linux/x64/jdk/hotspot/normal/eclipse" \
        -o /tmp/jdk.tar.gz \
    && mkdir -p /opt/java \
    && tar -xzf /tmp/jdk.tar.gz -C /opt/java --strip-components=1 \
    && rm /tmp/jdk.tar.gz

ENV JAVA_HOME=/opt/java
ENV PATH=${JAVA_HOME}/bin:${PATH}

# ── Go 1.26.3 ─────────────────────────────────────────────────────────────────
# go.dev/dl serves official releases at stable, content-addressed URLs.
RUN curl -fL \
        "https://go.dev/dl/go${GO_VERSION}.linux-amd64.tar.gz" \
        -o /tmp/go.tar.gz \
    && tar -xzf /tmp/go.tar.gz -C /usr/local \
    && rm /tmp/go.tar.gz

ENV GOROOT=/usr/local/go
ENV GOPATH=/root/go
ENV PATH=${GOROOT}/bin:${GOPATH}/bin:${PATH}

# ── Apache Maven 3.9.9 ────────────────────────────────────────────────────────
# archive.apache.org is the permanent archive and does not 404 on older releases.
RUN curl -fL \
        "https://archive.apache.org/dist/maven/maven-3/${MAVEN_VERSION}/binaries/apache-maven-${MAVEN_VERSION}-bin.tar.gz" \
        -o /tmp/maven.tar.gz \
    && tar -xzf /tmp/maven.tar.gz -C /opt \
    && ln -s /opt/apache-maven-${MAVEN_VERSION} /opt/maven \
    && rm /tmp/maven.tar.gz

ENV MAVEN_HOME=/opt/maven
ENV PATH=${MAVEN_HOME}/bin:${PATH}

# Tune JVM heap for Maven; adjust for available CI agent memory.
ENV MAVEN_OPTS="-Xms256m -Xmx2g -XX:+UseG1GC"

# ── Maven settings ────────────────────────────────────────────────────────────
# settings.xml references ${env.NEXUS_USER} / ${env.NEXUS_PASSWORD} — no
# credentials are stored in the image. See settings.xml for runtime options.
COPY settings.xml /root/.m2/settings.xml

# ── Cache directories ─────────────────────────────────────────────────────────
# Create mount points so bind-mount volume permissions are correct at run time.
RUN mkdir -p /root/.m2/repository \
             /root/go/pkg/mod \
             /root/.cache/go-build

# ── Sanity check (executed at image build time, not at container startup) ─────
RUN echo "=== java ===" && java -version \
    && echo "=== go ===" && go version \
    && echo "=== mvn ===" && mvn --version \
    && echo "=== git ===" && git --version

WORKDIR /workspace

CMD ["/bin/bash"]
