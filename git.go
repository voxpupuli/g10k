package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

func (rt *Runtime) resolveGitRepositories(uniqueGitModules map[string]GitModule) {
	defer rt.timeTrack(time.Now(), funcName())
	if len(uniqueGitModules) <= 0 {
		rt.Debugf("uniqueGitModules[] is empty, skipping...")
		return
	}
	// Dummy channel to coordinate the number of concurrent goroutines.
	// This channel should be buffered otherwise we will be immediately blocked
	// when trying to fill it.

	rt.Debugf("Resolving " + strconv.Itoa(len(uniqueGitModules)) + " Git modules with " + strconv.Itoa(rt.Config.Maxworker) + " workers")
	concurrentGoroutines := make(chan struct{}, rt.Config.Maxworker)
	// Fill the dummy channel with config.Maxworker empty struct.
	for i := 0; i < rt.Config.Maxworker; i++ {
		concurrentGoroutines <- struct{}{}
	}

	// The done channel indicates when a single goroutine has finished its job.
	done := make(chan bool)
	// The waitForAllJobs channel allows the main program
	// to wait until we have indeed done all the jobs.
	waitForAllJobs := make(chan bool)
	// Collect all the jobs, and since the job is finished, we can
	// release another spot for a goroutine.
	go func() {
		for _, gm := range uniqueGitModules {
			go func(gm GitModule) {
				<-done
				// Say that another goroutine can now start.
				concurrentGoroutines <- struct{}{}
			}(gm)
		}
		// We have collected all the jobs, the program can now terminate
		waitForAllJobs <- true
	}()
	wg := sync.WaitGroup{}
	wg.Add(len(uniqueGitModules))

	for url, gm := range uniqueGitModules {
		privateKey := gm.privateKey
		go func(url string, gm GitModule) {
			// Try to receive from the concurrentGoroutines channel. When we have something,
			// it means we can start a new goroutine because another one finished.
			// Otherwise, it will block the execution until an execution
			// spot is available.
			<-concurrentGoroutines
			defer wg.Done()

			if gm.useSSHAgent {
				rt.Debugf("git repo url " + url + " with loaded SSH keys from ssh-agent")
			} else if len(gm.privateKey) > 0 {
				rt.Debugf("git repo url " + url + " with SSH key " + privateKey)
			} else {
				rt.Debugf("git repo url " + url + " without ssh key")
			}

			//log.Println(config)
			// create save directory name from Git repo name
			repoDir := strings.ReplaceAll(strings.ReplaceAll(url, "/", "_"), ":", "-")
			workDir := filepath.Join(rt.Config.ModulesCacheDir, repoDir)

			success := rt.doMirrorOrUpdate(gm, workDir, 0)
			if !success && !rt.Config.UseCacheFallback {
				rt.Fatalf("Fatal: Failed to clone or pull " + url + " to " + workDir)
			}
			done <- true
		}(url, gm)
	}

	// Wait for all jobs to finish
	<-waitForAllJobs
	wg.Wait()
}

func (rt *Runtime) doMirrorOrUpdate(gitModule GitModule, workDir string, retryCount int) bool {
	//fmt.Printf("%+v\n", gitModule)
	isControlRepo := strings.HasPrefix(workDir, rt.Config.EnvCacheDir)
	isInModulesCacheDir := strings.HasPrefix(workDir, rt.Config.ModulesCacheDir)

	explicitlyLoadSSHKey := true
	if len(gitModule.privateKey) == 0 || strings.Contains(gitModule.git, "github.com") || gitModule.useSSHAgent || strings.HasPrefix(gitModule.git, "https://") {
		if gitModule.useSSHAgent || len(gitModule.privateKey) == 0 {
			explicitlyLoadSSHKey = false
		} else if isControlRepo {
			explicitlyLoadSSHKey = true
		} else {
			explicitlyLoadSSHKey = false
		}
	}
	er := ExecResult{}
	gitCmd := "git clone --mirror " + gitModule.git + " " + workDir
	if rt.Config.CloneGitModules && !isControlRepo && !isInModulesCacheDir {
		// only clone here, because we can't be sure if a branch is used or a commit hash or tag
		// we switch to the defined reference later
		gitCmd = "git clone " + gitModule.git + " " + workDir
	}
	if isDir(workDir) {
		if rt.detectGitRemoteURLChange(workDir, gitModule.git) && isControlRepo {
			rt.purgeDir(workDir, "git remote url changed")
		} else {
			// TODO: This should be reworked to use a git library or custom code.
			// Prefer not to shell out into cli tools we can't control/audit.
			gitCmd = "git --git-dir " + workDir + " remote update --prune"
		}
	}

	// check if git URL does match NO_PROXY
	disableHttpProxy := rt.matchGitRemoteURLNoProxy(gitModule.git)

	if explicitlyLoadSSHKey {
		sshAddCmd := "ssh-add "
		if runtime.GOOS == "darwin" {
			sshAddCmd = "ssh-add -K "
		}
		er = rt.executeCommand("ssh-agent bash -c '"+sshAddCmd+gitModule.privateKey+"; "+gitCmd+"'", "", rt.Config.Timeout, gitModule.ignoreUnreachable, disableHttpProxy)
	} else {
		er = rt.executeCommand(gitCmd, "", rt.Config.Timeout, gitModule.ignoreUnreachable, disableHttpProxy)
	}

	if er.returnCode != 0 {
		if rt.Config.UseCacheFallback {
			rt.Warnf("WARN: git repository " + gitModule.git + " does not exist or is unreachable at this moment!")
			rt.Warnf("WARN: Trying to use cache for " + gitModule.git + " git repository")
			return false
		} else if rt.Config.RetryGitCommands && retryCount > -1 {
			rt.Warnf("WARN: git command failed: " + gitCmd + " deleting local cached repository and retrying...")
			rt.purgeDir(workDir, "doMirrorOrUpdate, because git command failed, retrying")
			return rt.doMirrorOrUpdate(gitModule, workDir, retryCount-1)
		}
		rt.Warnf("WARN: git repository " + gitModule.git + " does not exist or is unreachable at this moment! Error: " + er.output)
		return false
	}

	if rt.Config.CloneGitModules && !isControlRepo && !isInModulesCacheDir {
		// if clone of git modules was specified, switch to the module and try to switch to the reference commit hash/tag/branch
		// TODO: Replace with programmitic methodology
		gitCmd = "git checkout " + gitModule.tree
		er = rt.executeCommand(gitCmd, workDir, rt.Config.Timeout, gitModule.ignoreUnreachable, disableHttpProxy)
		if er.returnCode != 0 {
			rt.Warnf("WARN: git repository " + gitModule.git + " does not exist or is unreachable at this moment! Error: " + er.output)
			return false
		}
	}

	return true
}

