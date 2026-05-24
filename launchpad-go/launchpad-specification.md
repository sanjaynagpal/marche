# Launchpad — Specification

> **Status:** Draft v0.8 — shared-script support: start/stop command strings now accept optional arguments (`bin/runCMD.sh start` / `bin/runCMD.sh stop`).

---

## 1. Purpose

Launchpad is a lightweight process supervisor and console written in Go. Its sole responsibility is to manage the lifecycle of services that are installed side-by-side under a shared root directory (`$COMPONENT_ROOT`). It provides a single pane of glass for operators to see which services are running, start any that are stopped, and stop any that are running — without requiring a full process-management platform such as systemd, Supervisor, or Kubernetes.

Launchpad supports two usage modes:

- **Interactive mode** (default) — a full-screen terminal console with a live status table and a command prompt.
- **CLI mode** — non-interactive subcommands suitable for shell scripts and automation.

Launchpad follows the same flat Go layout used by `gamma-go`. It has **no external runtime dependencies** and ships as a single self-contained binary.

---

## 2. Scope and Non-Goals

**In scope**

- Discovering all services installed under `$COMPONENT_ROOT`.
- Reporting the live status of each service (RUNNING / STOPPED).
- Starting a stopped service as a background process.
- Stopping a running service gracefully, with a configurable timeout before a forced kill.
- Persisting a PID file per service so state survives launchpad restarts.
- Logging per-service stdout/stderr to an append-mode log file inside the service folder.
- Cross-platform operation: Linux, macOS, and Windows (amd64).
- Non-interactive CLI subcommands for scripting and automation.

**Out of scope (v1)**

- Automatic restart / supervisor-style watchdog behaviour.
- Health-check polling or alerting.
- Service dependency ordering.
- Remote control (HTTP API, gRPC).
- Configuration of services themselves (environment variables, JVM flags, etc.).
- Log rotation.

---

## 3. Environment

| Variable | Default | Description |
|---|---|---|
| `COMPONENT_ROOT` | *(required)* | Absolute path to the directory that contains all installed service folders. |
| `LAUNCHPAD_ENV` | `dev` | Forwarded to each service process on start so it selects the correct config profile. |
| `LAUNCHPAD_STOP_TIMEOUT` | `10` | Seconds to wait for graceful shutdown before a forced kill. |
| `LAUNCHPAD_START_TIMEOUT` | `30` | Seconds to wait for a custom service's start script to write its PID file. |

---

## 4. Service Discovery

### 4.1 Installed Service Layout

Services arrive at `$COMPONENT_ROOT` already fully installed — built and extracted from the Maven assembly archives. Each service folder contains:

```
$COMPONENT_ROOT/
├── launchpad-registry.yaml     ← central registry (owned by launchpad)
├── alpha-eight/
│   ├── bin/                ← executables (pre-installed)
│   │   ├── run.sh          ← Unix launch script
│   │   ├── run.ps1         ← PowerShell launch script (Windows-preferred)
│   │   └── lib/            ← fat JAR and dependency JARs
│   ├── cfg/                ← YAML config files (pre-installed)
│   ├── logs/               ← created by launchpad on first start
│   └── .launchpad.pid      ← written by launchpad; absent when STOPPED
├── gamma-go/
│   ├── bin/
│   │   ├── run.sh
│   │   ├── run.ps1
│   │   └── lib/
│   └── cfg/
├── legacy-svc/             ← custom service: registered via launchpad.yaml
│   ├── launchpad.yaml      ← per-service override (highest precedence)
│   ├── bin/
│   │   ├── runCMD.sh       ← non-standard script names
│   │   ├── runCMD.ps1
│   │   ├── stopCMD.sh
│   │   ├── stopCMD.ps1
│   │   └── lib/
│   └── cfg/
├── polaris-eleven/             ← aggregator: no bin/ at this level
│   ├── polaris-havok/
│   │   ├── bin/
│   │   │   ├── run.sh
│   │   │   ├── run.ps1
│   │   │   └── lib/
│   │   └── cfg/
│   └── polaris-wanda/
│       ├── bin/
│       │   ├── run.sh
│       │   ├── run.ps1
│       │   └── lib/
│       └── cfg/
```

### 4.2 Three-Tier Detection

Launchpad discovers services by scanning the **immediate** children of `$COMPONENT_ROOT`. A child directory qualifies as a Launchpad-managed service if it satisfies **at least one** of three detection signals, evaluated in precedence order:

