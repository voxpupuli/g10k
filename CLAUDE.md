# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

g10k is a high-performance Go implementation of r10k for Puppet environment deployment. It syncs Puppet environments from Git control repositories and resolves Puppetfile dependencies (Forge modules + Git modules) with aggressive caching and parallelism.

## Build & Test Commands

```bash
make                              # lint, vet, imports, test, then build
make test                         # full test suite with race detection
go test -run 'TestName$' -v      # run a single test
go test -run 'TestA|TestB' -v    # run multiple specific tests
make lint                         # golint
make vet                          # go vet
make imports                      # goimports check
make clean                        # remove binary, coverage, cache/example dirs
```

Tests require `hashdeep` (apt-get install hashdeep / brew install hashdeep) for file tree verification. On macOS, tests use `MallocNanoZone=0` workaround.

## Architecture

### Data Flow

1. Parse config YAML (`config.go: readConfigfile()`) → `ConfigSettings` with `Sources` map
2. For each source: mirror control repo → enumerate branches/tags → read Puppetfiles (`puppetfile.go: resolvePuppetEnvironment()`)
3. Deduplicate and resolve modules in parallel (`puppetfile.go: resolvePuppetfile()` → `forge.go: resolveForgeModules()` + `git.go: resolveGitRepositories()`)
4. Extract/link modules to environment directories under each source's `basedir`
5. Purge stale content (`stale.go`) based on `purge_levels` config

### Two Modes

- **Config mode** (`-config file.yaml`): resolves multiple environments across sources
- **Puppetfile mode** (`-puppetfile`): resolves single `./Puppetfile` in cwd

### Concurrency

- `config.Maxworker` (default 50): parallel Forge API queries and git remote updates
- `config.MaxExtractworker` (default 20): parallel local extraction (git clone, untar)
- Uses `sizedwaitgroup` for bounded parallelism, `sync.Mutex` for shared global state

### Config File Format

Supports r10k-style YAML with Ruby symbols (`:cachedir`). Symbols are stripped during parsing in `readConfigfile()`.

### Key Extension Points

- CLI flags: `g10k.go` flag definitions (~line 225)
- Config options: `ConfigSettings` struct in `g10k.go` + `readConfigfile()` in `config.go`
- Puppetfile directives: `readPuppetfile()` in `puppetfile.go`
- Forge API: `queryForgeAPI()` in `forge.go` (uses `gjson` for JSON parsing)

## Global State

The codebase relies heavily on package-level globals (`config`, `uniqueForgeModules`, `latestForgeModules`, `needSyncEnvs`, `force`, `branchParam`, `quiet`, `debug`, etc.). These persist across test functions within the same process.

`Fatalf()` in `helper.go` calls `os.Exit(1)`, which kills the entire test process — this is why tests that exercise fatal paths use the subprocess pattern.

## Testing Patterns

### Subprocess pattern

Many integration tests re-execute themselves as a subprocess to safely handle `Fatalf`/`os.Exit`:

```go
if os.Getenv("TEST_FOR_CRASH_"+funcName) == "1" {
    // actual test logic (may call Fatalf)
    return
}
cmd := exec.Command(os.Args[0], "-test.run="+funcName+"$")
cmd.Env = append(os.Environ(), "TEST_FOR_CRASH_"+funcName+"=1")
out, err := cmd.CombinedOutput()
// parent checks exit code + output
```

### Test file conventions

- Config tests: `TestConfigXxx` reads `tests/TestConfigXxx.yaml`
- Puppetfile tests: `TestReadPuppetfileXxx` reads `tests/TestReadPuppetfileXxx` (no extension)
- Integration tests deploy to `/tmp/example`, `/tmp/full`, `/tmp/out` with cache in `/tmp/g10k`
- File tree verification uses `hashdeep` against `.hashdeep` reference files in `tests/`

### Global state pollution between tests

This is the most common source of test failures when adding new tests or features:

- **`uniqueForgeModules`**: retains ForgeModule entries (including `baseURL`) across `resolvePuppetfile` calls. Must be reinitialized at the start of `resolvePuppetfile()`.
- **`needSyncEnvs`**: retains `PuppetfileMatch` entries. When a test calls `resolvePuppetEnvironment` twice (e.g., deploy then check purge), reset with `needSyncEnvs = make(map[string]struct{})` between calls.
- **Deploy artifacts**: `.g10k-deploy.json` files in `/tmp/` dirs persist between test runs. If a prior run's deploy file exists with a matching signature, the PuppetfileMatch optimization skips module resolution. Fix by setting `force = true` in the subprocess, or purging deploy dirs before it runs.
- **Debugging tip**: set `quiet = false` and `debug = true` in the test to get verbose output. For subprocess tests, these must be set inside the `TEST_FOR_CRASH_` block.

### PuppetfileMatch optimization (`optimize_branch`)

- `syncToModuleDir()` in `git.go` sets `needSyncEnvs[env+":PuppetfileMatch"]` when the deploy result signature matches or the Puppetfile checksum matches between deployed and upstream.
- `resolvePuppetEnvironment()` reads this into `puppetfile.upstreamPuppetfileMatches`.
- `resolvePuppetfile()` skips module resolution when `pf.upstreamPuppetfileMatches && !force`.
- Setting `force = true` bypasses this optimization.
