package g10k

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

// Enum for LogLevel
// TODO: Unused right now.
type LogLevel string

const (
	DebugLevel   LogLevel = "debug"
	VerboseLevel LogLevel = "verbose"
	InfoLevel    LogLevel = "info"
	QuietLevel   LogLevel = "quiet"
)

// Options is the immutable, caller-supplied configuration for a run.
// All fields correspond 1:1 to the former CLI flags.
type Options struct {
	// mode
	ConfigFile string
	PFMode     bool
	PFLocation string
	// filters
	Branch      string // former branchParam
	Environment string // former environmentParam
	Module      string // former moduleParam
	OutputName  string // former outputNameParam
	ModuleDir   string // former moduleDirParam
	CacheDir    string // former cacheDirParam
	Tags        bool
	// behaviour toggles
	Force                       bool
	DryRun                      bool
	Validate                    bool
	Check4Update                bool
	CheckSum                    bool
	CloneGit                    bool
	UseMove                     bool
	UseCacheFallback            bool
	RetryGitCommands            bool
	GitObjectSyntaxNotSupported bool
	// verbosity
	Debug   bool
	Verbose bool
	Info    bool
	Quiet   bool
	// workers
	MaxWorker        int
	MaxExtractWorker int
	// build metadata (set by cmd/g10k via NewOptionsWithBuild)
	BuildVersion string
	BuildTime    string
}

// Runtime holds all the mutable state run touches.
// Construct one per invocation with NewRuntime(opts).
type Runtime struct {
	Options
	Config ConfigSettings
	Mutex  sync.Mutex
	empty  struct{}

	SyncGitCount       int
	SyncForgeCount     int
	NeedSyncGitCount   int
	NeedSyncForgeCount int
	NeedSyncDirs       []string
	NeedSyncEnvs       map[string]struct{}

	StartTime             time.Time
	SyncGitTime           float64
	SyncForgeTime         float64
	IoGitTime             float64
	IoForgeTime           float64
	ForgeJSONParseTime    float64
	metadataJSONParseTime float64

	UniqueForgeModules           map[string]ForgeModule
	LatestForgeModules           *LatestForgeModules
	forgeModuleDeprecationNotice string
	ValidationMessages           []string
}

// NewRuntime takes in an Options struct and returns a
// new pointer to a Runtime struct.
func NewRuntime(opts Options) *Runtime {
	return &Runtime{
		Options:            opts,
		NeedSyncEnvs:       make(map[string]struct{}),
		UniqueForgeModules: make(map[string]ForgeModule),
		LatestForgeModules: &LatestForgeModules{m: make(map[string]string)},
		StartTime:          time.Now(),
	}
}

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