| Tier | Signal | Controlled by |
|---|---|---|
| 1 (highest) | `<service-folder>/launchpad.yaml` exists | The service team (per-service override) |
| 2 | Folder name appears in `$COMPONENT_ROOT/launchpad-registry.yaml` | Launchpad operators (central registry) |
| 3 (lowest) | `bin/run.ps1` or `bin/run.sh` exists in the service folder | Standard Launchpad convention |

The first matching tier wins — higher tiers are not consulted once a match is found. Tier 3 uses the following platform-specific script priority:

| Platform | First choice | Fallback |
|---|---|---|
| Windows | `bin/run.ps1` (via `pwsh -File`) | `bin/run.bat` |
| Linux / macOS | `bin/run.sh` (via `bash`) | — |

Sub-directories are **not** recursed into beyond one level. Polaris-style aggregators (folders qualifying by none of the three signals but containing qualifying sub-folders) are transparently expanded: each qualifying sub-folder is registered as an independent service.

### 4.3 Service Metadata

After a service is discovered, launchpad resolves its display name and port using a three-level fallback cascade:

1. **`cfg/application-${LAUNCHPAD_ENV}.yaml`** — the primary source for all standard services. Launchpad reads `launchpad.module` (name) and `server.port` from this file.
2. **Registration metadata** — if the cfg file is absent (common for legacy services) or if the fields are blank, launchpad falls back to `name` and `port` declared in the service's `launchpad.yaml` or central registry entry.
3. **Folder name / empty** — ultimate fallback: the folder name is used as the display name; port is left blank and shown as `-`.

Legacy services may have no `cfg/` directory at all. In that case steps 2 and 3 ensure a usable name and port are still shown without error.

### 4.4 Per-Service Registration (`launchpad.yaml`)

Services that pre-date Launchpad, or that use non-standard script names, can register their start script, stop script, and PID file by placing a `launchpad.yaml` file at the service root. This file takes **highest precedence** and overrides any entry in the central registry for the same folder. Launchpad reads this file during discovery and delegates start, stop, and status tracking entirely to what the service declares.

#### File location

```
<service-folder>/launchpad.yaml
```

#### Format

```yaml
service:                      # optional — metadata for services without a cfg YAML
  name: My Legacy Service     # display name; overrides folder name when cfg YAML is absent
  port: "9090"                # port; shown in the status table when cfg YAML is absent
start:
  unix: bin/runCMD.sh         # path relative to service root, used on Linux/macOS
  windows: bin/runCMD.ps1     # path relative to service root, used on Windows
stop:
  unix: bin/stopCMD.sh
  windows: bin/stopCMD.ps1
pid:
  file: my-service.pid        # filename relative to service root
```

**Shared-script pattern** — when one script handles both start and stop via an argument, declare the same script path in both `start` and `stop` entries, appending the appropriate argument after the path:

```yaml
start:
  unix: bin/runCMD.sh start
  windows: bin/runCMD.ps1 start
stop:
  unix: bin/runCMD.sh stop
  windows: bin/runCMD.ps1 stop
pid:
  file: my-service.pid
```

Everything after the first whitespace on the value line is passed as arguments to the script. The first token is always the script path (relative to the service root). Multiple arguments are supported (`bin/manage.sh start --env prod`).

All paths are relative to the service root folder. The `pid.file` entry is **required**; a `launchpad.yaml` without it is ignored. The `service:` block is optional — omit it when `cfg/application-${LAUNCHPAD_ENV}.yaml` already provides the name and port. Entries for the platform launchpad is not running on are ignored.

#### Behaviour differences from standard services

| Aspect | Standard service | Custom service |
|---|---|---|
| Detection signal | Tier 3: `bin/run.ps1` / `bin/run.sh` | Tier 1: `launchpad.yaml` present; or Tier 2: entry in `launchpad-registry.yaml` |
| Start | Launchpad launches script as detached background child; writes PID itself | Launchpad runs start script synchronously (it daemonises); polls for `pid.file` to appear (up to `LAUNCHPAD_START_TIMEOUT` seconds) |
| Stop | Launchpad kills by PID (SIGTERM → SIGKILL / `Stop-Process`) | Launchpad runs stop script synchronously; polls until `pid.file` disappears or process exits |
| PID file | `.launchpad.pid` written and removed by launchpad | Declared `pid.file` written and removed by the service's own scripts; launchpad never touches it |
| Stale PID file cleanup | Launchpad removes `.launchpad.pid` when the PID is no longer alive | Launchpad leaves the declared PID file intact; the stop script owns cleanup |

