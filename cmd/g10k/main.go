package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/voxpupuli/g10k/pkg/g10k"
)

var (
	buildtime    string
	buildversion string
)

func main() {
	var (
		configFileFlag = flag.String("config", "", "which config file to use")
		versionFlag    = flag.Bool("version", false, "show build time and version number")
	)
	var opts g10k.Options

	flag.StringVar(&opts.Branch, "branch", "", "which git branch of the Puppet environment to update. Just the branch name, e.g. master, qa, dev")
	flag.StringVar(&opts.Environment, "environment", "", "which Puppet environment to update. Source name inside the config + '_' + branch name, e.g. foo_master, foo_qa, foo_dev")
	flag.BoolVar(&opts.Tags, "tags", false, "to pull tags as well as branches")
	flag.StringVar(&opts.OutputName, "outputname", "", "overwrite the environment name if -branch is specified")
	flag.StringVar(&opts.Module, "module", "", "which module of the Puppet environment to update, e.g. stdlib")
	flag.StringVar(&opts.ModuleDir, "moduledir", "", "allows overriding of Puppetfile specific moduledir setting")
	flag.StringVar(&opts.CacheDir, "cachedir", "", "allows overriding of the g10k config file cachedir setting")
	flag.IntVar(&opts.MaxWorker, "maxworker", 50, "how many Goroutines are allowed to run in parallel for Git and Forge module resolving")
	flag.IntVar(&opts.MaxExtractWorker, "maxextractworker", 20, "how many Goroutines are allowed to run in parallel for local Git and Forge module extracting processes")
	flag.BoolVar(&opts.PFMode, "puppetfile", false, "install all modules from Puppetfile in cwd")
	flag.StringVar(&opts.PFLocation, "puppetfilelocation", "./Puppetfile", "which Puppetfile to use in -puppetfile mode")
	flag.BoolVar(&opts.CloneGit, "clonegit", false, "populate the Puppet environment with a git clone of each git Puppet module. Helpful when developing locally with -puppetfile")
	flag.BoolVar(&opts.Force, "force", false, "purge the Puppet environment directory and do a full sync")
	flag.BoolVar(&opts.DryRun, "dryrun", false, "do not modify anything, just print what would be changed")
	flag.BoolVar(&opts.Validate, "validate", false, "only validate given configuration and exit")
	flag.BoolVar(&opts.UseMove, "usemove", false, "do not use hardlinks to populate your Puppet environments with Puppetlabs Forge modules...")
	flag.BoolVar(&opts.Check4Update, "check4update", false, "only check if there is a newer version of the Puppet module available...")
	flag.BoolVar(&opts.CheckSum, "checksum", false, "get the md5 check sum for each Puppetlabs Forge module...")
	flag.BoolVar(&opts.Debug, "debug", false, "log debug output, defaults to false")
	flag.BoolVar(&opts.Verbose, "verbose", false, "log verbose output, defaults to false")
	flag.BoolVar(&opts.Info, "info", false, "log info output, defaults to false")
	flag.BoolVar(&opts.Quiet, "quiet", false, "no output, defaults to false")
	flag.BoolVar(&opts.UseCacheFallback, "usecachefallback", false, "if g10k should try to use its cache for sources and modules instead of failing")
	flag.BoolVar(&opts.RetryGitCommands, "retrygitcommands", false, "if g10k should purge the local repository and retry a failed git command...")
	flag.BoolVar(&opts.GitObjectSyntaxNotSupported, "gitobjectsyntaxnotsupported", false, "if your git version is too old to support reference syntax like master^{object}...")
	flag.Parse()

	opts.ConfigFile = *configFileFlag
	if *versionFlag {
		fmt.Println("g10k ", buildversion, " Build time:", buildtime, "UTC")
		os.Exit(0)
	}
	opts.BuildVersion = buildversion
	opts.BuildTime = buildtime

	code, err := g10k.Run(opts)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
	}
	os.Exit(code)
}