func main() {

	var (
		configFileFlag = flag.String("config", "", "which config file to use")
		versionFlag    = flag.Bool("version", false, "show build time and version number")
		options        Options
	)

	flag.StringVar(&options.Branch, "branch", "", "which git branch of the Puppet environment to update. Just the branch name, e.g. master, qa, dev")
	flag.StringVar(&options.Environment, "environment", "", "which Puppet environment to update. Source name inside the config + '_' + branch name, e.g. foo_master, foo_qa, foo_dev")
	flag.BoolVar(&options.Tags, "tags", false, "to pull tags as well as branches")
	flag.StringVar(&options.OutputName, "outputname", "", "overwrite the environment name if -branch is specified")
	flag.StringVar(&options.Module, "module", "", "which module of the Puppet environment to update, e.g. stdlib")
	flag.StringVar(&options.ModuleDir, "moduledir", "", "allows overriding of Puppetfile specific moduledir setting, the folder in which Puppet modules will be extracted")
	flag.StringVar(&options.CacheDir, "cachedir", "", "allows overriding of the g10k config file cachedir setting, the folder in which g10k will download git repositories and Forge modules")
	flag.IntVar(&options.MaxWorker, "maxworker", 50, "how many Goroutines are allowed to run in parallel for Git and Forge module resolving")
	flag.IntVar(&options.MaxExtractWorker, "maxextractworker", 20, "how many Goroutines are allowed to run in parallel for local Git and Forge module extracting processes (git clone, untar and gunzip)")
	flag.BoolVar(&options.PFMode, "puppetfile", false, "install all modules from Puppetfile in cwd")
	flag.StringVar(&options.PFLocation, "puppetfilelocation", "./Puppetfile", "which Puppetfile to use in -puppetfile mode")
	flag.BoolVar(&options.CloneGit, "clonegit", false, "populate the Puppet environment with a git clone of each git Puppet module. Helpful when developing locally with -puppetfile")
	flag.BoolVar(&options.Force, "force", false, "purge the Puppet environment directory and do a full sync")
	flag.BoolVar(&options.DryRun, "dryrun", false, "do not modify anything, just print what would be changed")
	flag.BoolVar(&options.Validate, "validate", false, "only validate given configuration and exit")
	flag.BoolVar(&options.UseMove, "usemove", false, "do not use hardlinks to populate your Puppet environments with Puppetlabs Forge modules. Instead uses simple move commands and purges the Forge cache directory after each run! (Useful for g10k runs inside a Docker container)")
	flag.BoolVar(&options.Check4Update, "check4update", false, "only check if the is newer version of the Puppet module avaialable. Does implicitly set dryrun to true")
	flag.BoolVar(&options.CheckSum, "checksum", false, "get the md5 check sum for each Puppetlabs Forge module and verify the integrity of the downloaded archive. Increases g10k run time!")
	flag.BoolVar(&options.Debug, "debug", false, "log debug output, defaults to false")
	flag.BoolVar(&options.Verbose, "verbose", false, "log verbose output, defaults to false")
	flag.BoolVar(&options.Info, "info", false, "log info output, defaults to false")
	flag.BoolVar(&options.Quiet, "quiet", false, "no output, defaults to false")
	flag.BoolVar(&options.UseCacheFallback, "usecachefallback", false, "if g10k should try to use its cache for sources and modules instead of failing")
	flag.BoolVar(&options.RetryGitCommands, "retrygitcommands", false, "if g10k should purge the local repository and retry a failed git command (clone or remote update) instead of failing")
	flag.BoolVar(&options.GitObjectSyntaxNotSupported, "gitobjectsyntaxnotsupported", false, "if your git version is too old to support reference syntax like master^{object} use this setting to revert to the older syntax")
	flag.Parse()

	options.ConfigFile = *configFileFlag
	version := *versionFlag

	if version {
		fmt.Println("g10k ", options.BuildVersion, " Build time:", options.BuildTime, "UTC")
		os.Exit(0)
	}

	exitCode, err := Run(options)
	if err != nil {
		fmt.Println(err)
		os.Exit(exitCode)
	}
}

