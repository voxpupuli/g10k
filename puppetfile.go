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
func (rt *Runtime) sourceSanityCheck(source string, sa Source) {
	if len(sa.PrivateKey) > 0 {
		if _, err := os.Stat(sa.PrivateKey); err != nil {
			rt.Fatalf("resolvePuppetEnvironment(): could not find SSH private key " + sa.PrivateKey + " for source " + source + " in config file " + rt.ConfigFile + " Error: " + err.Error())
		}
	}
	if len(sa.Basedir) <= 0 {
		rt.Fatalf("resolvePuppetEnvironment(): config setting basedir is not set for source " + source + " in config file " + rt.ConfigFile)
	}
	if len(sa.Remote) <= 0 {
		rt.Fatalf("resolvePuppetEnvironment(): config setting remote is not set for source " + source + " in config file " + rt.ConfigFile)
	}
}

func (rt *Runtime) resolvePuppetEnvironment(tags bool, outputNameTag string) error {
	wg := sizedwaitgroup.New(rt.Config.MaxExtractworker + 1)
	allPuppetfiles := make(map[string]Puppetfile)
	allEnvironments := make(map[string]bool)
	allBasedirs := make(map[string]bool)
	foundMatch := false
	for source, sa := range rt.Config.Sources {
		wg.Add()
		go func(source string, sa Source) {
			defer wg.Done()
			if rt.Force {
				rt.createOrPurgeDir(sa.Basedir, "resolvePuppetEnvironment()")
			}

			sa.Basedir = rt.checkDirAndCreate(sa.Basedir, "basedir for source "+source)
			rt.Debugf("Puppet environment: " + source + " (" + fmt.Sprintf("%+v", sa) + ")")

			// check for a valid source that has all necessary attributes (basedir, remote, SSH key exist if given)
			rt.sourceSanityCheck(source, sa)

			workDir := filepath.Join(rt.Config.EnvCacheDir, source+".git")
			// check if sa.Basedir exists
			rt.checkDirAndCreate(sa.Basedir, "basedir")

			controlRepoGit := GitModule{}
			controlRepoGit.git = sa.Remote
			controlRepoGit.privateKey = sa.PrivateKey
			if success := rt.doMirrorOrUpdate(controlRepoGit, workDir, 0); success {

				// get all branches
				er := rt.executeCommand("git --git-dir "+workDir+" branch", "", rt.Config.Timeout, false, false)
				outputBranches := er.output
				outputTags := ""

				if tags {
					er := rt.executeCommand("git --git-dir "+workDir+" tag", "", rt.Config.Timeout, false, false)
					outputTags = er.output
				}

				branches := strings.Split(strings.TrimSpace(outputBranches+outputTags), "\n")

				foundBranch := false
				prefix := rt.resolveSourcePrefix(source, sa)
				for _, branch := range branches {
					branch = strings.TrimLeft(branch, "* ")
					reInvalidCharacters := regexp.MustCompile(`\W`)
					if sa.AutoCorrectEnvironmentNames == "error" && reInvalidCharacters.MatchString(branch) {
						rt.Warnf("Ignoring branch " + branch + ", because it contains invalid characters")
						continue
					}
					// XXX: maybe make this user configurable (either with dedicated file or as YAML array in g10k config)
					if strings.Contains(branch, ";") || strings.Contains(branch, "&") || strings.Contains(branch, "|") || strings.HasPrefix(branch, "tmp/") && strings.HasSuffix(branch, "/head") {
						rt.Debugf("Skipping branch " + branch + " of source " + source + ", because of invalid character(s) inside the branch name")
						continue
					}

					if len(sa.FilterCommand) > 0 {
						if rt.skipBasedOnFilterCommand(branch, source, sa, workDir) {
							rt.Debugf("Skipping branch " + branch + " of source " + source + ", because of filter_command setting")
							continue
						}
					}
					if len(sa.FilterRegex) > 0 {
						if rt.skipBasedOnFilterRegex(branch, source, sa, workDir) {
							rt.Debugf("Skipping branch " + branch + " of source " + source + ", because of filter_regex setting")
							continue
						}
					}

					if len(rt.Branch) > 0 {
						if branch == rt.Branch {
							foundBranch = true
						} else {
							rt.Debugf("Environment " + prefix + branch + " of source " + source + " does not match branch name filter '" + rt.Branch + "', skipping")
							continue
						}
					} else if len(rt.Environment) > 0 {
						if source+"_"+branch == rt.Environment {
							foundMatch = true
						} else {
							rt.Debugf("Environment " + prefix + branch + " of source " + source + " does not match environment name filter '" + rt.Environment + "', skipping")
							continue
						}
					}

					wg.Add()

					go func(branch string, sa Source, prefix string) {
						defer wg.Done()
						if len(branch) != 0 {
							rt.Debugf("Resolving environment " + prefix + branch + " of source " + source)

							renamedBranch := branch
							if (len(outputNameTag) > 0) && (len(rt.Branch) > 0) {
								renamedBranch = outputNameTag
								rt.Debugf("Renaming branch " + branch + " to " + renamedBranch + " from source " + source + " " + sa.Remote)
							}

							// https://github.com/puppetlabs/r10k/blob/main/doc/dynamic-environments/configuration.mkd#strip_component
							if len(sa.StripComponent) != 0 {
								stripRenamedBranch := stripComponent(sa.StripComponent, renamedBranch)
								if stripRenamedBranch != renamedBranch {
									// only print this if the branch was definately renamed, because of the strip component
									rt.Debugf("Renaming branch " + renamedBranch + " to " + stripRenamedBranch + ", because of strip_component in source " + source + " " + sa.Remote)
									renamedBranch = stripRenamedBranch
								}
							}

							if sa.AutoCorrectEnvironmentNames == "correct" || sa.AutoCorrectEnvironmentNames == "correct_and_warn" {
								oldBranch := renamedBranch
								renamedBranch = reInvalidCharacters.ReplaceAllString(renamedBranch, "_")
								if oldBranch != renamedBranch {
									if sa.AutoCorrectEnvironmentNames == "correct_and_warn" {
										rt.Warnf("Renaming branch " + oldBranch + " to " + renamedBranch + " from source " + source + " " + sa.Remote)
									} else {
										rt.Debugf("Renaming branch " + oldBranch + " to " + renamedBranch + " from source " + source + " " + sa.Remote)
									}
								}
							}

							rt.Mutex.Lock()
							if _, ok := allEnvironments[filepath.Join(sa.Basedir, prefix+renamedBranch)]; !ok {
								allEnvironments[filepath.Join(sa.Basedir, prefix+renamedBranch)] = true
							} else {
								rt.Fatalf("Renamed environment naming conflict detected with renamed environment " + prefix + renamedBranch)
							}
							rt.Mutex.Unlock()
							targetDir := filepath.Join(sa.Basedir, prefix+strings.ReplaceAll(renamedBranch, "/", "_"))
							targetDir = normalizeDir(targetDir)

							env := strings.ReplaceAll(strings.Replace(targetDir, sa.Basedir, "", 1), "/", "")
							if len(rt.Module) == 0 {
								gitModule := GitModule{}
								gitModule.tree = branch
								rt.syncToModuleDir(gitModule, workDir, targetDir, env)
							}
							pf := filepath.Join(targetDir, "Puppetfile")
							rt.Mutex.Lock()
							allBasedirs[sa.Basedir] = true
							rt.Mutex.Unlock()
							if !fileExists(pf) {
								rt.Debugf("resolvePuppetEnvironment(): Skipping branch " + source + "_" + branch + " because " + pf + " does not exist")
								deployFile := filepath.Join(targetDir, ".g10k-deploy.json")
								if fileExists(deployFile) {
									rt.Debugf("Finishing writing to deploy file " + deployFile)
									dr := rt.readDeployResultFile(deployFile)
									dr.DeploySuccess = true
									dr.FinishedAt = time.Now()
									dr.GitDir = sa.Basedir
									dr.GitURL = sa.Remote
									rt.writeStructJSONFile(deployFile, dr)
								}
							} else {
								puppetfile, err := rt.readPuppetfile(pf, sa.PrivateKey, source, branch, sa.ForceForgeVersions, false)
								if err != nil {
									rt.Fatalf(err.Error())
									return
								}
								puppetfile.workDir = normalizeDir(targetDir)
								puppetfile.controlRepoBranch = branch
								puppetfile.gitDir = workDir
								puppetfile.gitURL = sa.Remote
								rt.Mutex.Lock()
								for _, moduleDir := range puppetfile.moduleDirs {
									rt.checkDirAndCreate(filepath.Join(puppetfile.workDir, moduleDir), "moduledir for env")
								}
								allPuppetfiles[env] = puppetfile
								rt.Mutex.Unlock()

							}
						}
					}(branch, sa, prefix)
				}

				if sa.ErrorMissingBranch && !foundBranch {
					rt.Fatalf("Couldn't find specified branch '" + rt.Branch + "' anywhere in source '" + source + "' (" + sa.Remote + ")")
				} else if sa.WarnMissingBranch && !foundBranch {
					rt.Warnf("WARNING: Couldn't find specified branch '" + rt.Branch + "' anywhere in source '" + source + "' (" + sa.Remote + ")")
				}
			} else {
				rt.Warnf("WARNING: Could not resolve git repository in source '" + source + "' (" + sa.Remote + ")")
				if sa.ExitIfUnreachable {
					os.Exit(1)
				}
			}
		}(source, sa)
	}

	wg.Wait()
	if len(rt.Environment) > 0 {
		if !foundMatch {
			rt.Warnf("WARNING: Environment '" + rt.Environment + "' cannot be found in any source and will not be deployed.")
		}
	}
	//fmt.Println("allPuppetfiles: ", allPuppetfiles, len(allPuppetfiles))
	//fmt.Println("allPuppetfiles[0]: ", allPuppetfiles["postinstall"])
	rt.resolvePuppetfile(allPuppetfiles)
	// fmt.Printf("%+v\n", allEnvironments)
	if len(rt.Module) == 0 {
		rt.purgeUnmanagedContent(allBasedirs, allEnvironments)
	}
	return nil
}