func (rt *Runtime) syncToModuleDir(gitModule GitModule, srcDir string, targetDir string, correspondingPuppetEnvironment string) bool {
	startedAt := time.Now()
	rt.Mutex.Lock()
	rt.SyncGitCount++
	rt.Mutex.Unlock()
	if !isDir(srcDir) {
		if rt.Config.UseCacheFallback {
			rt.Fatalf("Could not find cached git module " + srcDir)
		}
	}
	revParseCmd := "git --git-dir " + srcDir + " rev-parse --verify '" + gitModule.tree
	if !rt.Config.GitObjectSyntaxNotSupported {
		revParseCmd = revParseCmd + "^{object}'"
	} else {
		revParseCmd = revParseCmd + "'"
	}

	isControlRepo := strings.HasPrefix(srcDir, rt.Config.EnvCacheDir)

	er := rt.executeCommand(revParseCmd, "", rt.Config.Timeout, gitModule.ignoreUnreachable, false)
	hashFile := filepath.Join(targetDir, ".latest_commit")
	deployFile := filepath.Join(targetDir, ".g10k-deploy.json")
	needToSync := true
	if er.returnCode != 0 {
		if gitModule.ignoreUnreachable {
			rt.Debugf("Failed to populate module " + targetDir + " but ignore-unreachable is set. Continuing...")
			rt.purgeDir(targetDir, "syncToModuleDir, because ignore-unreachable is set for this module")
		}
		return false
	}

	if len(er.output) > 0 {
		commitHash := strings.TrimSuffix(er.output, "\n")
		if strings.HasPrefix(srcDir, rt.Config.EnvCacheDir) {
			if fileExists(deployFile) {
				dr := rt.readDeployResultFile(deployFile)
				if dr.Signature == strings.TrimSuffix(er.output, "\n") && dr.DeploySuccess {
					needToSync = false
				}
			}
		} else {
			targetHashByte, _ := os.ReadFile(hashFile)
			targetHash := string(targetHashByte)
			rt.Debugf("string content of " + hashFile + " is: " + targetHash)
			if targetHash == commitHash {
				needToSync = false
				rt.Debugf("Skipping, because no diff found between " + srcDir + "(" + commitHash + ") and " + targetDir + "(" + targetHash + ")")
			} else {
				rt.Debugf("Need to sync, because existing Git module: " + targetDir + " has commit " + targetHash + " and the to be synced commit is: " + commitHash)
			}
		}

	}
	if needToSync && er.returnCode == 0 {
		rt.Mutex.Lock()
		rt.Infof("Need to sync " + targetDir)
		rt.NeedSyncDirs = append(rt.NeedSyncDirs, targetDir)
		if _, ok := rt.NeedSyncEnvs[correspondingPuppetEnvironment]; !ok {
			rt.NeedSyncEnvs[correspondingPuppetEnvironment] = rt.empty
		}
		rt.NeedSyncGitCount++
		rt.Mutex.Unlock()
		moduleDir := "modules"
		purgeWholeEnvDir := true
		// check if it is a control repo and already exists
		if isControlRepo && isDir(targetDir) {
			// then check if it contains a Puppetfile
			gitShowCmd := "git --git-dir " + srcDir + " show " + gitModule.tree + ":Puppetfile"
			executeResult := rt.executeCommand(gitShowCmd, "", rt.Config.Timeout, true, false)
			rt.Debugf("Executing " + gitShowCmd)
			if executeResult.returnCode != 0 {
				purgeWholeEnvDir = true
			} else {
				purgeWholeEnvDir = false
				lines := strings.Split(executeResult.output, "\n")
				for _, line := range lines {
					if m := reModuledir.FindStringSubmatch(line); len(m) > 1 {
						// moduledir CLI parameter override
						if len(rt.ModuleDir) != 0 {
							moduleDir = rt.ModuleDir
						} else {
							moduleDir = normalizeDir(m[1])
						}
					}
				}
			}
		}
		// if so delete everything except the moduledir where the Puppet modules reside
		// else simply delete the whole dir and check it out again
		if purgeWholeEnvDir {
			rt.purgeDir(targetDir, "need to sync")
		} else {
			rt.Infof("Detected control repo change, but trying to preserve module dir " + filepath.Join(targetDir, moduleDir))
			rt.purgeControlRepoExceptModuledir(targetDir, moduleDir)
		}

		if !rt.DryRun && !rt.Config.CloneGitModules || isControlRepo {
			if rt.PFMode {
				rt.purgeDir(targetDir, "git dir with changes in -puppetfile mode")
			}
			rt.checkDirAndCreate(targetDir, "git dir")
			gitArchiveArgs := []string{"--git-dir", srcDir, "archive", gitModule.tree}
			cmd := exec.Command("git", gitArchiveArgs...)
			rt.Debugf("Executing git --git-dir " + srcDir + " archive " + gitModule.tree)
			cmdOut, err := cmd.StdoutPipe()
			if err != nil {
				if gitModule.ignoreUnreachable {
					rt.Infof("Failed to populate module " + targetDir + " but ignore-unreachable is set. Continuing...")
					return false
				}
				rt.Fatalf("syncToModuleDir(): Failed to execute command: git --git-dir " + srcDir + " archive " + gitModule.tree + " Error: " + err.Error())
			}
			_ = cmd.Start() // FIXME: error should be handled

			before := time.Now()
			rt.unTar(cmdOut, targetDir)
			duration := time.Since(before).Seconds()
			rt.Mutex.Lock()
			rt.IoGitTime += duration
			rt.Mutex.Unlock()

			err = cmd.Wait()
			if err != nil {
				rt.Fatalf("syncToModuleDir(): Failed to execute command: git --git-dir " + srcDir + " archive " + gitModule.tree + " Error: " + err.Error())
				//"\nIf you are using GitLab please ensure that you've added your deploy key to your repository." +
				//"\nThe Puppet environment which is using this unresolveable repository is " + correspondingPuppetEnvironment)
			}

			rt.Verbosef("syncToModuleDir(): Executing git --git-dir " + srcDir + " archive " + gitModule.tree + " took " + strconv.FormatFloat(duration, 'f', 5, 64) + "s")

			commitHash := strings.TrimSuffix(er.output, "\n")
			if isControlRepo {
				rt.Debugf("Writing to deploy file " + deployFile)
				dr := DeployResult{
					Name:      gitModule.tree,
					Signature: commitHash,
					StartedAt: startedAt,
				}
				rt.writeStructJSONFile(deployFile, dr)
			} else {
				rt.Debugf("Writing hash " + commitHash + " from command " + revParseCmd + " to " + hashFile)
				f, _ := os.Create(hashFile)
				defer func() { _ = f.Close() }()
				_, _ = f.WriteString(commitHash) // FIXME: error should be handled
				_ = f.Sync()                     // FIXME: error should be handled
			}

		} else if rt.Config.CloneGitModules {
			return rt.doMirrorOrUpdate(gitModule, targetDir, 0)
		}
	}
	return true
}

