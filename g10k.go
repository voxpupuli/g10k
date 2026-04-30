package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	debug                        bool
	verbose                      bool
	info                         bool
	quiet                        bool
	force                        bool
	usemove                      bool
	usecacheFallback             bool
	retryGitCommands             bool
	pfMode                       bool
	pfLocation                   string
	clonegit                     bool
	dryRun                       bool
	validate                     bool
	check4update                 bool
	checkSum                     bool
	gitObjectSyntaxNotSupported  bool
	moduleDirParam               string
	cacheDirParam                string
	branchParam                  string
	environmentParam             string
	tags                         bool
	outputNameParam              string
	moduleParam                  string
	configFile                   string
	config                       ConfigSettings
	mutex                        sync.Mutex
	empty                        struct{}
	syncGitCount                 int
	syncForgeCount               int
	needSyncGitCount             int
	needSyncForgeCount           int
	needSyncDirs                 []string
	needSyncEnvs                 map[string]struct{}
	syncGitTime                  float64
	syncForgeTime                float64
	ioGitTime                    float64
	ioForgeTime                  float64
	forgeJSONParseTime           float64
	metadataJSONParseTime        float64
	buildtime                    string
	buildversion                 string
	uniqueForgeModules           map[string]ForgeModule
	latestForgeModules           LatestForgeModules
	maxworker                    int
	maxExtractworker             int
	forgeModuleDeprecationNotice string
)

// LatestForgeModules contains a map of unique Forge modules
// that should be the latest versions of them
type LatestForgeModules struct {
	sync.RWMutex
	m map[string]string
}

// ConfigSettings contains the key value pairs from the g10k config file
type ConfigSettings struct {
	CacheDir                    string `yaml:"cachedir"`
	ForgeCacheDir               string
	ModulesCacheDir             string
	EnvCacheDir                 string
	Git                         Git
	Sources                     map[string]Source
	Timeout                     int            `yaml:"timeout"`
	IgnoreUnreachableModules    bool           `yaml:"ignore_unreachable_modules"`
	Maxworker                   int            `yaml:"maxworker"`
	MaxExtractworker            int            `yaml:"maxextractworker"`
	UseCacheFallback            bool           `yaml:"use_cache_fallback"`
	RetryGitCommands            bool           `yaml:"retry_git_commands"`
	GitObjectSyntaxNotSupported bool           `yaml:"git_object_syntax_not_supported"`
	PostRunCommand              []string       `yaml:"postrun"`
	Deploy                      DeploySettings `yaml:"deploy"`
	PurgeLevels                 []string       `yaml:"purge_levels"`
	PurgeAllowList              []string       `yaml:"purge_allowlist"`
	DeploymentPurgeAllowList    []string       `yaml:"deployment_purge_allowlist"`
	WriteLock                   string         `yaml:"write_lock"`
	GenerateTypes               bool           `yaml:"generate_types"`
	PuppetPath                  string         `yaml:"puppet_path"`
	PurgeSkiplist               []string       `yaml:"purge_skiplist"`
	CloneGitModules             bool           `yaml:"clone_git_modules"`
	ForgeBaseURL                string         `yaml:"forge_base_url"`
	ForgeCacheTTLString         string         `yaml:"forge_cache_ttl"`
	ForgeCacheTTL               time.Duration
}

// DeploySettings is a struct for settings for controlling how g10k deploys behave.
// Trying to emulate r10k https://github.com/puppetlabs/r10k/blob/master/doc/dynamic-environments/configuration.mkd#deploy
type DeploySettings struct {
	PurgeLevels              []string `yaml:"purge_levels"`
	PurgeAllowList           []string `yaml:"purge_allowlist"`
	DeploymentPurgeAllowList []string `yaml:"deployment_purge_allowlist"`
	WriteLock                string   `yaml:"write_lock"`
	GenerateTypes            bool     `yaml:"generate_types"`
	PuppetPath               string   `yaml:"puppet_path"`
	PurgeSkiplist            []string `yaml:"purge_skiplist"`
}

// Forge is a simple struct that contains the base URL of
// the Forge that g10k should use. Defaults to: https://forgeapi.puppet.com
type Forge struct {
	Baseurl string `yaml:"baseurl"`
}

