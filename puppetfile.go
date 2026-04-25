package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/remeh/sizedwaitgroup"
	"github.com/xorpaul/uiprogress"
	"golang.org/x/term"
)

// sourceSanityCheck is a validation function that checks if the given source has all necessary attributes (basedir, remote, SSH key exists if given)
func sourceSanityCheck(source string, sa Source) {
	if len(sa.PrivateKey) > 0 {
		if _, err := os.Stat(sa.PrivateKey); err != nil {
			Fatalf("resolvePuppetEnvironment(): could not find SSH private key " + sa.PrivateKey + " for source " + source + " in config file " + configFile + " Error: " + err.Error())
		}
	}
	if len(sa.Basedir) <= 0 {
		Fatalf("resolvePuppetEnvironment(): config setting basedir is not set for source " + source + " in config file " + configFile)
	}
	if len(sa.Remote) <= 0 {
		Fatalf("resolvePuppetEnvironment(): config setting remote is not set for source " + source + " in config file " + configFile)
	}
}

func resolvePuppetEnvironment(tags bool, outputNameTag string) {
	wg := sizedwaitgroup.New(config.MaxExtractworker + 1)
	allPuppetfiles := make(map[string]Puppetfile)
	allEnvironments := make(map[string]bool)
	allBasedirs := make(map[string]bool)
	foundMatch := false
	for source, sa := range config.Sources {
		wg.Add()
		go func(source string, sa Source) {
			defer wg.Done()
			if force {
				createOrPurgeDir(sa.Basedir, "resolvePuppetEnvironment()")
			}

			sa.Basedir = checkDirAndCreate(sa.Basedir, "basedir for source "+source)
			Debugf("Puppet environment: " + source + " (" + fmt.Sprintf("%+v", sa) + ")")

			// check for a valid source that has all necessary attributes (basedir, remote, SSH key exist if given)
			sourceSanityCheck(source, sa)

			workDir := filepath.Join(config.EnvCacheDir, source+".git")
			// check if sa.Basedir exists
			checkDirAndCreate(sa.Basedir, "basedir")

			controlRepoGit := GitModule{}
			controlRepoGit.git = sa.Remote
			controlRepoGit.privateKey = sa.PrivateKey
			if success := doMirrorOrUpdate(controlRepoGit, workDir, 0); success {

				// get all branches
				er := executeCommand("git --git-dir "+workDir+" branch", "", config.Timeout, false, false)
				outputBranches := er.output
				outputTags := ""

				if tags {
					er := executeCommand("git --git-dir "+workDir+" tag", "", config.Timeout, false, false)
					outputTags = er.output
				}

				branches := strings.Split(strings.TrimSpace(outputBranches+outputTags), "\n")

				foundBranch := false
				prefix := resolveSourcePrefix(source, sa)
				for _, branch := range branches {
					branch = strings.TrimLeft(branch, "* ")
					reInvalidCharacters := regexp.MustCompile(`\W`)
					if sa.AutoCorrectEnvironmentNames == "error" && reInvalidCharacters.MatchString(branch) {
						Warnf("Ignoring branch " + branch + ", because it contains invalid characters")
						continue
					}
					// XXX: maybe make this user configurable (either with dedicated file or as YAML array in g10k config)
					if strings.Contains(branch, ";") || strings.Contains(branch, "&") || strings.Contains(branch, "|") || strings.HasPrefix(branch, "tmp/") && strings.HasSuffix(branch, "/head") {
						Debugf("Skipping branch " + branch + " of source " + source + ", because of invalid character(s) inside the branch name")
						continue
					}

					if len(sa.FilterCommand) > 0 {
						if skipBasedOnFilterCommand(branch, source, sa, workDir) {
							Debugf("Skipping branch " + branch + " of source " + source + ", because of filter_command setting")
							continue
						}
					}
					if len(sa.FilterRegex) > 0 {
						if skipBasedOnFilterRegex(branch, source, sa, workDir) {
							Debugf("Skipping branch " + branch + " of source " + source + ", because of filter_regex setting")
							continue
						}
					}

					if len(branchParam) > 0 {
						if branch == branchParam {
							foundBranch = true
						} else {
							Debugf("Environment " + prefix + branch + " of source " + source + " does not match branch name filter '" + branchParam + "', skipping")
							continue
						}
					} else if len(environmentParam) > 0 {
						if source+"_"+branch == environmentParam {
							foundMatch = true
						} else {
							Debugf("Environment " + prefix + branch + " of source " + source + " does not match environment name filter '" + environmentParam + "', skipping")
							continue
						}
					}

					wg.Add()

					go func(branch string, sa Source, prefix string) {
						defer wg.Done()
						if len(branch) != 0 {
							Debugf("Resolving environment " + prefix + branch + " of source " + source)

							renamedBranch := branch
							if (len(outputNameTag) > 0) && (len(branchParam) > 0) {
								renamedBranch = outputNameTag
								Debugf("Renaming branch " + branch + " to " + renamedBranch + " from source " + source + " " + sa.Remote)
							}

							// https://github.com/puppetlabs/r10k/blob/main/doc/dynamic-environments/configuration.mkd#strip_component
							if len(sa.StripComponent) != 0 {
								stripRenamedBranch := stripComponent(sa.StripComponent, renamedBranch)
								if stripRenamedBranch != renamedBranch {
									// only print this if the branch was definately renamed, because of the strip component
									Debugf("Renaming branch " + renamedBranch + " to " + stripRenamedBranch + ", because of strip_component in source " + source + " " + sa.Remote)
									renamedBranch = stripRenamedBranch
								}
							}

							if sa.AutoCorrectEnvironmentNames == "correct" || sa.AutoCorrectEnvironmentNames == "correct_and_warn" {
								oldBranch := renamedBranch
								renamedBranch = reInvalidCharacters.ReplaceAllString(renamedBranch, "_")
								if oldBranch != renamedBranch {
									if sa.AutoCorrectEnvironmentNames == "correct_and_warn" {
										Warnf("Renaming branch " + oldBranch + " to " + renamedBranch + " from source " + source + " " + sa.Remote)
									} else {
										Debugf("Renaming branch " + oldBranch + " to " + renamedBranch + " from source " + source + " " + sa.Remote)
									}
								}
							}

							mutex.Lock()
							if _, ok := allEnvironments[filepath.Join(sa.Basedir, prefix+renamedBranch)]; !ok {
								allEnvironments[filepath.Join(sa.Basedir, prefix+renamedBranch)] = true
							} else {
								Fatalf("Renamed environment naming conflict detected with renamed environment " + prefix + renamedBranch)
							}
							mutex.Unlock()
							targetDir := filepath.Join(sa.Basedir, prefix+strings.ReplaceAll(renamedBranch, "/", "_"))
							targetDir = normalizeDir(targetDir)

							env := strings.ReplaceAll(strings.Replace(targetDir, sa.Basedir, "", 1), "/", "")
							if len(moduleParam) == 0 {
								gitModule := GitModule{}
								gitModule.tree = branch
								syncToModuleDir(gitModule, workDir, targetDir, env)
							}
							pf := filepath.Join(targetDir, "Puppetfile")
							mutex.Lock()
							allBasedirs[sa.Basedir] = true
							mutex.Unlock()
							if !fileExists(pf) {
								Debugf("resolvePuppetEnvironment(): Skipping branch " + source + "_" + branch + " because " + pf + " does not exist")
								deployFile := filepath.Join(targetDir, ".g10k-deploy.json")
								if fileExists(deployFile) {
									Debugf("Finishing writing to deploy file " + deployFile)
									dr := readDeployResultFile(deployFile)
									dr.DeploySuccess = true
									dr.FinishedAt = time.Now()
									dr.GitDir = sa.Basedir
									dr.GitURL = sa.Remote
									writeStructJSONFile(deployFile, dr)
								}
							} else {
								puppetfile := readPuppetfile(pf, sa.PrivateKey, source, branch, sa.ForceForgeVersions, false)
								puppetfile.workDir = normalizeDir(targetDir)
								puppetfile.controlRepoBranch = branch
								puppetfile.gitDir = workDir
								puppetfile.gitURL = sa.Remote
								mutex.Lock()
								for _, moduleDir := range puppetfile.moduleDirs {
									checkDirAndCreate(filepath.Join(puppetfile.workDir, moduleDir), "moduledir for env")
								}
								allPuppetfiles[env] = puppetfile
								mutex.Unlock()

							}
						}
					}(branch, sa, prefix)
				}

				if sa.ErrorMissingBranch && !foundBranch {
					Fatalf("Couldn't find specified branch '" + branchParam + "' anywhere in source '" + source + "' (" + sa.Remote + ")")
				} else if sa.WarnMissingBranch && !foundBranch {
					Warnf("WARNING: Couldn't find specified branch '" + branchParam + "' anywhere in source '" + source + "' (" + sa.Remote + ")")
				}
			} else {
				Warnf("WARNING: Could not resolve git repository in source '" + source + "' (" + sa.Remote + ")")
				if sa.ExitIfUnreachable {
					os.Exit(1)
				}
			}
		}(source, sa)
	}

	wg.Wait()
	if len(environmentParam) > 0 {
		if !foundMatch {
			Warnf("WARNING: Environment '" + environmentParam + "' cannot be found in any source and will not be deployed.")
		}
	}
	//fmt.Println("allPuppetfiles: ", allPuppetfiles, len(allPuppetfiles))
	//fmt.Println("allPuppetfiles[0]: ", allPuppetfiles["postinstall"])
	resolvePuppetfile(allPuppetfiles)
	// fmt.Printf("%+v\n", allEnvironments)
	if len(moduleParam) == 0 {
		purgeUnmanagedContent(allBasedirs, allEnvironments)
	}
}