---

### 4.5 Central Registry (`launchpad-registry.yaml`)

Operators who cannot add files to individual service folders (e.g., because those folders are owned by an independent installer) can register services centrally via `$COMPONENT_ROOT/launchpad-registry.yaml`. This file is owned and maintained by Launchpad operators and is consulted when a service folder has no `launchpad.yaml` of its own.

#### File location

```
$COMPONENT_ROOT/launchpad-registry.yaml
```

#### Format

The registry uses the standard two-level YAML format. Each service is a top-level section keyed by its **folder name** (not module name), with dotted field names as sub-keys:

```yaml
legacy-svc:
  name: Legacy Service        # optional — display name when cfg YAML is absent
  port: "9000"                # optional — port when cfg YAML is absent
  start.unix: bin/runCMD.sh
  start.windows: bin/runCMD.ps1
  stop.unix: bin/stopCMD.sh
  stop.windows: bin/stopCMD.ps1
  pid.file: legacy-svc.pid

another-old-svc:              # no name/port — folder name shown, port shown as -
  start.unix: bin/start.sh
  start.windows: bin/start.ps1
  stop.unix: bin/stop.sh
  stop.windows: bin/stop.ps1
  pid.file: another-old-svc.pid

shared-script-svc:            # one script handles start and stop via an argument
  start.unix: bin/runCMD.sh start
  start.windows: bin/runCMD.ps1 start
  stop.unix: bin/runCMD.sh stop
  stop.windows: bin/runCMD.ps1 stop
  pid.file: shared-script-svc.pid
```

All script paths are relative to the service root folder. The `pid.file` entry is **required**; a service without it is silently ignored. `name` and `port` are optional — they are used only when `cfg/application-${LAUNCHPAD_ENV}.yaml` is absent or does not supply the field.

**Shared-script pattern** — the value of any `start.*` or `stop.*` field may include arguments after the script path. The first whitespace-separated token is the script path; everything following is passed as arguments when launchpad invokes the script (`pwsh -File runCMD.ps1 start` on Windows, `bash runCMD.sh start` on Unix). Entries for the platform launchpad is not running on are ignored.

#### Precedence

| Condition | Resolution |
|---|---|
| Service folder has its own `launchpad.yaml` | `launchpad.yaml` wins; registry entry is ignored |
| Service folder appears in registry only | Registry entry is used |
| Service folder has neither but has `bin/run.*` | Standard Tier 3 detection applies |
| Service appears in both `launchpad.yaml` and registry | `launchpad.yaml` always wins (Tier 1 > Tier 2) |

#### Service qualification via registry

A service folder qualifies as a Launchpad-managed service (Tier 2) if and only if its **folder name** (not module name) appears as a section in `launchpad-registry.yaml` with a valid `pid.file` declaration. The folder does not need a `bin/run.*` script — the registry declaration is sufficient.

---

## 5. Status Detection

A service is in exactly one of two states at any time:

| State | Meaning |
|---|---|
| **RUNNING** | A live process is associated with the service via its PID file. |
| **STOPPED** | No live process is associated with the service. |

### 5.1 PID File

Every service has exactly one PID file. Its path is fixed at discovery time and stored in `ServiceEntry.PIDFile`:

| Service type | PID file path | Written by | Removed by |
|---|---|---|---|
| Standard | `<service-folder>/.launchpad.pid` | Launchpad (after `cmd.Start()`) | Launchpad (on stop or stale detection) |
| Custom | `<service-folder>/<pid.file>` as declared in `launchpad.yaml` | The service's start script | The service's stop script |

On every status refresh launchpad:

1. Reads the PID from `ServiceEntry.PIDFile` (returns STOPPED if file absent or unreadable).
2. Probes whether a process with that PID exists:
   - **Linux / macOS**: `syscall.Kill(pid, 0)` — signal 0 checks existence without delivering a signal. `EPERM` (process exists, owned by another user) is treated as alive.
   - **Windows**: `OpenProcess(PROCESS_QUERY_LIMITED_INFORMATION)` + `GetExitCodeProcess`; `STILL_ACTIVE` (259) confirms liveness.