// resolveSourcePrefix implements the prefix read out from each source given in the config file, like r10k https://github.com/puppetlabs/r10k/blob/master/doc/dynamic-environments/configuration.mkd#prefix
func (rt *Runtime) resolveSourcePrefix(source string, sa Source) string {
	switch sa.Prefix {
	case "false", "":
		return ""
	case "true":
		return source + "_"
	default:
		return sa.Prefix + "_"
	}
}

func (rt *Runtime) resolvePuppetfile(allPuppetfiles map[string]Puppetfile) {
	wg := sizedwaitgroup.New(rt.Config.MaxExtractworker)
	exisitingModuleDirs := make(map[string]struct{})
	uniqueGitModules := make(map[string]GitModule)
	// if we made it this far initialize the global maps
	rt.LatestForgeModules.m = make(map[string]string)
	for env, pf := range allPuppetfiles {
		rt.Debugf("Resolving branch " + env + " of source " + pf.source)
		//fmt.Println(pf)
		for gitName, gitModule := range pf.gitModules {
			if len(rt.Module) > 0 {
				if gitName != rt.Module {
					rt.Debugf("Skipping git module " + gitName + ", because parameter -module is set to " + rt.Module)
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
			if len(rt.Module) > 0 {
				if forgeModuleName != rt.Module {
					rt.Debugf("Skipping forge module " + forgeModuleName + ", because parameter -module is set to " + rt.Module)
					delete(pf.forgeModules, forgeModuleName)
					continue
				}
			}
			fm.baseURL = pf.forgeBaseURL
			if pf.forgeCacheTTL != 0 {
				fm.cacheTTL = pf.forgeCacheTTL
			} else {
				fm.cacheTTL = rt.Config.ForgeCacheTTL
			}
			// fmt.Println("Found Forge module", fm.author, "/", forgeModuleName, "with version", fm.version, "and cacheTTL", fm.cacheTTL)
			forgeModuleName = strings.ReplaceAll(forgeModuleName, "/", "-")
			uniqueForgeModuleName := fm.author + "/" + forgeModuleName + "-" + fm.version
			if _, ok := rt.UniqueForgeModules[uniqueForgeModuleName]; !ok {
				rt.UniqueForgeModules[uniqueForgeModuleName] = fm
			} else {
				// Use the shortest Forge cache TTL for this module
				if rt.UniqueForgeModules[uniqueForgeModuleName].cacheTTL > pf.forgeCacheTTL {
					delete(rt.UniqueForgeModules, uniqueForgeModuleName)
					rt.UniqueForgeModules[uniqueForgeModuleName] = fm
				}
			}
		}
	}
	if !rt.Debug && !rt.Verbose && !rt.Info && !rt.Quiet && term.IsTerminal(int(os.Stdout.Fd())) {
		uiprogress.Start()
	}
	var wgResolve sync.WaitGroup
	wgResolve.Add(2)
	go func() {
		defer wgResolve.Done()
		rt.resolveGitRepositories(uniqueGitModules)
	}()
	go func() {
		defer wgResolve.Done()
		rt.resolveForgeModules(rt.UniqueForgeModules)
	}()
	wgResolve.Wait()
	//log.Println(config.Sources["cmdlineparam"])
	for env, pf := range allPuppetfiles {
		rt.Debugf("Syncing " + env + " with workDir " + pf.workDir)
		// this prevents g10k from purging module directories on the subsequent run in -puppetfile mode
		basedir := ""
		if !rt.PFMode {
			basedir = rt.checkDirAndCreate(pf.workDir, "basedir 2 for source "+pf.source)
		}

		for _, moduleDir := range pf.moduleDirs {
			moduleDir = normalizeDir(filepath.Join(pf.workDir, moduleDir))
			exisitingModuleDirsFI, _ := os.ReadDir(moduleDir)
			rt.Mutex.Lock()
			for _, exisitingModuleDir := range exisitingModuleDirsFI {
				// fmt.Println("adding dir: ", filepath.Join(moduleDir, exisitingModuleDir.Name()))
				exisitingModuleDirs[filepath.Join(moduleDir, exisitingModuleDir.Name())] = rt.empty
			}
			rt.Mutex.Unlock()
		}

		for gitName, gitModule := range pf.gitModules {
			moduleDir := filepath.Join(pf.workDir, gitModule.moduleDir)
			moduleDir = normalizeDir(moduleDir)
			if gitModule.local {
				moduleDirectory := filepath.Join(moduleDir, gitName)
				rt.Debugf("Not deleting " + moduleDirectory + " as it is declared as a local module")
				// remove this module from the exisitingModuleDirs map
				if len(gitModule.installPath) > 0 {
					moduleDirectory = filepath.Join(normalizeDir(basedir), normalizeDir(gitModule.installPath), gitName)
				}
				moduleDirectory = normalizeDir(moduleDirectory)
				rt.Mutex.Lock()
				delete(exisitingModuleDirs, moduleDirectory)
				for existingDir := range exisitingModuleDirs {
					rel, _ := filepath.Rel(existingDir, moduleDirectory)
					if len(rel) > 0 && !strings.Contains(rel, "..") {
						rt.Debugf("not removing moduleDirectory " + moduleDirectory + " because it's a subdirectory to existingDir " + existingDir)
						delete(exisitingModuleDirs, existingDir)
					}
				}
				rt.Mutex.Unlock()
				continue
			}
			wg.Add()
			go func(gitName string, gitModule GitModule, env string, pf Puppetfile) {
				defer wg.Done()
				targetDir := normalizeDir(filepath.Join(moduleDir, gitName))
				moduleCacheDir := filepath.Join(rt.Config.ModulesCacheDir, strings.ReplaceAll(strings.ReplaceAll(gitModule.git, "/", "_"), ":", "-"))
				tree := rt.detectDefaultBranch(moduleCacheDir)
				rt.Debugf("Setting " + tree + " as default branch for " + gitModule.git)
				if len(gitModule.branch) > 0 {
					tree = gitModule.branch
				} else if len(gitModule.commit) > 0 {
					tree = gitModule.commit
				} else if len(gitModule.tag) > 0 {
					tree = gitModule.tag
				} else if len(gitModule.ref) > 0 {
					tree = gitModule.ref
				} else if gitModule.link {
					if rt.PFMode {
						if len(os.Getenv("g10k_branch")) > 0 {
							tree = os.Getenv("g10k_branch")
						} else if len(rt.Branch) > 0 {
							tree = rt.Branch
						} else {
							rt.Fatalf("resolvePuppetfile(): found module " + gitName + " with module link mode enabled and g10k in Puppetfile mode which is not supported, as g10k can not detect the environment branch of the Puppetfile. You can explicitly set the module link branch you want to use in Puppetfile mode by setting the environment variable 'g10k_branch' or using the -branch parameter")
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
					rt.Debugf("Trying to resolve " + moduleCacheDir + " with branch " + tree)
					gitModule.tree = tree
					success = rt.syncToModuleDir(gitModule, moduleCacheDir, targetDir, env)
				}

				if len(gitModule.fallback) > 0 {
					if !success {
						for i, fallbackBranch := range gitModule.fallback {
							if i == len(gitModule.fallback)-1 {
								// last try
								gitModule.ignoreUnreachable = true
							}
							rt.Debugf("Trying to resolve " + moduleCacheDir + " with branch " + fallbackBranch)
							gitModule.tree = fallbackBranch
							success = rt.syncToModuleDir(gitModule, moduleCacheDir, targetDir, env)
							if success {
								break
							}
						}
						// possible TODO: shouldn't this fail if all fallback branches fail?
					}
				} else {
					gitModule.tree = tree
					success = rt.syncToModuleDir(gitModule, moduleCacheDir, targetDir, env)
					if !success && !rt.Config.IgnoreUnreachableModules {
						rt.Fatalf("Failed to resolve git module '" + gitName + "' with repository " + gitModule.git + " and branch/reference '" + tree + "' used in control repository branch '" + pf.sourceBranch + "' or Puppet environment '" + env + "'")
					}
				}

				// remove this module from the exisitingModuleDirs map
				moduleDirectory := filepath.Join(moduleDir, gitName)
				if len(gitModule.installPath) > 0 {
					moduleDirectory = filepath.Join(normalizeDir(basedir), normalizeDir(gitModule.installPath), gitName)
				}
				moduleDirectory = normalizeDir(moduleDirectory)
				rt.Mutex.Lock()
				delete(exisitingModuleDirs, moduleDirectory)
				for existingDir := range exisitingModuleDirs {
					rel, _ := filepath.Rel(existingDir, moduleDirectory)
					if len(rel) > 0 && !strings.Contains(rel, "..") {
						rt.Debugf("not removing moduleDirectory " + moduleDirectory + " because it's a subdirectory to existingDir " + existingDir)
						delete(exisitingModuleDirs, existingDir)
					}
				}
				rt.Mutex.Unlock()
			}(gitName, gitModule, env, pf)
		}
		for forgeModuleName, fm := range pf.forgeModules {
			wg.Add()
			moduleDir := filepath.Join(pf.workDir, fm.moduleDir)
			moduleDir = normalizeDir(moduleDir)
			go func(forgeModuleName string, fm ForgeModule, moduleDir string, env string) {
				defer wg.Done()
				rt.syncForgeToModuleDir(forgeModuleName, fm, moduleDir, env)
				// remove this module from the exisitingModuleDirs map
				rt.Mutex.Lock()
				mDir := filepath.Join(moduleDir, fm.name)
				delete(exisitingModuleDirs, mDir)
				rt.Mutex.Unlock()
			}(forgeModuleName, fm, moduleDir, env)
		}
	}
	wg.Wait()

	if stringSliceContains(rt.Config.PurgeLevels, "puppetfile") {
		if len(exisitingModuleDirs) > 0 && len(rt.Module) == 0 {
			for d := range exisitingModuleDirs {
				rt.Infof("Removing unmanaged path " + d)
				if !rt.DryRun {
					rt.purgeDir(d, "purge_level puppetfile")
				}
			}
		}
	}
	// TODO: Use properly scaled log levels here rather than a bunch of booleans.
	// There is an enum created, but unused for this purpose.
	if !rt.Debug && !rt.Verbose && !rt.Info && !rt.Quiet && term.IsTerminal(int(os.Stdout.Fd())) {
		uiprogress.Stop()
	}

	for _, pf := range allPuppetfiles {
		deployFile := filepath.Join(pf.workDir, ".g10k-deploy.json")
		if fileExists(deployFile) {
			rt.Debugf("Finishing writing to deploy file " + deployFile)
			dr := rt.readDeployResultFile(deployFile)
			dr.DeploySuccess = true
			dr.FinishedAt = time.Now()
			dr.PuppetfileChecksum = rt.getSha256sumFile(filepath.Join(pf.workDir, "Puppetfile"))
			dr.GitDir = pf.gitDir
			dr.GitURL = pf.gitURL
			rt.writeStructJSONFile(deployFile, dr)
		}
	}

}

func (rt *Runtime) skipBasedOnFilterCommand(branch string, sourceName string, sa Source, workDir string) bool {
	branchFilterCommand := sa.FilterCommand
	branchFilterCommand = strings.ReplaceAll(branchFilterCommand, "$R10K_BRANCH", branch)
	branchFilterCommand = strings.ReplaceAll(branchFilterCommand, "$G10K_BRANCH", branch)
	branchFilterCommand = strings.ReplaceAll(branchFilterCommand, "$R10K_NAME", sourceName)
	branchFilterCommand = strings.ReplaceAll(branchFilterCommand, "$G10K_NAME", sourceName)
	branchFilterCommand = strings.ReplaceAll(branchFilterCommand, "$GIT_DIR", workDir)
	rt.Debugf("executing filter_command: " + branchFilterCommand)
	er := rt.executeCommand(branchFilterCommand, "", 30, true, false)
	if rt.Debug {
		fmt.Printf("filter_command %s result: %+v", branchFilterCommand, er)
	}
	return er.returnCode != 0
}

func (rt *Runtime) skipBasedOnFilterRegex(branch string, sourceName string, sa Source, workDir string) bool {
	reFilterRegex, err := regexp.Compile(sa.FilterRegex)
	if err != nil {
		rt.Fatalf("Setting filter_branch of source " + sourceName + " could not be compiled to a valid Go regex please fix!")
	}

	m := reFilterRegex.FindStringSubmatch(branch)
	return len(m) <= 0

}
