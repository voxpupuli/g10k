# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [v0.9.11-alpha] - 2025-11-06

### Fixed

- Fix data race
- Only delete environments inside the g10k basedir if the git/source remote matches (#182)

### Changed

- Add valid basedir even when it does not contain a Puppetfile
- Update vendor

## [v0.9.10] - 2025-02-14

### Changed

- Rework 2024/2025 (#224)
- Add git pull before doing anything

## [v0.9.9] - 2024-02-08

### Fixed

- Fix same branch name different sources (#222)
- Fix response code (#219, #221)

## [v0.9.8] - 2023-05-25

### Added

- Add support for NO_PROXY (#217)

## [v0.9.7] - 2023-02-01

### Fixed

- Fix ref with clone modules (#213, #214)

### Changed

- Always include debug symbols (#212)

## [v0.9.6] - 2023-01-19

### Fixed

- Remove debug output from filter command (#211)

## [v0.9.5] - 2022-12-01

### Added

- Support clone puppetfile (#210)

### Fixed

- Cache forge modules under its own cache directory in puppetfile mode (#207)
- Fix default moduledir not being overridden by moduledir parameter (#208)

### Changed

- Improve release process (#201)
- Update vendor (#206)

## [v0.9.4] - 2022-11-18

### Added

- Add support for strip_component (#204)

### Fixed

- Fix print formats
- Print better renaming statements

## [v0.9.3] - 2022-06-23

### Fixed

- Remove stale detection (#199)

## [v0.9.2] - 2022-05-25

### Fixed

- Fix .resource_types folder and content purge skip (#198)

## [v0.9.1] - 2022-05-25

### Changed

- Rename whitelist to allowlist

## [v0.9.0] - 2022-05-25

### Changed

- Rename lists and fix .resource_types purge (#196)

## [v0.8.17] - 2022-05-06

### Fixed

- Skip purge when -module parameter is used (#175, #183)
- Fix default Forge API URL and plain git test environments (#194)
- Fix issues #187 #189 and updates (#190)

### Changed

- Use golang.org/x/term
- Only execute git rev-parse once
- Switch to Github Actions (#192)
- Update vendor

## [v0.8.16] - 2021-08-14

### Security

- Update modules, including tidwall/gjson fixing CVE-2020-36066 and CVE-2020-35380

### Changed

- Add vendor

## [v0.8.15] - 2021-04-19

### Fixed

- Use the correct cache directories for modules and for environment even in mode

### Changed

- Update modules

## [v0.8.14] - 2021-04-19

### Fixed

- Fix staticcheck problems (#179)

## [v0.8.13] - 2021-03-31

### Added

- Add module parameter :use_ssh_agent (#177)
- Add error_if_branch_is_missing (#160)
- Add Makefile & Dockerfile (#168)

### Fixed

- Use ssh-add -K (Keychain) for Mac OS X (#176)

### Changed

- Remove vendor folder (#173)

## [v0.8.12] - 2020-08-21

### Added

- Add git_dir and git_url to .g10k-deploy.json
- Add purge_whitelist glob and double star glob pattern with filepathx (#169)
- Add branch filtering with filter_command and filter_regex (#167)

### Fixed

- Fix hashdeep tests with new git_dir and git_url fields

## [v0.8.11] - 2020-07-16

### Fixed

- Allow ssh key even for github.com repositories if it is the control repo (#165)
- Fix main.buildtime as single string

## [v0.8.10] - 2020-07-01

### Fixed

- Always purge and redeploy git modules in -puppetfile mode (#162)
- Corrupt local git repository detection improved

## [v0.8.9] - 2019-12-20

### Added

- Add clone_git_modules config setting (#151)

## [v0.8.8] - 2019-12-11

### Changed

- Output used Puppet environment for unresolvable git module repository
- Improve output for unresolvable Forge module/version

## [v0.8.7] - 2019-11-28

### Fixed

- Exit despite -retrygitcommands and invalid git reference (#156)

## [v0.8.6] - 2019-11-20

### Fixed

- Use the possibly renamed branch name instead of the original name (#154)
- Set invalid_branches default to correct_and_warn like r10k

## [v0.8.5] - 2019-10-08

### Changed

- Improve module deprecation check (#153)
- Add unchangedModuleDirs and addDesiredContent for control repository

## [v0.8.4] - 2019-10-01

### Fixed

- Detect symlinks (#150)
- Remove Puppetfile caching feature for now (#138)

## [v0.8.3] - 2019-09-25

### Fixed

- Always remove (sym)link if it exists and re-create it (#149)

## [v0.8.2] - 2019-09-18

### Fixed

- Fix race condition

## [v0.8.1] - 2019-09-18

### Fixed

- Fix race condition for managed content map

## [v0.8.0] - 2019-09-18

### Changed

- Big rework of file/dir paths (#146)
- Move purge/stale stuff to dedicated file
- Fix symlink creation bug
- Use branchParam as global variable

## [v0.7.4] - 2019-09-13

### Added

- Add purge_levels
- Check for deprecation notice (#148)

### Fixed

- Fix -puppetfile purging all modules on the subsequent run

## [v0.7.3] - 2019-09-12

### Fixed

- Stop purging content with -environment parameter (#147)
- Stop purging g10k deployfile
- Fix unnecessary sync of unchanged modules due to newline char

## [v0.7.2] - 2019-08-28

### Fixed

- Add hotfix for (sym)links creating bug

## [v0.7.1] - 2019-08-27

### Added

- Add purge_blacklist feature
- Add .g10k-deploy.json deploy file and detect Puppetfile changes (#138)
- Add -environment parameter (#132)

### Fixed

- Really respect `maxworker` parameter during git clone/remote update (#140, #141)

## [v0.7.0] - 2019-08-16

### Added

- Add support for purge_levels, purge_whitelist, and deployment_purge_whitelist (#139)
- Add resolveSourcePrefix() preparation for #132
- Check writability of configured CacheDir (#135)

### Changed

- Respect prefix for -branch parameter (#132)

## [v0.6.1] - 2019-07-02

### Fixed

- Exit if git commands fail (#130)
- Fix Git module with Forge notation (#131)

### Changed

- Add exclude_fields to improve Forge API performance

## [v0.6] - 2019-03-29

### Fixed

- Only warn when correct_and_warn if environment name is changed (#120)

## [v0.5.9] - 2019-03-29

### Fixed

- Fix :control_branch bug (#124, #125)
- Clean removed environments (#126)
- Fix minor typo in log message (#123)

## [v0.5.8] - 2019-02-01

### Fixed

- Fix -usemove feature (#119)

## [v0.5.7] - 2019-01-11

### Added

- Puppetfile validation (#118)

### Fixed

- Fix data race of env variable

## [v0.5.6] - 2018-11-13

### Changed

- Version bump

## [v0.5.5] - 2018-10-24

### Fixed

- Exclude .resource_types directory from purge (#115)

## [v0.5.4] - 2018-10-24

### Fixed

- Always initialize global maps (#116)
- Fix race condition for needSyncDirs

### Added

- Add modifiedenvs and modifieddirs variables to postrun command (#112, #113)

## [v0.5.3] - 2018-09-07

### Changed

- Version bump

## [v0.5.2] - 2018-08-24

### Added

- Add postrun command support (#100)
- Support multiple moduledir directives (#107)
- Support Git module in Forge notation (#104)
- Replace $environment with branch name in postrun command (#111)

### Fixed

- Fix empty -latest-last-checked from older g10k cache versions
- Check for dangling module attributes (#108, #89)

## [v0.5.1] - 2018-07-05

### Fixed

- Avoid git archive hanging due to trailing null bytes (#103, #98)

### Changed

- Reduce discarded tar message level to debug (#105)

## [v0.5] - 2018-06-26

### Fixed

- Avoid git archive hanging due to trailing null bytes (#103, #99)
- Fix false positive declaration of deprecated Forge modules

### Changed

- Cache Forge query result and module metadata response in the last-checked file

## [v0.4.9] - 2018-06-25

### Fixed

- Fix false positive declaration of deprecated Forge modules

## [v0.4.8] - 2018-06-25

### Changed

- Cache Forge query result and module metadata response in the last-checked file

## [v0.4.7] - 2018-06-07

### Added

- Add -gitobjectsyntaxnotsupported parameter (#95)

## [v0.4.63] - 2018-05-28

### Changed

- Version bump

## [v0.4.62] - 2018-05-28

### Changed

- Version bump

## [v0.4.61] - 2018-05-23

### Added

- Add -gitobjectsyntaxnotsupported parameter (#95)

## [v0.4.6] - 2018-05-17

### Added

- Add support for source setting invalid_branches for autocorrecting environment names like r10k (#81)
- Add g10k config setting git_object_syntax_not_supported (#95)
- Add tag and rename output option (#85)

### Fixed

- Fix incorrect file timestamps from Git modules
- Set atime and mtime for files of forge modules (#96)

### Changed

- Switch to debug verbosity level for ignore-unreachable output (#97)

## [v0.4.5] - 2018-03-06

### Fixed

- Add missing trailing / to targetDir (#94)
- Use /modules/ again for better query results (#88)

### Changed

- Add ^{object} to detect commit-ish looking hashes
- More robust git failure detection (#92)
- Print fatal errors to stderr instead of stdout (#87)

## [v0.4.4] - 2017-11-22

### Fixed

- Fix local modules and modules with install_path purging bug (#80, #82)

### Added

- Local modules support (#75)

## [v0.4.3] - 2017-11-17

### Added

- Add local module support

## [v0.4.2] - 2017-11-09

### Fixed

- Fix -retrygitcommands cli parameter in Puppetfile mode

## [v0.4.1] - 2017-11-08

### Added

- Add -retrygitcommands cli parameter or retry_git_commands g10k config setting (#76)

## [v0.4] - 2017-11-07

### Added

- Add -maxextractworker parameter/config setting (#79, #77, #76)

### Changed

- Remove any mention of drop-in replacement (#2, #78)

## [v0.3.14] - 2017-09-19

### Added

- Add :control_branch r10k Puppetfile setting

## [v0.3.13] - 2017-09-19

### Fixed

- Use /modules Forge API endpoint to get the latest release
- Fix fallback attribute and add default_branch r10k logic

## [v0.3.12] - 2017-09-01

### Added

- Add install_path git module attribute
- Add use_cache_fallback g10k config option

## [v0.3.11] - 2017-08-21

### Added

- Add -puppetfilelocation parameter
- Add ProxyCommand check and fix unique Forge module map

## [v0.3.10] - 2017-08-04

### Fixed

- Fix exit_if_unreachable section position

## [v0.3.9] - 2017-08-04

### Added

- Add exit_if_unreachable source config setting (#66)

## [v0.3.8] - 2017-08-02

### Added

- Add isDir() to check for directory

## [v0.3.7] - 2017-08-01

### Added

- Add -maxworker parameter to limit parallel Goroutines (#64)

### Fixed

- Fix build on Mac and possibly Windows (#61)

## [v0.3.6] - 2017-06-16

### Fixed

- Fix console color with Fatal logging (#59)

## [v0.3.5] - 2017-05-12

### Added

- Add ignore_unreachable_modules g10k config setting

### Fixed

- Fix conflict detection for Forge and Git module with the same name

### Changed

- Print wall time for Git and Forge module resolving

## [v0.3.4] - 2017-04-03

### Fixed

- Fix already existing Forge module directories
- Determine existing ref with rev-parse instead of log (#54)

## [v0.3.3] - 2017-03-23

### Fixed

- Fix file permission in unTar

## [v0.3.2] - 2017-03-23

### Fixed

- Fix regression with dash in Forge module version

## [v0.3.1] - 2017-03-22

### Fixed

- Fix unTar for git modules containing symlinks and hardlinks

## [v0.3.0] - 2017-03-22

### Added

- Add functionality test with hashdeep
- Add -cachedir param
- Add warn_if_branch_is_missing g10k config source setting

### Fixed

- Fix remote execution (#44)

### Changed

- Stop using bash -c and unnecessary exec's
- Remove ruby symbols from config YAML
- Remove go 1.3, 1.4 and 1.5 support

## [v0.2.9b] - 2017-03-16

### Added

- Add support for Forge module dash notation

## [v0.2.8b] - 2017-02-16

### Changed

- Version bump

## [v0.2.7] - 2017-02-16

### Added

- Add -moduledir parameter to override Puppetfile setting and allow absolute moduledir paths
- Add :sha256sum Forge module attribute
- Add force_forge_versions support
- Extract, download and checksum Forge modules in parallel

### Fixed

- Fix prefix source setting for modules

## [v0.2.6] - 2016-11-08

### Added

- Add fallback attribute for Git modules
- Add forge.cacheTtl feature

## [v0.2.5] - 2016-10-28

### Added

- Add forge.CacheTtl config setting for Puppetfile to skip :latest Forge API checks
- Add uiprogress bars for Forge and Git modules sync progress
- Add -checksum flag (#24)
- Add checksum verify for Forge module archives

### Fixed

- Fix prefix setting

## [v0.2.4] - 2016-08-22

### Changed

- Version bump

## [v0.2.3] - 2016-08-22

### Added

- Support inline comments in Puppetfile

### Changed

- Use faster gjson module
- Adjust ldflags parameter for go 1.7

## [v0.2.2] - 2016-08-11

### Added

- Add custom Forge base URL support in Puppetfile
- Add support for Forge API base URL

## [v0.2.1] - 2016-06-29

### Added

- Add -check4update param and colors

### Fixed

- Ensure to remove module directory if ignore-unreachable is set
- Support \_ as source name

## [v0.2.0] - 2016-03-18

### Added

- Add ignore-unreachable param for git modules
- Add -usemove param
- Add dryrun mode

### Changed

- Resolve Puppetlabs Forge and Git modules in parallel
- Only use branches and not references
- Restrict -usemove to -puppetfile mode

## [v0.1.3] - 2016-02-03

### Changed

- Version bump

## [v0.1.2] - 2016-02-03

### Changed

- Version bump

## [v0.1.1] - 2016-02-03

### Added

- Add puppetfile mode with -puppetfile that uses ./Puppetfile
- Module link support (#7)

## [v0.1.0] - 2016-01-04

### Added

- Initial release
- Puppetlabs Forge module support
- Git module support with :ref, :branch, :tag
- Parallel module resolution
- SSH key support
- Prune parameter to remove deleted remote references/branches
- Static file modes for Forge module extraction