3. Process alive → **RUNNING**; otherwise → **STOPPED**.

**Stale PID file handling**: for standard services, launchpad removes `.launchpad.pid` when the stored PID is no longer alive. For custom services, the PID file is owned by the service's scripts — launchpad never deletes it.

---

## 6. User Interface

### 6.1 Interactive Mode (default)

When invoked with no subcommand, launchpad enters interactive mode: it clears the screen, renders the status table, and drops into a command prompt. No external TUI library is used; the interface is built with ANSI escape sequences via the Go standard library only — consistent with the `gamma-go` no-external-deps policy.

#### Status Table

```
Launchpad   root=/opt/launchpad   env=dev
───────────────────────────────────────────────────────────────
 #   Module              Status    Port   PID
─── ─────────────────── ──────── ────── ──────
 1  alpha-eight         RUNNING   8080   14321
 2  gamma-go            STOPPED   8086   —
 3  kepler-eleven       RUNNING   8084   14398
 4  kepler-twenty-one   STOPPED   8085   —
 5  orion-eleven        RUNNING   8081   14356
 6  polaris-havok       STOPPED   8090   —
 7  polaris-wanda       STOPPED   8091   —
 8  sirius-seventeen    RUNNING   8082   14372
 9  vega-twenty-one     STOPPED   8083   —
───────────────────────────────────────────────────────────────
Commands: [s]tart <#>  [k]ill <#>  [r]efresh  [q]uit
>
```

- **RUNNING** is rendered in green; **STOPPED** in yellow.
- The table is sorted alphabetically by module name.
- Column widths adapt to the longest module name discovered.

#### Interactive Commands

Numbers refer to the `#` column. Commands are case-insensitive.

| Command | Short | Description |
|---|---|---|
| `start <#>` | `s <#>` | Start the service at row `#`. No-op if already RUNNING. |
| `kill <#>` | `k <#>` | Stop the service at row `#`. No-op if already STOPPED. |
| `refresh` | `r` | Re-scan all statuses and redraw the table. |
| `quit` | `q` | Exit launchpad. Running services are **not** stopped. |

Invalid input (unknown command, out-of-range number) prints a one-line error and re-shows the prompt.

### 6.2 CLI Mode (non-interactive)

When invoked with a subcommand, launchpad runs the operation, prints plain-text output to stdout, and exits. Designed for shell scripts and automation.

```
launchpad <subcommand> [argument]
```

| Subcommand | Argument | Description |
|---|---|---|
| `status` | — | Print the status of all discovered services, one per line. |
| `start <name>` | module name or folder name | Start a specific service. |
| `stop <name>` | module name or folder name | Stop a specific service. |
| `start-all` | — | Start every STOPPED service. |
| `stop-all` | — | Stop every RUNNING service. |
| `init` | — | Scan for legacy folders and seed `launchpad-registry.yaml`. |

#### `status` Output Format

Each line follows a stable, parseable format:

```
alpha-eight    RUNNING  8080  14321
gamma-go       STOPPED  8086  -
kepler-eleven  RUNNING  8084  14398
```

Fields are tab-separated: `<module-name> TAB <state> TAB <port> TAB <pid>`.  
PID is `-` when STOPPED.

#### Exit Codes

| Code | Meaning |
|---|---|
| `0` | Success (or all services in expected state). |
| `1` | General error (bad arguments, `COMPONENT_ROOT` not set, etc.). |
| `2` | Service not found. |
| `3` | Operation failed (start or stop did not complete successfully). |

#### Examples

```sh
# Print status of all services
launchpad status

# Start a specific service
launchpad start alpha-eight

# Stop a specific service
launchpad stop gamma-go

# Start everything (idempotent — already-running services are skipped)
launchpad start-all

# Seed the central registry for legacy service folders, then fine-tune the file
launchpad init

# Use in a script: check if a service is running
launchpad status | grep "^alpha-eight" | awk '{print $2}'
```

---

## 7. Registry Initialization (`launchpad init`)

`launchpad init` is a one-shot bootstrap command for environments that already have legacy service folders installed but no central registry. It scans `$COMPONENT_ROOT`, identifies every folder that does not yet qualify under any detection tier, infers registration fields from the folder's contents, and writes the results into `$COMPONENT_ROOT/launchpad-registry.yaml`.

### 7.1 Discovery of Legacy Candidates

A folder is treated as a legacy candidate when **all** of the following are true:

