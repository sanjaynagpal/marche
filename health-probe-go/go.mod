module marche/health-probe-go

go 1.26.3

// No external dependencies — standard library only.
// CGO_ENABLED=0 is set by the Maven build so cross-compilation works
// for both linux/amd64 and windows/amd64 from any host platform.