// resolveSourcePrefix implements the prefix read out from each source given in the config file, like r10k https://github.com/puppetlabs/r10k/blob/master/doc/dynamic-environments/configuration.mkd#prefix
func resolveSourcePrefix(source string, sa Source) string {
	switch sa.Prefix {
	case "false", "":
		return ""
	case "true":
		return source + "_"
	default:
		return sa.Prefix + "_"
	}
}

func resolvePuppetfile(allPuppetfiles map[string]Puppetfile) {
	wg := sizedwaitgroup.New(config.MaxExtractworker)
	exisitingModuleDirs := make(map[string]struct{})
	uniqueGitModules := make(map[string]GitModule)
	// if we made it this far initialize the global maps
	latestForgeModules.m = make(map[string]string)
	for env, pf := range allPuppetfiles {
		Debugf("Resolving branch " + env + " of source " + pf.source)
		//fmt.Println(pf)
		for gitName, gitModule := range pf.gitModules {
			if len(moduleParam) > 0 {
				if gitName != moduleParam {
					Debugf("Skipping git module " + gitName + ", because parameter -module is set to " + moduleParam)
					delete(pf.gitModules, gitName)
					continue
				}
			}
			if gitModule.local {
				continue
			}

			gitModule.privateKey = pf.privateKey
			if _, ok := uniqueGitModules[gitModule.git]; !ok {
				uniqueGitModules[gitModule.git] = gitModule
			}
		}
		for forgeModuleName, fm := range pf.forgeModules {
			if len(moduleParam) > 0 {
				if forgeModuleName != moduleParam {
					Debugf("Skipping forge module " + forgeModuleName + ", because parameter -module is set to " + moduleParam)
					delete(pf.forgeModules, forgeModuleName)
					continue
				}
			}
			fm.baseURL = pf.forgeBaseURL
			if pf.forgeCacheTTL != 0 {
				fm.cacheTTL = pf.forgeCacheTTL
			} else {
				fm.cacheTTL = config.ForgeCacheTTL
			}
			// fmt.Println("Found Forge module", fm.author, "/", forgeModuleName, "with version", fm.version, "and cacheTTL", fm.cacheTTL)
			forgeModuleName = strings.ReplaceAll(forgeModuleName, "/", "-")
			uniqueForgeModuleName := fm.author + "/" + forgeModuleName + "-" + fm.version
			if _, ok := uniqueForgeModules[uniqueForgeModuleName]; !ok {
				uniqueForgeModules[uniqueForgeModuleName] = fm
			} else {
				// Use the shortest Forge cache TTL for this module
				if uniqueForgeModules[uniqueForgeModuleName].cacheTTL > pf.forgeCacheTTL {
					delete(uniqueForgeModules, uniqueForgeModuleName)
					uniqueForgeModules[uniqueForgeModuleName] = fm
				}
			}
		}
	}
	if !debug && !verbose && !info && !quiet && term.IsTerminal(int(os.Stdout.Fd())) {
		uiprogress.Start()
	}
	var wgResolve sync.WaitGroup
	wgResolve.Add(2)
	go func() {
		defer wgResolve.Done()
		resolveGitRepositories(uniqueGitModules)
	}()
	go func() {
		defer wgResolve.Done()
		resolveForgeModules(uniqueForgeModules)
	}()
	wgResolve.Wait()
	//log.Println(config.Sources["cmdlineparam"])
	for env, pf := range allPuppetfiles {
		Debugf("Syncing " + env + " with workDir " + pf.workDir)
		// this prevents g10k from purging module directories on the subsequent run in -puppetfile mode
		basedir := ""
		if !pfMode {
			basedir = checkDirAndCreate(pf.workDir, "basedir 2 for source "+pf.source)
		}

		for _, moduleDir := range pf.moduleDirs {
			moduleDir = normalizeDir(filepath.Join(pf.workDir, moduleDir))
			exisitingModuleDirsFI, _ := os.ReadDir(moduleDir)
			mutex.Lock()
			for _, exisitingModuleDir := range exisitingModuleDirsFI {
				// fmt.Println("adding dir: ", filepath.Join(moduleDir, exisitingModuleDir.Name()))
				exisitingModuleDirs[filepath.Join(moduleDir, exisitingModuleDir.Name())] = empty
			}
			mutex.Unlock()
		}

		for gitName, gitModule := range pf.gitModules {
			moduleDir := filepath.Join(pf.workDir, gitModule.moduleDir)
			moduleDir = normalizeDir(moduleDir)
			if gitModule.local {
				moduleDirectory := filepath.Join(moduleDir, gitName)
				Debugf("Not deleting " + moduleDirectory + " as it is declared as a local module")
				// remove this module from the exisitingModuleDirs map
				if len(gitModule.installPath) > 0 {
					moduleDirectory = filepath.Join(normalizeDir(basedir), normalizeDir(gitModule.installPath), gitName)
				}
				moduleDirectory = normalizeDir(moduleDirectory)
				mutex.Lock()
				delete(exisitingModuleDirs, moduleDirectory)
				for existingDir := range exisitingModuleDirs {
					rel, _ := filepath.Rel(existingDir, moduleDirectory)
					if len(rel) > 0 && !strings.Contains(rel, "..") {
						Debugf("not removing moduleDirectory " + moduleDirectory + " because it's a subdirectory to existingDir " + existingDir)
						delete(exisitingModuleDirs, existingDir)
					}
				}
				mutex.Unlock()
				continue
			}
			wg.Add()
			go func(gitName string, gitModule GitModule, env string, pf Puppetfile) {
				defer wg.Done()
				targetDir := normalizeDir(filepath.Join(moduleDir, gitName))
				moduleCacheDir := filepath.Join(config.ModulesCacheDir, strings.ReplaceAll(strings.ReplaceAll(gitModule.git, "/", "_"), ":", "-"))
				tree := detectDefaultBranch(moduleCacheDir)
				Debugf("Setting " + tree + " as default branch for " + gitModule.git)
				if len(gitModule.branch) > 0 {
					tree = gitModule.branch
				} else if len(gitModule.commit) > 0 {
					tree = gitModule.commit
				} else if len(gitModule.tag) > 0 {
					tree = gitModule.tag
				} else if len(gitModule.ref) > 0 {
					tree = gitModule.ref
				} else if gitModule.link {
					if pfMode {
						if len(os.Getenv("g10k_branch")) > 0 {
							tree = os.Getenv("g10k_branch")
						} else if len(branchParam) > 0 {
							tree = branchParam
						} else {
							Fatalf("resolvePuppetfile(): found module " + gitName + " with module link mode enabled and g10k in Puppetfile mode which is not supported, as g10k can not detect the environment branch of the Puppetfile. You can explicitly set the module link branch you want to use in Puppetfile mode by setting the environment variable 'g10k_branch' or using the -branch parameter")
						}
					} else {
						// we want only the branch name of the control repo and not the resulting
						// Puppet environment folder name, which could contain a prefix
						tree = pf.controlRepoBranch
					}
				}

				if len(gitModule.installPath) > 0 {
					targetDir = filepath.Join(basedir, normalizeDir(gitModule.installPath), gitName)
				}
				targetDir = normalizeDir(targetDir)
				success := false

				if gitModule.link {
					Debugf("Trying to resolve " + moduleCacheDir + " with branch " + tree)
					gitModule.tree = tree
					success = syncToModuleDir(gitModule, moduleCacheDir, targetDir, env)
				}

				if len(gitModule.fallback) > 0 {
					if !success {
						for i, fallbackBranch := range gitModule.fallback {
							if i == len(gitModule.fallback)-1 {
								// last try
								gitModule.ignoreUnreachable = true
							}
							Debugf("Trying to resolve " + moduleCacheDir + " with branch " + fallbackBranch)
							gitModule.tree = fallbackBranch
							success = syncToModuleDir(gitModule, moduleCacheDir, targetDir, env)
							if success {
								break
							}
						}
						// possible TODO: shouldn't this fail if all fallback branches fail?
					}
				} else {
					gitModule.tree = tree
					success = syncToModuleDir(gitModule, moduleCacheDir, targetDir, env)
					if !success && !config.IgnoreUnreachableModules {
						Fatalf("Failed to resolve git module '" + gitName + "' with repository " + gitModule.git + " and branch/reference '" + tree + "' used in control repository branch '" + pf.sourceBranch + "' or Puppet environment '" + env + "'")
					}
				}

				// remove this module from the exisitingModuleDirs map
				moduleDirectory := filepath.Join(moduleDir, gitName)
				if len(gitModule.installPath) > 0 {
					moduleDirectory = filepath.Join(normalizeDir(basedir), normalizeDir(gitModule.installPath), gitName)
				}
				moduleDirectory = normalizeDir(moduleDirectory)
				mutex.Lock()
				delete(exisitingModuleDirs, moduleDirectory)
				for existingDir := range exisitingModuleDirs {
					rel, _ := filepath.Rel(existingDir, moduleDirectory)
					if len(rel) > 0 && !strings.Contains(rel, "..") {
						Debugf("not removing moduleDirectory " + moduleDirectory + " because it's a subdirectory to existingDir " + existingDir)
						delete(exisitingModuleDirs, existingDir)
					}
				}
				mutex.Unlock()
			}(gitName, gitModule, env, pf)
		}
		for forgeModuleName, fm := range pf.forgeModules {
			wg.Add()
			moduleDir := filepath.Join(pf.workDir, fm.moduleDir)
			moduleDir = normalizeDir(moduleDir)
			go func(forgeModuleName string, fm ForgeModule, moduleDir string, env string) {
				defer wg.Done()
				syncForgeToModuleDir(forgeModuleName, fm, moduleDir, env)
				// remove this module from the exisitingModuleDirs map
				mutex.Lock()
				mDir := filepath.Join(moduleDir, fm.name)
				delete(exisitingModuleDirs, mDir)
				mutex.Unlock()
			}(forgeModuleName, fm, moduleDir, env)
		}
	}
	wg.Wait()

	if stringSliceContains(config.PurgeLevels, "puppetfile") {
		if len(exisitingModuleDirs) > 0 && len(moduleParam) == 0 {
			for d := range exisitingModuleDirs {
				Infof("Removing unmanaged path " + d)
				if !dryRun {
					purgeDir(d, "purge_level puppetfile")
				}
			}
		}
	}
	if !debug && !verbose && !info && !quiet && term.IsTerminal(int(os.Stdout.Fd())) {
		uiprogress.Stop()
	}

	for _, pf := range allPuppetfiles {
		deployFile := filepath.Join(pf.workDir, ".g10k-deploy.json")
		if fileExists(deployFile) {
			Debugf("Finishing writing to deploy file " + deployFile)
			dr := readDeployResultFile(deployFile)
			dr.DeploySuccess = true
			dr.FinishedAt = time.Now()
			dr.PuppetfileChecksum = getSha256sumFile(filepath.Join(pf.workDir, "Puppetfile"))
			dr.GitDir = pf.gitDir
			dr.GitURL = pf.gitURL
			writeStructJSONFile(deployFile, dr)
		}
	}

}