1. It does not already qualify under Tier 1 (no `launchpad.yaml`).
2. It does not already qualify under Tier 2 (not yet in the registry).
3. It does not already qualify under Tier 3 (no standard `bin/run.*` script).
4. It contains a `bin/` subdirectory (indicating some kind of launch infrastructure).

Aggregator expansion (one level deep) mirrors the `DiscoverServices` logic, so sub-services inside Polaris-style parent folders are also discovered.

### 7.2 Script Inference Heuristics

For each candidate, `init` scans `bin/` for `*.sh` (Unix) and `*.ps1` (Windows) scripts and applies the following heuristics:

| Script role | Selection rule |
|---|---|
| Start (Unix / Windows) | Prefer name containing `run` or `start` (case-insensitive) that does not also contain `stop`. If exactly one non-stop script exists, use it unconditionally. |
| Stop (Unix / Windows) | Require name containing `stop`. Left blank if none found. |

When a script cannot be identified with confidence (e.g., two equally-named start candidates), the field is left blank and written as a commented-out `# TODO` placeholder in the registry.

### 7.3 PID File Inference

1. If any `*.pid` file exists directly in the service root, its name is used.
2. Otherwise the fallback is `<foldername>.pid`.

### 7.4 Metadata Inference

If `cfg/application-${LAUNCHPAD_ENV}.yaml` is present and contains `launchpad.module` and/or `server.port`, those values are written into the registry entry's `name` and `port` fields. If the cfg file is absent, those fields are omitted from the generated entry (the operator adds them manually if needed).

### 7.5 Output

`init` prints a human-readable summary of every candidate it processes, showing which fields were detected and which need manual attention:

```
Found 3 unregistered legacy folder(s):

  legacy-named  (legacy-named)
    start (unix):       bin/runCMD.sh
    start (windows):    bin/runCMD.ps1
    stop (unix):        bin/stopCMD.sh
    stop (windows):     bin/stopCMD.ps1
    pid file:           legacy-named.pid
    name:               LegacyNamedService
    port:               9001

  legacy-partial  (legacy-partial)
    start (unix):       bin/startApp.sh
    start (windows):    bin/startApp.ps1
    stop (unix):        — not detected, set manually
    stop (windows):     — not detected, set manually
    pid file:           legacy-partial.pid

  legacy-ambiguous  (legacy-ambiguous)
    start (unix):       — not detected, set manually
    ...
```

### 7.6 Registry File Behavior

| Condition | Result |
|---|---|
| Registry file does not exist | Created with a field-reference header block |
| Registry file already exists | New entries appended after a `# --- entries added by launchpad init ---` separator; existing entries untouched |
| Folder already registered | Skipped — existing entry is never overwritten |
| No legacy candidates found | Prints a notice; registry file unchanged |

`init` is **idempotent**: running it again after all folders are registered prints "No unregistered legacy service folders found" and exits without modifying the registry.

### 7.7 Workflow

```
1. launchpad init          # seed the registry
2. $EDITOR $COMPONENT_ROOT/launchpad-registry.yaml
                           # fill in # TODO lines, add name/port if needed
3. launchpad status        # verify all services are discovered
4. launchpad start-all     # bring them up
```

---

## 8. Start Sequence

Applies to both interactive (`start <#>`) and CLI (`launchpad start <name>`) modes. The path taken depends on whether the service is standard or custom.

### 7.1 Standard Service

1. Assemble the child environment: inherit current env, override `LAUNCHPAD_ENV` and set `LAUNCHPAD_CONFIG_DIR` to `<service-folder>/cfg`.
2. Create `<service-folder>/logs/` if absent.
3. Open (append) `<service-folder>/logs/stdout.log` and `<service-folder>/logs/stderr.log`.
4. Launch `bin/run.ps1` (Windows via `pwsh -NonInteractive -File`) or `bin/run.sh` (Unix via `bash`) as a **detached** background process with stdout/stderr redirected to those log files. Detach from the parent process group so the child survives launchpad exiting (`Setpgid: true` on Unix; `CREATE_NEW_PROCESS_GROUP` on Windows).
5. Write the child PID to `<service-folder>/.launchpad.pid`.
6. Refresh status.

### 7.2 Custom Service

