# g10k Copilot Instructions

## Project Overview

g10k is a high-performance Go implementation of r10k for Puppet environment deployment. It syncs Puppet environments from Git control repositories and resolves Puppetfile dependencies (Forge modules + Git modules) with aggressive caching and parallelism.

## Architecture

### Core Components

- **g10k.go** - Entry point, CLI flag parsing, orchestrates config vs puppetfile modes
- **config.go** - YAML config parsing (`readConfigfile()`), handles r10k-style Ruby symbols (`:cachedir`)
- **puppetfile.go** - Puppetfile parser and environment resolver (`resolvePuppetEnvironment()`, `resolvePuppetfile()`)
- **forge.go** - Puppetlabs Forge API client, module download/caching (`queryForgeAPI()`, `downloadForgeModule()`)
- **git.go** - Git operations with SSH key support (`doMirrorOrUpdate()`, `syncToModuleDir()`)
- **helper.go** - Logging (`Debugf`, `Verbosef`, `Fatalf`), file utilities, command execution
- **modules.go** - Tar extraction for Forge modules

### Key Data Flow

1. Parse config YAML → `ConfigSettings` struct with `Sources` map
2. For each source: mirror control repo → enumerate branches → resolve Puppetfiles
3. Parallel resolution of Git modules (`uniqueGitModules`) and Forge modules (`uniqueForgeModules`)
4. Extract/link modules to target environment directories under `basedir`

### Concurrency Model

- `config.Maxworker` (default 50) - parallel Forge/Git API operations
- `config.MaxExtractworker` (default 20) - parallel local extraction (clone, untar)
- Uses `sizedwaitgroup` for bounded parallelism, mutex for shared state

## Development Commands

```bash
make              # Run lint, vet, imports, tests, then build
make test         # Run full test suite with race detection
make lint         # golint all .go files
make vet          # go vet
make imports      # goimports check
make clean        # Remove build artifacts and cache/example dirs
make update-deps  # go get -u ./... && go mod tidy
```

## Testing Patterns

Tests follow naming convention: `TestConfigXxx` reads `tests/TestConfigXxx.yaml`

```go
func TestConfigPrefix(t *testing.T) {
    funcName := strings.Split(funcName(), ".")[len(strings.Split(funcName(), "."))-1]
    got := readConfigfile(filepath.Join("tests", funcName+".yaml"))
    // ... compare with expected ConfigSettings
}
```

For crash/exit tests, use subprocess pattern:

```go
if os.Getenv("TEST_FOR_CRASH_"+funcName) == "1" {
    // code that should exit
    return
}
cmd := exec.Command(os.Args[0], "-test.run="+funcName+"$")
cmd.Env = append(os.Environ(), "TEST_FOR_CRASH_"+funcName+"=1")
```

## Code Conventions

### Logging Levels

- `Debugf()` - detailed debug (requires `-debug` flag)
- `Verbosef()` - verbose output (requires `-verbose` flag)
- `Infof()` - informational, green colored
- `Warnf()` - warnings, yellow colored
- `Fatalf()` - fatal errors, exits with code 1 (or collects for `-validate` mode)

### Error Handling

Functions return success booleans; use `Fatalf()` for unrecoverable errors. Cache fallback mode (`-usecachefallback`) converts some fatals to warnings.

### Module Structs

- `ForgeModule` - Forge module metadata (author, name, version, checksums)
- `GitModule` - Git module config (git URL, branch/tag/commit/ref, privateKey)
- `Puppetfile` - parsed Puppetfile with `forgeModules` and `gitModules` maps

### Config File Format

Supports r10k-style YAML with Ruby symbols (`:cachedir`). Symbols are stripped during parsing:

```yaml
:cachedir: "/var/cache/g10k"
sources:
  puppet:
    remote: "https://github.com/org/control-repo.git"
    basedir: "/etc/puppetlabs/code/environments/"
```

## Key Files for New Features

- Adding CLI flags → `g10k.go` (flag definitions around line 225)
- New config options → `config.go` (struct fields + YAML tags) and `ConfigSettings` in `g10k.go`
- Puppetfile directives → `puppetfile.go` (`readPuppetfile()` function)
- Forge API changes → `forge.go` (`queryForgeAPI()`, JSON parsing with gjson)