// Git is a simple struct that contains the optional SSH private key to
// use for authentication
type Git struct {
	privateKey string `yaml:"private_key"`
}

// Source contains basic information about a Puppet environment repository
type Source struct {
	Remote                      string
	Basedir                     string
	Prefix                      string
	PrivateKey                  string `yaml:"private_key"`
	ForceForgeVersions          bool   `yaml:"force_forge_versions"`
	WarnMissingBranch           bool   `yaml:"warn_if_branch_is_missing"`
	ErrorMissingBranch          bool   `yaml:"error_if_branch_is_missing"`
	ExitIfUnreachable           bool   `yaml:"exit_if_unreachable"`
	AutoCorrectEnvironmentNames string `yaml:"invalid_branches"`
	FilterCommand               string `yaml:"filter_command"`
	FilterRegex                 string `yaml:"filter_regex"`
	StripComponent              string `yaml:"strip_component"`
}

// Puppetfile contains the key value pairs from the Puppetfile
type Puppetfile struct {
	forgeBaseURL      string
	forgeCacheTTL     time.Duration
	forgeModules      map[string]ForgeModule
	gitModules        map[string]GitModule
	privateKey        string
	source            string
	sourceBranch      string
	workDir           string
	gitDir            string
	gitURL            string
	moduleDirs        []string
	controlRepoBranch string
}

// ForgeModule contains information (Version, Name, Author, md5 checksum, file size of the tar.gz archive, Forge BaseURL if custom) about a Puppetlabs Forge module
type ForgeModule struct {
	version      string
	name         string
	author       string
	md5sum       string
	fileSize     int64
	baseURL      string
	cacheTTL     time.Duration
	sha256sum    string
	moduleDir    string
	sourceBranch string
}

// GitModule contains information about a Git Puppet module
type GitModule struct {
	privateKey        string
	git               string
	branch            string
	tag               string
	commit            string
	ref               string
	tree              string
	link              bool
	ignoreUnreachable bool
	fallback          []string
	installPath       string
	local             bool
	moduleDir         string
	useSSHAgent       bool
}

// ForgeResult is returned by queryForgeAPI and contains if and which version of the Puppetlabs Forge module needs to be downloaded
type ForgeResult struct {
	needToGet     bool
	versionNumber string
	md5sum        string
	fileSize      int64
}

// ExecResult contains the exit code and output of an external command (e.g. git)
type ExecResult struct {
	returnCode int
	output     string
}

// DeployResult contains information about the Puppet environment which was deployed by g10k and tries to emulate the .r10k-deploy.json
type DeployResult struct {
	Name               string    `json:"name"`
	Signature          string    `json:"signature"`
	StartedAt          time.Time `json:"started_at"`
	FinishedAt         time.Time `json:"finished_at"`
	DeploySuccess      bool      `json:"deploy_success"`
	PuppetfileChecksum string    `json:"puppetfile_checksum"`
	GitDir             string    `json:"git_dir"`
	GitURL             string    `json:"git_url"`
}

func init() {
	// initialize global maps
	needSyncEnvs = make(map[string]struct{})
	uniqueForgeModules = make(map[string]ForgeModule)
}