1. Assemble the child environment: same as standard (step 1 above).
2. Run the declared start script **synchronously** (`cmd.Run()`):
   - Windows: `pwsh -NonInteractive -File <start.windows>`
   - Unix: `bash <start.unix>`
   
   The script is expected to daemonise the service (fork, background itself) and exit quickly. Launchpad blocks until the script process exits.
3. Poll `<service-folder>/<pid.file>` every 500 ms until it appears and contains a valid PID, or until `LAUNCHPAD_START_TIMEOUT` seconds elapse. Return an error on timeout.
4. Read the PID from the file. The file was written by the start script; launchpad does not write it.
5. Refresh status.

---

## 8. Stop Sequence

Applies to both interactive (`kill <#>`) and CLI (`launchpad stop <name>`) modes.

### 8.1 Standard Service

1. Read the PID from `<service-folder>/.launchpad.pid`.
2. Send the graceful stop signal:
   - **Linux / macOS**: `SIGTERM` via `syscall.Kill`.
   - **Windows**: `pwsh -Command "Stop-Process -Id <pid>"`.
3. Poll for process exit every 500 ms up to `LAUNCHPAD_STOP_TIMEOUT` seconds (default 10).
4. If still alive after the timeout, force-kill:
   - **Linux / macOS**: `SIGKILL` via `syscall.Kill`.
   - **Windows**: `pwsh -Command "Stop-Process -Id <pid> -Force"`.
5. Remove `<service-folder>/.launchpad.pid`.
6. Refresh status.

### 8.2 Custom Service

1. Snapshot the current PID from `<service-folder>/<pid.file>` (used for liveness polling after the script runs).
2. Run the declared stop script **synchronously** (`cmd.Run()`):
   - Windows: `pwsh -NonInteractive -File <stop.windows>`
   - Unix: `bash <stop.unix>`
   
   The script is responsible for terminating the service and removing the PID file.
3. Poll every 500 ms until either `<pid.file>` no longer exists **or** the snapshotted PID is no longer alive — whichever comes first — up to `LAUNCHPAD_STOP_TIMEOUT` seconds. Return an error on timeout.
4. Refresh status.

---

## 9. Logging

Per-service process output is captured into append-mode log files:

```
<service-folder>/logs/stdout.log   — service standard output
<service-folder>/logs/stderr.log   — service standard error
```

Successive starts accumulate history in the same files. No rotation is performed in v1.

Launchpad's own diagnostic messages (errors, warnings) are written to its stderr. In interactive mode they appear briefly below the table before the next refresh. In CLI mode they appear on stderr so stdout remains parseable.

---

## 10. Exit Behaviour

- Running services are **left running** when launchpad exits. They are independent OS processes launched in their own process groups.
- If launchpad receives SIGINT or SIGTERM (e.g., Ctrl-C in interactive mode), it exits cleanly, restoring the terminal cursor and prompt. No services are touched.

---

## 11. Module File Layout

Launchpad follows the flat Go layout established by `gamma-go`, with platform-specific files split using Go build tags:

```
launchpad-go/
├── launchpad-specification.md    ← this file
├── go.mod
├── main.go                       ← arg parsing; interactive vs CLI dispatch
├── discover.go                   ← COMPONENT_ROOT scan, aggregator expansion,
│                                    YAML config parsing, ServiceEntry struct
├── status.go                     ← PID file read/write; refresh all statuses
├── status_unix.go                ← kill-0 liveness probe  (//go:build !windows)
├── status_windows.go             ← FindProcess liveness probe (//go:build windows)
├── process.go                    ← start/stop orchestration, log file setup
├── process_unix.go               ← bash exec, Setpgid, SIGTERM/SIGKILL
├── process_windows.go            ← pwsh exec, CREATE_NEW_PROCESS_GROUP, Stop-Process
├── display.go                    ← ANSI table renderer, color helpers
├── cli.go                        ← CLI subcommand handlers, tab-separated output
└── configuration/
    ├── application-dev.yaml
    ├── application-prod.yaml
    └── application-test.yaml
```

---

## 12. Implementation Plan

The feature set is now finalised. Implementation proceeds in six phases, each building on the last and independently testable by running launchpad against a synthetic `$COMPONENT_ROOT`.

---

### Phase 1 — Module Skeleton and Argument Routing

**Goal**: A compilable binary that knows whether it was invoked interactively or with a CLI subcommand.

**Files**: `go.mod`, `main.go`

