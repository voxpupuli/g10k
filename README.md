[![Build Status](https://github.com/voxpupuli/g10k/actions/workflows/build.yml/badge.svg)](https://github.com/voxpupuli/g10k/actions) [![Go Report Card](https://goreportcard.com/badge/github.com/xorpaul/g10k)](https://goreportcard.com/report/github.com/xorpaul/g10k)

# g10k

My r10k fork written in Go, designed to work somewhat similar like [puppetlabs/r10k](https://github.com/puppetlabs/r10k).

### Why fork?

- Lack of caching/version-pre-checking in current r10k implementation hurt performance beyond a certain # of modules per Puppetfile
- We need distinct SSHKeys for each source in the r10k.yaml and 'rugged' never really wanted to play nice (fixed in r10k [2.2.0](https://github.com/puppetlabs/r10k/blob/master/CHANGELOG.mkd#220))
- Good excuse to try Go ;)

### Changes breaking complete r10k compatibility

- No SVN support
- Forge modules must be specified like this:

```
mod 'theforeman/puppet'
```

- Git modules must be specified like this:

```
mod 'apache',
  :git => 'https://github.com/puppetlabs/puppetlabs-apache.git'
```

### Non-breaking changes to r10k

- Download/Cache each git Puppet Module repository and each Puppetlabs Forge Puppet Module for each respective version only once
- Most things (git, forge, and copy operations) done in parallel over each branch
- Optional support for different ssh keys for each source inside the r10k.yaml

### Pseudo "benchmark"

Using Puppetfile with 4 git repositories and 25 Forge modules
https://github.com/xorpaul/g10k-environment/blob/benchmark/Puppetfile

| 2016-10-14 | w/o cache         | w/ cache    |
| ---------- | ----------------- | ----------- |
| r10k       | 1m14s,1m18s,1m12s | 18s,17s,17s |
| g10k       | 4.6s,5s,4.7s      | 1s,1s,1s    |

Using go 1.7.1 and g10k commit 7524778
Using ruby 2.1.5+deb8u2 and r10k v2.4.3
On Dell PowerEdge R320 Intel Xeon E5-2430 24 GB RAM on Debian Jessie

##### Benchmark w/o cache

```
rm -rf /tmp/g10k ; GDIR=$RANDOM ; mkdir /tmp/$GDIR/ ; cd /tmp/$GDIR/ ; \
wget https://raw.githubusercontent.com/xorpaul/g10k-environment/benchmark/Puppetfile ; \
time g10k -puppetfile

RDIR=$RANDOM ; mkdir /tmp/$RDIR/ ; cd /tmp/$RDIR/ ; \
wget https://raw.githubusercontent.com/xorpaul/g10k-environment/benchmark/Puppetfile ; \
time r10k puppetfile install
```

##### Benchmark w/ cache

```
cd /tmp/$GDIR/ ; time g10k -puppetfile
cd /tmp/$RDIR/ ; time r10k puppetfile install
```

# installation

You can just grab the most recent stable release here:
https://github.com/voxpupuli/g10k/releases

Tagged releases are built by GitHub Actions with GoReleaser.

- Before using g10k with a large Puppet setup with many modules, be sure to increase the amount of open file handles (nfiles) and number of child processes (nproc), see limits.conf(5) for details.
- If you are using a private Git or Forge server think about adjusting the `-maxworker` parameter/config setting before DOSing your own infrastructure ;) (default 50)
- To protect your local machine use `-maxextractworker` parameter/config setting with wich you can limit the number of Goroutines that are allowed to run in parallel for local Git and Forge module extracting processes (git clone, untar and gunzip) (default 20)

## installation of g10k via Puppet module

User @Conzar was so nice and shared his g10k Puppet module that you can check out here:

- [Puppet Forge](https://forge.puppet.com/landcareresearch/g10k)
- [Source code](https://bitbucket.org/landcareresearch/puppet-g10k)

## Configuration File

If you do not run g10k in `-puppetfile` mode, which would just populate the current folder with all Puppet module inside the Puppetfile, then g10k uses a YAML configuration file to define how it should sync your Puppet environments. The configuration file must be specified using the `-config` parameter - there is no default location.

The g10k configuration format is inspired by r10k's configuration which is [documented here](https://github.com/puppetlabs/r10k/blob/main/doc/dynamic-environments/configuration.mkd), though not all r10k features are available in g10k.

### Basic Configuration Example

Create a configuration file (e.g., `g10k.yaml`) with the following structure:

```yaml
---
:cachedir: "/var/cache/g10k"

sources:
  puppet:
    remote: "https://github.com/your-org/puppet-control-repo.git"
    basedir: "/etc/puppetlabs/code/environments/"
```

Then run g10k with: `./g10k -config g10k.yaml`

### Required Configuration Options

- **cachedir**: Directory where g10k will cache git repositories and Forge modules
- **sources**: One or more Puppet environment sources, each containing:
  - **remote**: Git URL of your Puppet control repository
  - **basedir**: Directory where Puppet environments will be created

### Common Configuration Options

- **timeout**: Timeout in seconds for git and forge operations (default: 5)
- **maxworker**: Number of concurrent workers for git/forge operations (default: 50)
- **maxextractworker**: Number of concurrent workers for extraction operations (default: 20)
- **forge_base_url**: Custom Forge API URL (default: https://forgeapi.puppet.com)
- **deploy**: Advanced deployment settings including purge levels and allowlists
  - **purge_levels**: Array controlling what to purge (e.g., `['deployment', 'puppetfile']`)
  - **purge_allowlist**: Files/directories to preserve during purge operations

### Per-Source Options

Each source can have additional options:

- **prefix**: Prefix to add to environment names (boolean or string)
- **private_key**: Path to SSH private key for authentication
- **force_forge_versions**: Require explicit versions for Forge modules (boolean)
- **invalid_branches**: How to handle invalid branch names (`correct`, `correct_and_warn`, `error`)
- **filter_regex**: Regex pattern to filter which branches to sync

See the [additional g10k config features](#additional-g10k-config-features-compared-to-r10k) section for more advanced options.

## Usage Docs

```
Usage of ./g10k:
  -branch string
        which git branch of the Puppet environment to update. Just the branch name, e.g. master, qa, dev
  -cachedir string
        allows overriding of the g10k config file cachedir setting, the folder in which g10k will download git repositories and Forge modules
  -check4update
        only check if the is newer version of the Puppet module avaialable. Does implicitly set dryrun to true
  -checksum
        get the md5 check sum for each Puppetlabs Forge module and verify the integrity of the downloaded archive. Increases g10k run time!
  -clonegit
        populate the Puppet environment with a git clone of each git Puppet module. Helpful when developing locally with -puppetfile
  -config string
        which config file to use
  -debug
        log debug output, defaults to false
  -dryrun
        do not modify anything, just print what would be changed
  -environment string
        which Puppet environment to update. Source name inside the config + '_' + branch name, e.g. foo_master, foo_qa, foo_dev
  -force
        purge the Puppet environment directory and do a full sync
  -gitobjectsyntaxnotsupported
        if your git version is too old to support reference syntax like master^{object} use this setting to revert to the older syntax
  -info
        log info output, defaults to false
  -maxextractworker int
        how many Goroutines are allowed to run in parallel for local Git and Forge module extracting processes (git clone, untar and gunzip) (default 20)
  -maxworker int
        how many Goroutines are allowed to run in parallel for Git and Forge module resolving (default 50)
  -module string
        which module of the Puppet environment to update, e.g. stdlib
  -moduledir string
        allows overriding of Puppetfile specific moduledir setting, the folder in which Puppet modules will be extracted
  -outputname string
        overwrite the environment name if -branch is specified
  -puppetfile
        install all modules from Puppetfile in cwd
  -puppetfilelocation string
        which Puppetfile to use in -puppetfile mode (default "./Puppetfile")
  -quiet
        no output, defaults to false
  -retrygitcommands
        if g10k should purge the local repository and retry a failed git command (clone or remote update) instead of failing
  -tags
        to pull tags as well as branches
  -usecachefallback
        if g10k should try to use its cache for sources and modules instead of failing
  -usemove
        do not use hardlinks to populate your Puppet environments with Puppetlabs Forge modules. Instead uses simple move commands and purges the Forge cache directory after each run! (Useful for g10k runs inside a Docker container)
  -validate
        only validate given configuration and exit
  -verbose
        log verbose output, defaults to false
  -version
        show build time and version number
```

Regarding anything usage/workflow you really can just use the great [puppetlabs/r10k](https://github.com/puppetlabs/r10k/blob/master/doc/dynamic-environments.mkd) docs as the [Puppetfile](https://github.com/puppetlabs/r10k/blob/master/doc/puppetfile.mkd) etc. are all intentionally kept unchanged.

## Using g10k behind a proxy

Set the environment variables `http_proxy` or `https_proxy` to make g10k use a proxy.
E.g. `http_proxy=http://proxy.domain.tld:8080 ./g10k -puppetfile`
See https://golang.org/pkg/net/http/#ProxyFromEnvironment for details.

# additional Puppetfile features

- link Git module branch to the current environment branch:

```
mod 'awesomemodule',
    :git => 'http://github.com/foo/bar.git',
    :link => 'true'
```

If you are in environment branch `dev` then g10k would try to check out this module with branch `dev`.
This helps to be able to use the same Puppetfile over multiple environment branches and makes merges easier.
See https://github.com/voxpupuli/g10k/issues/6 for details.

Now also supports the r10k setting name `:branch => :control_branch` See [#73](https://github.com/voxpupuli/g10k/issues/73)

See https://github.com/voxpupuli/g10k/issues/6 for details.

```
mod 'awesomemodule',
    :git => 'http://github.com/foo/bar.git',
    :ignore-unreachable => 'true'
```

In combination with the previous link feature you don't need to keep all environment branches also available for your modules.
See https://github.com/voxpupuli/g10k/issues/9 for details.

- use different Forge base URL for your modules in your Puppetfile

```
forge.baseUrl http://foobar.domain.tld/
```

- skip version checks for latest Forge modules for a certain time to speed up the sync

```
forge.cacheTtl 4h
```

You need to specify the TTL value in the form of golang Duration (https://golang.org/pkg/time/#ParseDuration)

- try multiple Git branches for a Puppet module until one can be used

```
mod 'stdlib',
    :git => 'https://github.com/puppetlabs/puppetlabs-stdlib.git',
    :fallback => '4.889.x|foobar|master'
```

In this example g10k tries to use the branches:

`4.889.x` -> `foobar` -> `master`

Because there are no branches `4.889.x` or `foobar`.

All without failing or error messages.

Tip: You can see which branch was used, when using the `-verbose` parameter:

```
./g10k -puppetfile -verbose
2016/11/08 14:16:40 Executing git --git-dir ./tmp/https-__github.com_puppetlabs_puppetlabs-stdlib.git remote update --prune took 1.05001s
2016/11/08 14:16:40 Executing git --git-dir ./tmp/https-__github.com_puppetlabs_puppetlabs-stdlib.git log -n1 --pretty=format:%H master took 0.00299s
Synced ./Puppetfile with 4 git repositories and 0 Forge modules in 1.1s with git (1.1s sync, I/O 0.0s) and Forge (0.0s query+download, I/O 0.0s)
```

Now also supports the r10k setting name `:default_branch => 'master'` See [#73](https://github.com/voxpupuli/g10k/issues/73)

- additional Git attribute `:use_ssh_agent`:

Normally g10k adds the SSH key specified in the g10k config for each SSH+Git module in your Puppetfile.
If you don't want to use this SSH key, need a different key for a certain Git module or have the key encrypted in your SSH agent, then use this parameter to skip the `ssh-add` commands:

```
mod 'example_module',
  :git => 'git@somehost.com/foo/example-module.git',
  :branch => 'foo',
  :use_ssh_agent => true
```

See [#171](https://github.com/voxpupuli/g10k/issues/171) for more details.

- additional Forge attribute `:sha256sum`:

Starting with [v.0.7.1](https://github.com/voxpupuli/g10k/releases/tag/v0.7.1) g10k supports `purge_skiplist` feature to remove unnecessary files from the sync / Puppetservers.

```
mod 'puppetlabs/ntp', '6.0.0', :sha256sum => 'a988a172a3edde6ac2a26d0e893faa88d37bc47465afc50d55225a036906c944'

```

This does provide a very crude way to detect manipulated Forge modules and MITM attacks until the Puppetlabs Forge does support some sort of signing of Forge module releases.

If the SHA256 sum does not match the expected hash sum, g10k will warn the user and retry a download until giving up:

```
Resolving Forge modules (0/1)   --- [--------------------------------------------------------------------]   0%
WARNING: calculated sha256sum a988a172a3edde6ac2a26d0e893faa88d37bc47465afc50d55225a036906c944 for ./tmp/puppetlabs-ntp-6.0.0.tar.gz does not match expected sha256sum a988a172a3edde6ac2a26d0e893faa88d37bc47465afc50d55225a036906c94
Resolving Forge modules (0/1)   --- [--------------------------------------------------------------------]   0%
WARNING: calculated sha256sum a988a172a3edde6ac2a26d0e893faa88d37bc47465afc50d55225a036906c944 for ./tmp/puppetlabs-ntp-6.0.0.tar.gz does not match expected sha256sum a988a172a3edde6ac2a26d0e893faa88d37bc47465afc50d55225a036906c94
2016/12/08 18:05:11 downloadForgeModule(): giving up for Puppet module puppetlabs-ntp version: 6.0.0

```

(The Forge module retry count in case the Puppetlabs Forge provided MD5 sum, file archive size or SHA256 sum doesn't match defaults to `1`, but will be user configurable later.)

- override g10k cache directory with environment variable

You can use the following environment variable to make g10k use a different cache directory:

```
g10k_cachedir=/var/tmp g10k ...
```

This will also override the `-cachedir` parameter.

# additional g10k config features compared to r10k

- you can enforce version numbers of Forge modules in your Puppetfiles instead of `:latest` or `:present` by adding `force_forge_versions: true` to the g10k config in the specific resource

```
---
:cachedir: '/tmp/g10k'

sources:
  example:
    remote: 'https://github.com/xorpaul/g10k-environment.git'
    basedir: '/tmp/example/'
    force_forge_versions: true
```

If g10k then encounters `:latest` or `:present` for a Forge module it errors out with:

```
2016/11/15 18:45:38 Error: Found present setting for forge module in /tmp/example/example_benchmark/Puppetfile for module puppetlabs/concat line: mod 'puppetlabs/concat' and force_forge_versions is set to true! Please specify a version (e.g. '2.3.0')
```

- g10k can let you know if your source does not contain the branch you specified with the `-branch` parameter:

```
---
:cachedir: '/tmp/g10k'

sources:
  example:
    remote: 'https://github.com/xorpaul/g10k-environment.git'
    basedir: '/tmp/example/'
    warn_if_branch_is_missing: true
```

If you then call g10k with this config file and the following parameter `-branch nonExistingBranch`. You should get:

```
WARNING: Couldn't find specified branch 'nonExistingBranch' anywhere in source 'example' (https://github.com/xorpaul/g10k-environment.git)
```

This can be helpful if you use a dedicated hiera repository/g10k source and you want to ensure that you always have a matching branch, see [#45](https://github.com/voxpupuli/g10k/issues/45)

- By default g10k fails if one of your Puppet environments could not be completely populated (e.g. if one of your Puppet Git module branches doesn't exist anymore). You can change this by setting `ignore_unreachable_modules` to true in your g10k config:

```
---
:cachedir: '/tmp/g10k'
ignore_unreachable_modules: true

sources:
  example:
    remote: 'https://github.com/xorpaul/g10k-failing-env.git'
    basedir: '/tmp/failing/'
```

If you then call g10k with this config file and `debug` verbosity level, you should get:

```
DEBUG: Failed to populate module /tmp/failing/master/modules//sensu/ but ignore-unreachable is set. Continuing...
```

See [#57](https://github.com/voxpupuli/g10k/issues/57) for details.

- abort g10k run if source repository is unreachable

```
---
:cachedir: '/tmp/g10k'

sources:
  example:
    remote: 'git://github.com/xorpaul/g10k-environment-unavailable.git'
    basedir: '/tmp/example/'
    exit_if_unreachable: true
```

If you then call g10k with this config file. You should get:

```
WARN: git repository git://github.com/xorpaul/g10k-environment-unavailable.git does not exist or is unreachable at this moment!
WARNING: Could not resolve git repository in source 'example' (git://github.com/xorpaul/g10k-environment-unavailable.git)
```

with an exit code 1

- g10k can use the cached version of Forge and git modules if their sources are currently not available:

```
---
:cachedir: '/tmp/g10k'
use_cache_fallback: true

sources:
  example:
    remote: 'git://github.com/xorpaul/g10k-environment-unavailable.git'
    basedir: '/tmp/example/'
```

If you then call g10k with this config file and your github.com repository is unavailable your g10k run tries to find a suitable cached version of your modules:

```
WARN: git repository https://github.com/puppetlabs/puppetlabs-firewall.git does not exist or is unreachable at this moment!
WARN: Trying to use cache for https://github.com/puppetlabs/puppetlabs-firewall.git git repository
```

if your g10k did manage to at least once cache this git repository.

If there is no useable cache available your g10k run still fails.

- You can let g10k retry to git clone or update the local repository if it failed before and was left in a corrupted state:

```
---
:cachedir: '/tmp/g10k'
retry_git_commands: true

sources:
  example:
    remote: 'https://github.com/xorpaul/g10k-environment.git'
    basedir: '/tmp/example/'
```

If you then call g10k with this config file and have a corrupted local Git repository, g10k deletes the local cache and retries the Git clone command once:

```
WARN: git command failed: git --git-dir /tmp/g10k/modules/https-__github.com_puppetlabs_puppetlabs-firewall.git remote update --prune deleting local cached repository and retrying...
```

See [#76](https://github.com/voxpupuli/g10k/issues/76) for details.

- Autocorrecting Puppet environment names

Like in [r10k](https://github.com/puppetlabs/r10k/blob/master/doc/dynamic-environments/git-environments.mkd#invalid_branches) for each source in your g10k config you can set the attribute `invalid_branches` with the following values:

- `correct_and_warn`: Non-word characters will be replaced with underscores and a warning will be emitted.
- `correct`: Non-word characters will silently be replaced with underscores.
- `error`: Branches with non-word characters will be ignored and an error will be emitted.

The default value is to leave the environment unchanged, which differs from the r10k default!

Example:

```
---
:cachedir: '/tmp/g10k'

sources:
  example:
    remote: 'https://github.com/xorpaul/g10k-environment.git'
    basedir: '/tmp/example/'
    invalid_branches: 'correct'
```

If you then call g10k with this config file and have a branch named something like `single_autocorrect-%-fooo` it will be renamed to `single_autocorrect___fooo`

See [#81](https://github.com/voxpupuli/g10k/issues/81) for details.

- Support for older Git versions, like on CentOS 6

To check for really existing objects, g10k uses `master^{object}` syntax, which is not supported in older Git versions, like on CentOS 6, see [#91](https://github.com/voxpupuli/g10k/issues/91)
g10k will skip this sanity check when the g10k config setting `git_object_syntax_not_supported` is set to `true` (defaults to `false`)
Example:

```
---
:cachedir: '/tmp/g10k'
git_object_syntax_not_supported: true

sources:
  example:
    remote: 'https://github.com/xorpaul/g10k-environment.git'
    basedir: '/tmp/example/'
```

- Added support for r10k-like purge behaviour of stale content

Starting with [v.0.9.0](https://github.com/voxpupuli/g10k/releases/tag/v0.9.0) g10k supports the r10k-like purge behaviour of stale content with the different configuration settings `purge_level` and `purge_allowlist` as documented [here for purge_levels](https://github.com/puppetlabs/r10k/blob/master/doc/dynamic-environments/configuration.mkd#purge_levels) and [here for purge_allowlist](https://github.com/puppetlabs/r10k/blob/master/doc/dynamic-environments/configuration.mkd#purge_allowlist)

Please check if you need to allowlist files/folders inside your Puppet environments!

As an additional setting, you can also allowlist Puppet environments with `deployment_purge_allowlist`, that would've been purged by the [deployment](https://github.com/puppetlabs/r10k/blob/master/doc/dynamic-environments/configuration.mkd#deployment) `purge_level`.
This can be helpful if you have a similar source name or prefix set. E.g. having a source called `foobar` and another one `foobar_hiera` would have purged all foobar*hiera*\* branches if there are not branches called `hiera_master` or similar in the `foobar` source.

Example:

```
---
deploy:
  purge_levels: ['deployment', 'puppetfile', 'environment']
  purge_allowlist: [ '.latest_revision', '.resource_types', 'resource_types/*.pp', '**/*.pp'. ]
  deployment_purge_allowlist: [ 'example_hiera_*', '.resource_types' ]

sources:
  example:
    remote: 'https://github.com/xorpaul/g10k-environment.git'
    basedir: '/tmp/out/'
    prefix: true
  example_hiera:
    remote: 'https://github.com/xorpaul/g10k-hiera.git'
    basedir: '/tmp/out/'
    prefix: true
```

Starting with [v.0.7.1](https://github.com/voxpupuli/g10k/releases/tag/v0.7.1) g10k supports `purge_skiplist` feature to remove unnecessary files from the sync / Puppetservers.

Example:

```
---
deploy:
  purge_skiplist: [ 'spec', 'readmes', 'examples', '*.markdown', '*.md', 'junit', 'docs' ]

sources:
  example:
    remote: 'https://github.com/xorpaul/g10k-environment.git'
    prefix: true
    basedir: './example/'
```

Starting with [v.0.8.12](https://github.com/voxpupuli/g10k/releases/tag/v0.8.12) g10k supports filtering branches via regex or an external script:

Example using external script:

```
---
sources:
  example:
    remote: 'https://github.com/xorpaul/g10k-environment.git'
    basedir: './example/'
    filter_command: 'tests/branch_filter_command.sh $R10K_BRANCH ^(single|master)$'
```

or via regex

```
---
sources:
  example:
    remote: 'https://github.com/xorpaul/g10k-environment.git'
    basedir: './example/'
    filter_regex: '^(single|master)$'
```

See #166 for the discussion and #167 for the merge request.

# building

```
# build a local binary with a fake version string
make build BUILDVERSION=v0.0.0-dev

# make the fake build time deterministic if you want to compare outputs
BUILDTIME=2026-07-02_12:00:00 make build BUILDVERSION=v0.0.0-dev

# exercise the release packaging locally without publishing anything
goreleaser build --snapshot --clean
```

# execute example with debug output

```
./g10k -debug -config test.yaml
```

# testing

Integration tests verify the complete directory structure of deployed Puppet environments using snapshots. The test suite creates golden files that are committed to the repository.

### Regenerate snapshots

When integration test snapshots need to be updated (e.g., after intentional deployment changes), run:

```
# Generate new reference snapshots
go test -v -run 'TestResolveStaticWithSnapshot|TestResolveStaticChangesDetected|TestResolveStaticSkiplistWithSnapshot|TestSymlinkHandlingWithSnapshot|TestSymlinkStabilityAcrossRuns|TestConfigFullworkingMultipleSourcesWithSnapshot|TestFilePermissionsPreserved' -count=1

# Verify tests pass with new snapshots
go test -v -run 'TestResolveStaticWithSnapshot|TestResolveStaticChangesDetected|TestResolveStaticSkiplistWithSnapshot|TestSymlinkHandlingWithSnapshot|TestSymlinkStabilityAcrossRuns|TestConfigFullworkingMultipleSourcesWithSnapshot|TestFilePermissionsPreserved'
```

Snapshots are stored in `tests/snapshots/` as JSON files and should be committed along with the code changes.