func main() {

	var (
		configFileFlag = flag.String("config", "", "which config file to use")
		versionFlag    = flag.Bool("version", false, "show build time and version number")
	)
	flag.StringVar(&branchParam, "branch", "", "which git branch of the Puppet environment to update. Just the branch name, e.g. master, qa, dev")
	flag.StringVar(&environmentParam, "environment", "", "which Puppet environment to update. Source name inside the config + '_' + branch name, e.g. foo_master, foo_qa, foo_dev")
	flag.BoolVar(&tags, "tags", false, "to pull tags as well as branches")
	flag.StringVar(&outputNameParam, "outputname", "", "overwrite the environment name if -branch is specified")
	flag.StringVar(&moduleParam, "module", "", "which module of the Puppet environment to update, e.g. stdlib")
	flag.StringVar(&moduleDirParam, "moduledir", "", "allows overriding of Puppetfile specific moduledir setting, the folder in which Puppet modules will be extracted")
	flag.StringVar(&cacheDirParam, "cachedir", "", "allows overriding of the g10k config file cachedir setting, the folder in which g10k will download git repositories and Forge modules")
	flag.IntVar(&maxworker, "maxworker", 50, "how many Goroutines are allowed to run in parallel for Git and Forge module resolving")
	flag.IntVar(&maxExtractworker, "maxextractworker", 20, "how many Goroutines are allowed to run in parallel for local Git and Forge module extracting processes (git clone, untar and gunzip)")
	flag.BoolVar(&pfMode, "puppetfile", false, "install all modules from Puppetfile in cwd")
	flag.StringVar(&pfLocation, "puppetfilelocation", "./Puppetfile", "which Puppetfile to use in -puppetfile mode")
	flag.BoolVar(&clonegit, "clonegit", false, "populate the Puppet environment with a git clone of each git Puppet module. Helpful when developing locally with -puppetfile")
	flag.BoolVar(&force, "force", false, "purge the Puppet environment directory and do a full sync")
	flag.BoolVar(&dryRun, "dryrun", false, "do not modify anything, just print what would be changed")
	flag.BoolVar(&validate, "validate", false, "only validate given configuration and exit")
	flag.BoolVar(&usemove, "usemove", false, "do not use hardlinks to populate your Puppet environments with Puppetlabs Forge modules. Instead uses simple move commands and purges the Forge cache directory after each run! (Useful for g10k runs inside a Docker container)")
	flag.BoolVar(&check4update, "check4update", false, "only check if the is newer version of the Puppet module avaialable. Does implicitly set dryrun to true")
	flag.BoolVar(&checkSum, "checksum", false, "get the md5 check sum for each Puppetlabs Forge module and verify the integrity of the downloaded archive. Increases g10k run time!")
	flag.BoolVar(&debug, "debug", false, "log debug output, defaults to false")
	flag.BoolVar(&verbose, "verbose", false, "log verbose output, defaults to false")
	flag.BoolVar(&info, "info", false, "log info output, defaults to false")
	flag.BoolVar(&quiet, "quiet", false, "no output, defaults to false")
	flag.BoolVar(&usecacheFallback, "usecachefallback", false, "if g10k should try to use its cache for sources and modules instead of failing")
	flag.BoolVar(&retryGitCommands, "retrygitcommands", false, "if g10k should purge the local repository and retry a failed git command (clone or remote update) instead of failing")
	flag.BoolVar(&gitObjectSyntaxNotSupported, "gitobjectsyntaxnotsupported", false, "if your git version is too old to support reference syntax like master^{object} use this setting to revert to the older syntax")
	flag.Parse()

	configFile = *configFileFlag
	version := *versionFlag

	if version {
		fmt.Println("g10k ", buildversion, " Build time:", buildtime, "UTC")
		os.Exit(0)
	}

	if check4update {
		dryRun = true
	}

	// check for git executable dependency
	if _, err := exec.LookPath("git"); err != nil {
		Fatalf("Error: could not find 'git' executable in PATH")
	}

	target := ""
	before := time.Now()
	if len(configFile) > 0 {
		if usemove {
			Fatalf("Error: -usemove parameter is only allowed in -puppetfile mode!")
		}
		if pfMode {
			Fatalf("Error: -puppetfile parameter is not allowed with -config parameter!")
		}
		if (len(outputNameParam) > 0) && (len(branchParam) == 0) {
			Fatalf("Error: -outputname specified without -branch!")
		}
		if usecacheFallback {
			config.UseCacheFallback = true
		}
		Debugf("Using as config file: " + configFile)
		config = readConfigfile(configFile)
		checkDirAndCreate(config.CacheDir, "cachedir configured value")
		target = configFile
		if len(branchParam) > 0 {
			resolvePuppetEnvironment(tags, outputNameParam)
			target += " with branch " + branchParam
		} else {
			branchParam = ""
			resolvePuppetEnvironment(tags, "")
		}
	} else {
		if pfMode {
			Debugf("Trying to use as Puppetfile: " + pfLocation)
			sm := make(map[string]Source)
			sm["cmdlineparam"] = Source{Basedir: "./"}
			cachedir := os.Getenv("XDG_CACHE_HOME")
			if cachedir == "" {
				cachedir = filepath.Join(os.Getenv("HOME"), ".cache")
			}
			cachedir = filepath.Join(cachedir, "g10k")
			if len(os.Getenv("g10k_cachedir")) > 0 {
				cachedir = os.Getenv("g10k_cachedir")
				cachedir = checkDirAndCreate(cachedir, "cachedir environment variable g10k_cachedir")
				Debugf("Found environment variable g10k_cachedir set to: " + cachedir)
			} else if len(cacheDirParam) > 0 {
				Debugf("Using -cachedir parameter set to : " + cacheDirParam)
				cachedir = checkDirAndCreate(cacheDirParam, "cachedir CLI param")
			} else {
				cachedir = checkDirAndCreate(cachedir, "cachedir default value")
			}
			forgeCachedir := checkDirAndCreate(filepath.Join(cachedir, "forge"), "default in pfMode")
			modulesCacheDir := checkDirAndCreate(filepath.Join(cachedir, "modules"), "default in pfMode")
			envsCacheDir := checkDirAndCreate(filepath.Join(cachedir, "environments"), "default in pfMode")
			config = ConfigSettings{CacheDir: cachedir, ForgeCacheDir: forgeCachedir, ModulesCacheDir: modulesCacheDir, EnvCacheDir: envsCacheDir, Sources: sm, ForgeBaseURL: "https://forgeapi.puppet.com", Maxworker: maxworker, UseCacheFallback: usecacheFallback, MaxExtractworker: maxExtractworker, RetryGitCommands: retryGitCommands, GitObjectSyntaxNotSupported: gitObjectSyntaxNotSupported}
			// default purge_levels
			config.PurgeLevels = []string{"puppetfile"}
			if clonegit {
				config.CloneGitModules = true
			}
			target = pfLocation
			puppetfile := readPuppetfile(target, "", "cmdlineparam", "cmdlineparam", false, false)
			puppetfile.workDir = ""
			pfm := make(map[string]Puppetfile)
			pfm["cmdlineparam"] = puppetfile
			resolvePuppetfile(pfm)
		} else {
			Fatalf("Error: you need to specify at least a config file or use the Puppetfile mode\nExample call: " + os.Args[0] + " -config test.yaml or " + os.Args[0] + " -puppetfile\n")
		}
	}

	if usemove {
		// we can not reuse the Forge cache at all when -usemove gets used, because we can not delete the -latest link for some reason
		defer purgeDir(config.ForgeCacheDir, "main() -puppetfile mode with -usemove parameter")
	}

	Debugf("Forge response JSON parsing took " + strconv.FormatFloat(forgeJSONParseTime, 'f', 4, 64) + " seconds")
	Debugf("Forge modules metadata.json parsing took " + strconv.FormatFloat(metadataJSONParseTime, 'f', 4, 64) + " seconds")

	if !check4update && !quiet {
		if len(forgeModuleDeprecationNotice) > 0 {
			Warnf(strings.TrimSuffix(forgeModuleDeprecationNotice, "\n"))
		}
		fmt.Println("Synced", target, "with", syncGitCount, "git repositories and", syncForgeCount, "Forge modules in "+strconv.FormatFloat(time.Since(before).Seconds(), 'f', 1, 64)+"s with git ("+strconv.FormatFloat(syncGitTime, 'f', 1, 64)+"s sync, I/O", strconv.FormatFloat(ioGitTime, 'f', 1, 64)+"s) and Forge ("+strconv.FormatFloat(syncForgeTime, 'f', 1, 64)+"s query+download, I/O", strconv.FormatFloat(ioForgeTime, 'f', 1, 64)+"s) using", strconv.Itoa(config.Maxworker), "resolve and", strconv.Itoa(config.MaxExtractworker), "extract workers")
	}
	if dryRun && (needSyncForgeCount > 0 || needSyncGitCount > 0) {
		os.Exit(1)
	}

	checkForAndExecutePostrunCommand()
}
