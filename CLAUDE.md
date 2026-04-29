# CLAUDE.md — g10k Codebase Guide

## Project Overview

g10k is a high-performance Go implementation of [r10k](https://github.com/puppetlabs/r10k) for deploying Puppet environments. It syncs Puppet control repositories from Git and resolves Puppetfile dependencies (Puppetlabs Forge modules and Git modules) using aggressive caching and parallelism.

Key differentiators from r10k:
- Written in Go for concurrency and speed
- Bounded goroutine pools for parallel Git/Forge operations
- Hardlink-based module installation to avoid redundant copies
- Fallback caching when sources are temporarily unreachable

---

## Repository Structure

```
g10k/
├── g10k.go                 # Entry point, CLI flag parsing, global vars, all major structs
├── config.go               # YAML config parsing, Ruby symbol handling, config validation
├── puppetfile.go           # Puppetfile parser and environment resolver
├── forge.go                # Puppetlabs Forge API client, download/caching
├── git.go                  # Git operations (mirror, sync, SSH key support)
├── helper.go               # Logging utilities, file ops, shell command execution
├── modules.go              # Tar archive extraction for Forge modules
├── stale.go                # Purge stale/unmanaged Puppet environments
├── g10k_test.go            # Main test suite (~3300 lines)
├── g10k_puppetfile_test.go # Puppetfile-specific tests
├── tests/                  # Test fixtures: YAML configs, Puppetfiles, .hashdeep files
├── vendor/                 # Vendored Go dependencies
├── Makefile                # Build and test automation
├── Dockerfile              # Multi-stage Docker build
├── build_release.sh        # Cross-platform release script
├── go.mod / go.sum         # Go module definitions
└── .github/
    ├── workflows/main.yml          # CI/CD (GitHub Actions, Ubuntu + macOS)
    └── copilot-instructions.md     # AI assistant hints (less detailed than this file)
```

---

## Key Source Files

### `g10k.go` — Entry Point & Type Definitions

Contains:
- **All major struct definitions**: `ConfigSettings`, `Source`, `Puppetfile`, `ForgeModule`, `GitModule`, `ForgeResult`, `ExecResult`, `DeployResult`, `DeploySettings`
- **Global variables**: `debug`, `verbose`, `config`, `mutex`, counters, timing vars
- **`main()`**: Parses CLI flags, chooses config-file mode vs puppetfile mode, calls `resolvePuppetEnvironment()` or `resolvePuppetfile()`
- **`init()`**: Initializes global maps

**Adding a CLI flag** → edit the `flag.*Var` declarations around line 227.

### `config.go` — Configuration Parsing

- `readConfigfile(filename string) ConfigSettings` — main entry point for YAML parsing
- Handles r10k-style Ruby symbol keys (`:cachedir` → `cachedir`) by stripping leading `:`
- Normalizes cache directory sub-paths (`forge/`, `modules/`, `environments/`)
- Validates `ForgeCacheTTL` duration strings

**Adding a config option** → add field to `ConfigSettings` in `g10k.go` with YAML tag, then handle defaults in `readConfigfile()`.

### `puppetfile.go` — Puppetfile Parsing & Deployment

- `readPuppetfile(...)` — parses a Puppetfile into a `Puppetfile` struct
- `resolvePuppetEnvironment(tags bool, outputName string)` — enumerates all branches from control repo sources and resolves each
- `resolvePuppetfile(pfm map[string]Puppetfile)` — orchestrates parallel module resolution using goroutine workers

**Adding a Puppetfile directive** → edit `readPuppetfile()`.

### `forge.go` — Forge API Client

- `queryForgeAPI(fm ForgeModule, ...) ForgeResult` — queries `https://forgeapi.puppet.com` (or custom URL)
- `downloadForgeModule(fm ForgeModule)` — downloads `.tar.gz`, verifies checksums
- `getMetadataForgeModule(fm ForgeModule) ForgeModule` — fetches md5/fileSize metadata
- Uses `tidwall/gjson` for JSON parsing
- Caches by module version under `config.ForgeCacheDir`

**Changing Forge API behavior** → edit `queryForgeAPI()` and the JSON field paths using gjson syntax.

### `git.go` — Git Operations

- `doMirrorOrUpdate(gitURL, gitDir string, ...)` — mirrors or updates a git repository
- `syncToModuleDir(gitDir, moduleDir string, ...)` — checks out a ref into the target directory
- Supports per-source SSH private keys via `GIT_SSH_COMMAND`
- Handles branch/tag/commit/ref resolution

### `helper.go` — Utilities

- Logging functions: `Debugf`, `Verbosef`, `Infof`, `Warnf`, `Fatalf`
- File utilities: `fileExists`, `checkDirAndCreate`, `purgeDir`, `writeStructJSONFile`
- Shell execution: `executeCommand(ExecResult)` with timeout support
- SHA256 helpers

### `stale.go` — Purge Logic

- Removes environments not present in any control repo branch
- Respects `purge_allowlist` and `purge_skiplist` config options
- Three purge levels: `deployment`, `environment`, `puppetfile`

---

## Core Data Structures

```go
// Top-level config from YAML
type ConfigSettings struct {
    CacheDir             string
    Sources              map[string]Source
    Maxworker            int           // default 50, controls parallel API ops
    MaxExtractworker     int           // default 20, controls parallel local ops
    UseCacheFallback     bool
    PurgeLevels          []string
    PurgeAllowList       []string
    ForgeCacheTTL        time.Duration
    // ... more fields in g10k.go
}

// A Puppet control repo source
type Source struct {
    Remote       string
    Basedir      string
    Prefix       string
    PrivateKey   string
    FilterRegex  string
    FilterCommand string
    // ... more fields
}

// A parsed Puppetfile
type Puppetfile struct {
    forgeModules  map[string]ForgeModule
    gitModules    map[string]GitModule
    privateKey    string
    workDir       string
    // ... more fields
}

// A Forge module entry
type ForgeModule struct {
    version   string   // e.g. "2.2.0" or "present"
    name      string
    author    string
    md5sum    string
    // ...
}

// A Git module entry
type GitModule struct {
    git      string   // git URL
    branch   string
    tag      string
    commit   string
    ref      string
    fallback []string // alternate branches to try
    // ...
}
```

---

## Development Workflow

### Prerequisites

- Go (see `go.mod` for minimum version)
- `git` in `PATH` (required at runtime)
- `hashdeep` (required for integration tests)
- `golint`, `goimports` (installed automatically by `make`)

### Common Commands

```bash
make              # lint + vet + imports + test + build binary
make test         # run full test suite with race detection
make lint         # run golint on all .go files
make vet          # run go vet
make imports      # check goimports formatting
make clean        # remove binary, coverage.txt, cache/, example/ dirs
make build-image  # build Docker image tagged with git version
make update-deps  # go get -u && go mod vendor
```

### Build Details

- Binary: `./g10k`
- Version/buildtime injected via `-ldflags`: `main.buildversion` and `main.buildtime`
- Race detector enabled on Linux builds; disabled on macOS due to a known issue in older Go versions
- `CGO_ENABLED=1` required (native git support)

---

## Testing Patterns

### Naming Convention

Config tests follow a strict convention: `TestConfigXxx` reads `tests/TestConfigXxx.yaml`.

```go
func TestConfigPrefix(t *testing.T) {
    funcName := strings.Split(funcName(), ".")[len(strings.Split(funcName(), "."))-1]
    got := readConfigfile(filepath.Join("tests", funcName+".yaml"))
    // build expected ConfigSettings and compare with reflect.DeepEqual
}
```

### Crash / Fatal Tests (subprocess pattern)

Tests that exercise `Fatalf()` (which calls `os.Exit(1)`) use subprocess testing:

```go
func TestSomeFatalCondition(t *testing.T) {
    funcName := strings.Split(funcName(), ".")[len(strings.Split(funcName(), "."))-1]
    if os.Getenv("TEST_FOR_CRASH_"+funcName) == "1" {
        // trigger the code that calls Fatalf()
        return
    }
    cmd := exec.Command(os.Args[0], "-test.run="+funcName+"$")
    cmd.Env = append(os.Environ(), "TEST_FOR_CRASH_"+funcName+"=1")
    out, err := cmd.CombinedOutput()
    // assert non-zero exit, check stderr message
}
```

### Integration Tests with `hashdeep`

Full-environment deployment tests verify the resulting directory tree against `.hashdeep` files in `tests/`:

```go
out, err := exec.Command("hashdeep", "-rl", "-k", "tests/some.hashdeep", "example/").CombinedOutput()
// expect empty output (no mismatches)
```

### HTTP Mock for Forge

Tests that exercise Forge API calls use `net/http/httptest`:

```go
ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
    fmt.Fprintln(w, `{"version": "2.2.0", ...}`)
}))
defer ts.Close()
```

### Struct Comparison Helpers

- `equalPuppetfile(a, b Puppetfile)` — compares Puppetfile structs field by field
- `equalGitModule(a, b GitModule)` — same for GitModule
- `spew.Dump()` from `github.com/davecgh/go-spew` for verbose diff output on failures

---

## Code Conventions

### Logging

All logging goes through helpers in `helper.go`. Do not use `fmt.Println` or `log.Print` directly in business logic.

| Function | Condition | Color |
|---|---|---|
| `Debugf(s)` | `-debug` flag | none (prefixed with `DEBUG funcname():`) |
| `Verbosef(s)` | `-verbose` or `-debug` | none |
| `Infof(s)` | `-info`, `-verbose`, or `-debug` | green |
| `Warnf(s)` | always | yellow |
| `Fatalf(s)` | always | red + `os.Exit(1)` (or collect in `-validate` mode) |

### Error Handling

- Functions return a `bool` for success/failure or an `ExecResult` struct
- Use `Fatalf()` for unrecoverable errors
- In `-usecachefallback` mode, some `Fatalf` calls are demoted to `Warnf` — check `config.UseCacheFallback` before deciding severity
- Never `panic()` in application code

### Concurrency

- The two primary worker pools are controlled by `config.Maxworker` (default 50) and `config.MaxExtractworker` (default 20)
- Use `sizedwaitgroup.New(n)` to create bounded goroutine pools
- Shared mutable state (`syncGitCount`, `uniqueForgeModules`, etc.) is protected by `mutex sync.Mutex`
- `LatestForgeModules` embeds `sync.RWMutex` for concurrent reads

### Module Installation Modes

| Mode | Behavior | Use case |
|---|---|---|
| Default (hardlinks) | Links files from cache | Production, fastest |
| `-usemove` | Moves files, purges cache after run | Docker containers |
| `-clonegit` | Full `git clone` per module | Local development |

### Config File Format

Supports r10k-compatible YAML. Ruby-style symbol keys (`:cachedir`) are stripped of the leading `:` during parsing.

```yaml
:cachedir: "/var/cache/g10k"
sources:
  puppet:
    remote: "https://github.com/org/control-repo.git"
    basedir: "/etc/puppetlabs/code/environments/"
    prefix: false
    invalid_branches: "correct_and_warn"
```

---

## CLI Flags Reference

| Flag | Type | Default | Purpose |
|---|---|---|---|
| `-config` | string | — | Path to g10k config YAML |
| `-puppetfile` | bool | false | Puppetfile-only mode (no config) |
| `-puppetfilelocation` | string | `./Puppetfile` | Puppetfile path in `-puppetfile` mode |
| `-branch` | string | — | Only sync this branch |
| `-environment` | string | — | Only sync this environment (source_branch) |
| `-module` | string | — | Only sync this module |
| `-maxworker` | int | 50 | Parallel Forge/Git resolve workers |
| `-maxextractworker` | int | 20 | Parallel extract workers |
| `-debug` | bool | false | Enable debug logging |
| `-verbose` | bool | false | Enable verbose logging |
| `-dryrun` | bool | false | Print what would change, no writes |
| `-validate` | bool | false | Validate config and exit |
| `-force` | bool | false | Purge environment dir and full resync |
| `-usemove` | bool | false | Move instead of hardlink (Docker mode) |
| `-usecachefallback` | bool | false | Use cache on network failures |
| `-clonegit` | bool | false | `git clone` each git module |
| `-checksum` | bool | false | Verify Forge module MD5 checksums |
| `-check4update` | bool | false | Check for newer module versions only |
| `-tags` | bool | false | Also pull git tags |
| `-cachedir` | string | — | Override cachedir from config |
| `-moduledir` | string | — | Override moduledir from Puppetfile |

---

## Where to Make Changes

| Task | File | Location |
|---|---|---|
| Add a CLI flag | `g10k.go` | `flag.*Var` declarations in `main()`, ~line 227 |
| Add a config option | `g10k.go` + `config.go` | Add field to `ConfigSettings`, handle in `readConfigfile()` |
| Add a Puppetfile directive | `puppetfile.go` | `readPuppetfile()` function |
| Change Forge API behavior | `forge.go` | `queryForgeAPI()`, gjson field paths |
| Change Git behavior | `git.go` | `doMirrorOrUpdate()`, `syncToModuleDir()` |
| Change purge behavior | `stale.go` | |
| Add a log helper | `helper.go` | |

---

## CI/CD

GitHub Actions (`.github/workflows/main.yml`):
- Triggers: push, pull_request, workflow_dispatch
- Matrix: `ubuntu-latest` and `macOS-latest`
- Go version: 1.24 (pinned, check-latest enabled)
- Steps: install `hashdeep` → `make` (runs lint + vet + imports + test + build)
- No separate deploy step; releases are done manually via `build_release.sh`

### Release Process (`build_release.sh`)

1. Runs `make test`
2. Creates and pushes a git tag
3. Builds binaries for darwin-amd64, darwin-arm64, linux-amd64
4. Uploads to GitHub Releases via `github-release` CLI

---

## Dependencies

Managed with Go modules and vendored in `vendor/`.

| Package | Purpose |
|---|---|
| `gopkg.in/yaml.v2` | YAML config/Puppetfile parsing |
| `github.com/tidwall/gjson` | Forge API JSON parsing |
| `github.com/fatih/color` | Colored terminal output |
| `github.com/remeh/sizedwaitgroup` | Bounded goroutine pools |
| `github.com/klauspost/pgzip` | Parallel gzip for Forge archives |
| `github.com/kballard/go-shellquote` | Shell command string parsing |
| `github.com/xorpaul/uiprogress` | Progress bars |
| `github.com/davecgh/go-spew` | Debug struct printing in tests |
| `golang.org/x/sys`, `golang.org/x/term` | Platform syscalls |

Update deps: `make update-deps` (runs `go get -u && go mod vendor`).