func Run(opts Options) (int, error) {
	if opts.Check4Update {
		opts.DryRun = true
	}

	if _, err := exec.LookPath("git"); err != nil {
		return 1, fmt.Errorf("could not find `git` executable in PATH")
	}

	rt := NewRuntime(opts)

	if opts.ConfigFile != "" {
		if opts.UseMove {
			return 1, fmt.Errorf("error: -usemove is only allowed in puppetfile mode")
		}
		if opts.PFMode {
			return 1, fmt.Errorf("error: -puppetfile parameter not allowed with -config parameter")
		}
		if opts.OutputName != "" && opts.Branch == "" {
			return 1, fmt.Errorf("error: -outputname specified without a branch")
		}
		if opts.UseCacheFallback {
			rt.Config.UseCacheFallback = true
		}

		cfg, err := rt.readConfigfile(opts.ConfigFile)
		if err != nil {
			return 1, err
		}
		rt.Config = cfg
		rt.checkDirAndCreate(rt.Config.CacheDir, "cachedir configured value")
		target := opts.ConfigFile
		if opts.Branch != "" {
			if err := rt.resolvePuppetEnvironment(rt.Tags, rt.OutputName); err != nil {
				return 1, err
			}
			target += " with branch " + opts.Branch
		} else {
			if err := rt.resolvePuppetEnvironment(rt.Tags, ""); err != nil {
				return 1, err
			}
		}
		return rt.finish(target)
	}
	if opts.PFMode {
		rt.Debugf("Trying to use as Puppetfile: " + rt.PFLocation)
		sm := make(map[string]Source)
		sm["cmdlineparam"] = Source{Basedir: "./"}
		cachebase, err := os.UserCacheDir()
		if err != nil {
			cachebase = "/tmp"
		}
		cachedir := filepath.Join(cachebase, "g10k")
		if len(os.Getenv("g10k_cachedir")) > 0 {
			cachedir = os.Getenv("g10k_cachedir")
			cachedir = rt.checkDirAndCreate(cachedir, "cachedir environment variable g10k_cachedir")
			rt.Debugf("Found environment variable g10k_cachedir set to: " + cachedir)
		} else if len(rt.CacheDir) > 0 {
			rt.Debugf("Using -cachedir parameter set to : " + rt.CacheDir)
			cachedir = rt.checkDirAndCreate(rt.CacheDir, "cachedir CLI param")
		} else {
			cachedir = rt.checkDirAndCreate(cachedir, "cachedir default value")
		}
		forgeCachedir := rt.checkDirAndCreate(filepath.Join(cachedir, "forge"), "default in pfMode")
		modulesCacheDir := rt.checkDirAndCreate(filepath.Join(cachedir, "modules"), "default in pfMode")
		envsCacheDir := rt.checkDirAndCreate(filepath.Join(cachedir, "environments"), "default in pfMode")
		rt.Config = ConfigSettings{CacheDir: cachedir, ForgeCacheDir: forgeCachedir, ModulesCacheDir: modulesCacheDir, EnvCacheDir: envsCacheDir, Sources: sm, ForgeBaseURL: "https://forgeapi.puppet.com", Maxworker: rt.MaxWorker, UseCacheFallback: rt.UseCacheFallback, MaxExtractworker: rt.MaxExtractWorker, RetryGitCommands: rt.RetryGitCommands, GitObjectSyntaxNotSupported: rt.GitObjectSyntaxNotSupported}
		// default purge_levels
		rt.Config.PurgeLevels = []string{"puppetfile"}
		if rt.CloneGit {
			rt.Config.CloneGitModules = true
		}
		target := rt.PFLocation
		puppetfile, err := rt.readPuppetfile(target, "", "cmdlineparam", "cmdlineparam", false, false)
		if err != nil {
			return 1, err
		}
		puppetfile.workDir = ""
		pfm := make(map[string]Puppetfile)
		pfm["cmdlineparam"] = puppetfile
		rt.resolvePuppetfile(pfm)
		return rt.finish(target)
	}
	return 1, fmt.Errorf("you need to specify at least a config file or use the Puppetfile mode")
}

func (rt *Runtime) finish(target string) (int, error) {
	if rt.UseMove {
		// we can not reuse the Forge cache at all when -usemove gets used, because we can not delete the -latest link for some reason
		defer rt.purgeDir(rt.Config.ForgeCacheDir, "main() -puppetfile mode with -usemove parameter")
	}

	rt.Debugf("Forge response JSON parsing took " + strconv.FormatFloat(rt.ForgeJSONParseTime, 'f', 4, 64) + " seconds")
	rt.Debugf("Forge modules metadata.json parsing took " + strconv.FormatFloat(rt.metadataJSONParseTime, 'f', 4, 64) + " seconds")

	if !rt.Check4Update && !rt.Quiet {
		if len(rt.forgeModuleDeprecationNotice) > 0 {
			rt.Warnf(strings.TrimSuffix(rt.forgeModuleDeprecationNotice, "\n"))
		}
		fmt.Println("Synced", target, "with", rt.SyncGitCount, "git repositories and", rt.SyncForgeCount, "Forge modules in "+strconv.FormatFloat(time.Since(rt.StartTime).Seconds(), 'f', 1, 64)+"s with git ("+strconv.FormatFloat(rt.SyncGitTime, 'f', 1, 64)+"s sync, I/O", strconv.FormatFloat(rt.IoGitTime, 'f', 1, 64)+"s) and Forge ("+strconv.FormatFloat(rt.SyncForgeTime, 'f', 1, 64)+"s query+download, I/O", strconv.FormatFloat(rt.IoForgeTime, 'f', 1, 64)+"s) using", strconv.Itoa(rt.Config.Maxworker), "resolve and", strconv.Itoa(rt.Config.MaxExtractworker), "extract workers")
	}
	if rt.DryRun && (rt.NeedSyncForgeCount > 0 || rt.NeedSyncGitCount > 0) {
		return 1, nil
	}

	rt.checkForAndExecutePostrunCommand()
	return 0, nil
}