**Tasks**:
1. Create `go.mod` — module path `launchpad/launchpad-go`, Go 1.26, no external dependencies.
2. In `main.go`, read `os.Args`:
   - No args → interactive mode (placeholder: print "interactive mode" and exit).
   - First arg matches a known subcommand → CLI mode (placeholder: print subcommand and exit).
   - Unknown arg → print usage to stderr, exit code 1.
3. Validate `COMPONENT_ROOT` is set and the directory exists; exit with a clear message if not.
4. Define a `Config` struct holding `ComponentRoot`, `Env`, and `StopTimeout` parsed from environment variables with defaults applied.

**Acceptance**: `go build ./...` succeeds; `launchpad` prints "interactive mode"; `launchpad status` prints "CLI: status".

---

### Phase 2 — Service Discovery

**Goal**: Populate a slice of `ServiceEntry` structs from `$COMPONENT_ROOT`.

**Files**: `discover.go`

**Tasks**:
1. Define `ServiceEntry`:
   ```go
   type ServiceEntry struct {
       Name       string // from launchpad.module or folder name
       FolderPath string // absolute path to service folder
       Port       string // from server.port or ""
       RunScript  string // absolute path to the selected launch script
   }
   ```
2. `DiscoverServices(root, env string) ([]ServiceEntry, error)`:
   - Read immediate children of `root` with `os.ReadDir`.
   - For each child directory:
     - Call `hasRunScript(dir)` — returns the resolved script path if `run.ps1` / `run.sh` / `run.bat` exists (platform-priority order).
     - If no script found, call `expandAggregator(dir)` — read one further level and collect qualifying sub-folders.
   - For each qualifying folder, call `loadServiceMeta(folderPath, env)` to fill `Name` and `Port` from `cfg/application-${env}.yaml`.
3. Sort results alphabetically by `Name`.
4. `loadServiceMeta` uses the same two-level YAML parser pattern from `gamma-go` (`loadConfig`).

**Acceptance**: Running against a synthetic `$COMPONENT_ROOT` with three service folders returns a correctly sorted slice with names and ports populated.

---

### Phase 3 — Status Detection

**Goal**: For any `ServiceEntry`, determine RUNNING or STOPPED via the PID file.

**Files**: `status.go`, `status_unix.go`, `status_windows.go`

**Tasks**:
1. Define `ServiceStatus`:
   ```go
   type ServiceStatus struct {
       Entry   ServiceEntry
       State   string // "RUNNING" or "STOPPED"
       PID     int    // 0 when STOPPED
   }
   ```
2. `PIDFilePath(entry ServiceEntry) string` — returns `<folder>/.launchpad.pid`.
3. `ReadPID(entry ServiceEntry) (int, error)` — reads and parses the PID file; returns 0 and no error if file absent.
4. `WritePID(entry ServiceEntry, pid int) error` — atomically writes PID to the PID file.
5. `RemovePID(entry ServiceEntry) error` — deletes the PID file.
6. `IsAlive(pid int) bool` — platform-specific:
   - `status_unix.go` (`//go:build !windows`): `syscall.Kill(pid, 0) == nil`.
   - `status_windows.go` (`//go:build windows`): `os.FindProcess` + `GetExitCodeProcess` == `STILL_ACTIVE`.
7. `CheckStatus(entry ServiceEntry) ServiceStatus` — orchestrates 3–6: read PID → if 0, STOPPED; if PID > 0 and `IsAlive`, RUNNING; else STOPPED and remove stale file.
8. `RefreshAll(entries []ServiceEntry) []ServiceStatus` — maps `CheckStatus` over all entries.

**Acceptance**: `RefreshAll` returns STOPPED for all entries in a fresh synthetic root; returns RUNNING after manually writing a real PID to a `.launchpad.pid` file.

---

### Phase 4 — Process Start and Stop

**Goal**: Start a STOPPED service as a detached background process; stop a RUNNING one gracefully.

**Files**: `process.go`, `process_unix.go`, `process_windows.go`