func skipBasedOnFilterCommand(branch string, sourceName string, sa Source, workDir string) bool {
	branchFilterCommand := sa.FilterCommand
	branchFilterCommand = strings.ReplaceAll(branchFilterCommand, "$R10K_BRANCH", branch)
	branchFilterCommand = strings.ReplaceAll(branchFilterCommand, "$G10K_BRANCH", branch)
	branchFilterCommand = strings.ReplaceAll(branchFilterCommand, "$R10K_NAME", sourceName)
	branchFilterCommand = strings.ReplaceAll(branchFilterCommand, "$G10K_NAME", sourceName)
	branchFilterCommand = strings.ReplaceAll(branchFilterCommand, "$GIT_DIR", workDir)
	Debugf("executing filter_command: " + branchFilterCommand)
	er := executeCommand(branchFilterCommand, "", 30, true, false)
	if debug {
		fmt.Printf("filter_command %s result: %+v", branchFilterCommand, er)
	}
	return er.returnCode != 0
}

func skipBasedOnFilterRegex(branch string, sourceName string, sa Source, workDir string) bool {
	reFilterRegex, err := regexp.Compile(sa.FilterRegex)
	if err != nil {
		Fatalf("Setting filter_branch of source " + sourceName + " could not be compiled to a valid Go regex please fix!")
	}

	m := reFilterRegex.FindStringSubmatch(branch)
	return len(m) <= 0

}