func (rt *Runtime) detectDefaultBranch(gitDir string) string {
	remoteShowOriginCmd := "git ls-remote --symref " + gitDir
	er := rt.executeCommand(remoteShowOriginCmd, "", rt.Config.Timeout, false, false)
	foundRefs := strings.Split(er.output, "\n")
	if len(foundRefs) < 1 {
		rt.Fatalf("Unable to detect default branch for git repository with command git ls-remote --symref " + gitDir)
	}
	// should look like this:
	// ref: refs/heads/main\tHEAD
	headBranchParts := strings.Split(foundRefs[0], "\t")
	defaultBranch := strings.TrimPrefix(string(headBranchParts[0]), "ref: refs/heads/")
	//fmt.Println(defaultBranch)
	return defaultBranch
}

func (rt *Runtime) detectGitRemoteURLChange(d string, url string) bool {
	gitRemoteCmd := "git --git-dir " + d + " remote -v"

	er := rt.executeCommand(gitRemoteCmd, "", rt.Config.Timeout, false, false)
	if er.returnCode != 0 {
		rt.Warnf("WARN: Could not detect remote URL for git repository " + d + " trying to purge it and mirror it again")
		return true
	}

	f := strings.Fields(er.output)
	if len(f) < 3 {
		rt.Warnf("WARN: Could not detect remote URL for git repository " + d + " trying to purge it and mirror it again")
		return true
	}
	configuredRemote := f[1]
	return configuredRemote != url
}