**Tasks**:
1. `EnsureLogsDir(entry ServiceEntry) error` — creates `<folder>/logs/` if absent.
2. `OpenLogFiles(entry ServiceEntry) (*os.File, *os.File, error)` — opens `logs/stdout.log` and `logs/stderr.log` in append+create mode.
3. `StartService(entry ServiceEntry, env string) (int, error)`:
   - Resolves launch script from `entry.RunScript`.
   - Builds `exec.Cmd`:
     - `process_unix.go`: `exec.Command("bash", entry.RunScript)` with `SysProcAttr{Setpgid: true}`.
     - `process_windows.go`: `exec.Command("pwsh", "-NonInteractive", "-File", entry.RunScript)` with `SysProcAttr{CreationFlags: CREATE_NEW_PROCESS_GROUP}`.
   - Sets `cmd.Env` = `os.Environ()` + `LAUNCHPAD_ENV=<env>` + `LAUNCHPAD_CONFIG_DIR=<folder>/cfg`.
   - Attaches stdout/stderr log files.
   - Calls `cmd.Start()`, writes PID, returns PID.
4. `StopService(entry ServiceEntry, timeoutSecs int) error`:
   - Reads PID; if 0, return nil (already stopped).
   - Sends graceful signal:
     - `process_unix.go`: `syscall.Kill(pid, syscall.SIGTERM)`.
     - `process_windows.go`: runs `pwsh -Command "Stop-Process -Id <pid>"`.
   - Polls `IsAlive` every 500 ms up to `timeoutSecs` seconds.
   - On timeout, force-kill:
     - `process_unix.go`: `syscall.Kill(pid, syscall.SIGKILL)`.
     - `process_windows.go`: `pwsh -Command "Stop-Process -Id <pid> -Force"`.
   - Removes PID file.

**Acceptance**: `StartService` on a stopped `gamma-go` service starts the Go HTTP server in the background and the PID file is written; `StopService` terminates it and removes the PID file.

---

### Phase 5 — Display and Interactive Loop

**Goal**: Full-screen interactive console with live status table and command prompt.

**Files**: `display.go`, `main.go` (interactive path)

**Tasks**:
1. `display.go` — `RenderTable(statuses []ServiceStatus, root, env string)`:
   - Clear screen: `\033[H\033[2J`.
   - Print header line with root and env.
   - Compute column widths dynamically from the longest module name.
   - Print separator lines using `─` (U+2500).
   - For each row: print index, name, state (green ANSI for RUNNING, yellow for STOPPED), port, PID.
   - Print command help line at the bottom.
2. `display.go` — `PrintError(msg string)` — prints a red one-line error message.
3. Interactive loop in `main.go`:
   - Loop: `RenderTable` → read line from stdin → parse command → dispatch → repeat.
   - `start <n>`: validate index, call `StartService`, call `RefreshAll`.
   - `kill <n>`: validate index, call `StopService`, call `RefreshAll`.
   - `refresh` / `r`: call `RefreshAll` (table redraws at top of loop).
   - `quit` / `q`: restore cursor, exit 0.
   - Unknown input: `PrintError`, re-render table.

**Acceptance**: Running `launchpad` against a live `$COMPONENT_ROOT` shows the table; typing `s 2` starts the second service; typing `k 2` stops it; typing `q` exits without disturbing other services.

---

### Phase 6 — CLI Subcommands

**Goal**: Non-interactive subcommands with stable stdout output and correct exit codes.

**Files**: `cli.go`

**Tasks**:
1. `RunStatus(statuses []ServiceStatus)` — prints tab-separated lines to stdout; exits 0.
2. `RunStart(statuses []ServiceStatus, name, env string) int` — finds entry by name (case-insensitive folder name or module name match):
   - Not found → stderr message, exit 2.
   - Already RUNNING → silent success, exit 0.
   - Call `StartService`; on error → stderr message, exit 3; on success → exit 0.
3. `RunStop(statuses []ServiceStatus, name string, timeoutSecs int) int` — same pattern as `RunStart` but calls `StopService`.
4. `RunStartAll` / `RunStopAll` — iterate over statuses, call start/stop for each in applicable state, collect errors; exit 3 if any operation failed, else 0.
5. Wire subcommand dispatch in `main.go` using a `switch` on `os.Args[1]`:
   ```
   "status"    → RunStatus
   "start"     → RunStart (requires os.Args[2])
   "stop"      → RunStop  (requires os.Args[2])
   "start-all" → RunStartAll
   "stop-all"  → RunStopAll
   default     → print usage, exit 1
   ```

**Acceptance**:  
- `launchpad status` exits 0 and prints one tab-separated line per service.  
- `launchpad start alpha-eight` starts the service or is a no-op if already running.  
- `launchpad start no-such-service` exits 2.  
- `launchpad start-all` starts every STOPPED service.

---

*End of specification draft v0.5*
