package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/xorpaul/uiprogress"
)

func resolveGitRepositories(uniqueGitModules map[string]GitModule) {
	defer timeTrack(time.Now(), funcName())
	if len(uniqueGitModules) <= 0 {
		Debugf("uniqueGitModules[] is empty, skipping...")
		return
	}
	bar := uiprogress.AddBar(len(uniqueGitModules)).AppendCompleted().PrependElapsed()
	bar.PrependFunc(func(b *uiprogress.Bar) string {
		return fmt.Sprintf("Resolving Git modules (%d/%d)", b.Current(), len(uniqueGitModules))
	})
	// Dummy channel to coordinate the number of concurrent goroutines.
	// This channel should be buffered otherwise we will be immediately blocked
	// when trying to fill it.

	Debugf("Resolving " + strconv.Itoa(len(uniqueGitModules)) + " Git modules with " + strconv.Itoa(config.Maxworker) + " workers")
	concurrentGoroutines := make(chan struct{}, config.Maxworker)
	// Fill the dummy channel with config.Maxworker empty struct.
	for i := 0; i < config.Maxworker; i++ {
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
		go func(url string, gm GitModule, bar *uiprogress.Bar) {
			// Try to receive from the concurrentGoroutines channel. When we have something,
			// it means we can start a new goroutine because another one finished.
			// Otherwise, it will block the execution until an execution
			// spot is available.
			<-concurrentGoroutines
			defer bar.Incr()
			defer wg.Done()

			if gm.useSSHAgent {
				Debugf("git repo url " + url + " with loaded SSH keys from ssh-agent")
			} else if len(gm.privateKey) > 0 {
				Debugf("git repo url " + url + " with SSH key " + privateKey)
			} else {
				Debugf("git repo url " + url + " without ssh key")
			}

			//log.Println(config)
			// create save directory name from Git repo name
			repoDir := strings.Replace(strings.Replace(url, "/", "_", -1), ":", "-", -1)
			workDir := filepath.Join(config.ModulesCacheDir, repoDir)

			success := doMirrorOrUpdate(gm, workDir, 0)
			if !success && !config.UseCacheFallback {
				Fatalf("Fatal: Failed to clone or pull " + url + " to " + workDir)
			}
			done <- true
		}(url, gm, bar)
	}

	// Wait for all jobs to finish
	<-waitForAllJobs
	wg.Wait()
}

func doMirrorOrUpdate(gitModule GitModule, workDir string, retryCount int) bool {
	//fmt.Printf("%+v\n", gitModule)
	isControlRepo := strings.HasPrefix(workDir, config.EnvCacheDir)
	isInModulesCacheDir := strings.HasPrefix(workDir, config.ModulesCacheDir)

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
	if config.CloneGitModules && !isControlRepo && !isInModulesCacheDir {
		// only clone here, because we can't be sure if a branch is used or a commit hash or tag
		// we switch to the defined reference later
		gitCmd = "git clone " + gitModule.git + " " + workDir
	}
	if isDir(workDir) {
		if detectGitRemoteURLChange(workDir, gitModule.git) && isControlRepo {
			purgeDir(workDir, "git remote url changed")
		} else {
			gitCmd = "git --git-dir " + workDir + " remote update --prune"
		}
	}

	// check if git URL does match NO_PROXY
	disableHttpProxy := false
	if matchGitRemoteURLNoProxy(gitModule.git) {
		disableHttpProxy = true
	}

	if explicitlyLoadSSHKey {
		sshAddCmd := "ssh-add "
		if runtime.GOOS == "darwin" {
			sshAddCmd = "ssh-add -K "
		}
		er = executeCommand("ssh-agent bash -c '"+sshAddCmd+gitModule.privateKey+"; "+gitCmd+"'", "", config.Timeout, gitModule.ignoreUnreachable, disableHttpProxy)
	} else {
		er = executeCommand(gitCmd, "", config.Timeout, gitModule.ignoreUnreachable, disableHttpProxy)
	}

	if er.returnCode != 0 {
		if config.UseCacheFallback {
			Warnf("WARN: git repository " + gitModule.git + " does not exist or is unreachable at this moment!")
			Warnf("WARN: Trying to use cache for " + gitModule.git + " git repository")
			return false
		} else if config.RetryGitCommands && retryCount > -1 {
			Warnf("WARN: git command failed: " + gitCmd + " deleting local cached repository and retrying...")
			purgeDir(workDir, "doMirrorOrUpdate, because git command failed, retrying")
			return doMirrorOrUpdate(gitModule, workDir, retryCount-1)
		}
		Warnf("WARN: git repository " + gitModule.git + " does not exist or is unreachable at this moment! Error: " + er.output)
		return false
	}

	if config.CloneGitModules && !isControlRepo && !isInModulesCacheDir {
		// if clone of git modules was specified, switch to the module and try to switch to the reference commit hash/tag/branch
		gitCmd = "git checkout " + gitModule.tree
		er = executeCommand(gitCmd, workDir, config.Timeout, gitModule.ignoreUnreachable, disableHttpProxy)
		if er.returnCode != 0 {
			Warnf("WARN: git repository " + gitModule.git + " does not exist or is unreachable at this moment! Error: " + er.output)
			return false
		}
	}

	return true
}

func syncToModuleDir(gitModule GitModule, srcDir string, targetDir string, correspondingPuppetEnvironment string) bool {
	startedAt := time.Now()
	mutex.Lock()
	syncGitCount++
	mutex.Unlock()
	if !isDir(srcDir) {
		if config.UseCacheFallback {
			Fatalf("Could not find cached git module " + srcDir)
		}
	}
	revParseCmd := "git --git-dir " + srcDir + " rev-parse --verify '" + gitModule.tree
	if !config.GitObjectSyntaxNotSupported {
		revParseCmd = revParseCmd + "^{object}'"
	} else {
		revParseCmd = revParseCmd + "'"
	}

	isControlRepo := strings.HasPrefix(srcDir, config.EnvCacheDir)

	er := executeCommand(revParseCmd, "", config.Timeout, gitModule.ignoreUnreachable, false)
	hashFile := filepath.Join(targetDir, ".latest_commit")
	deployFile := filepath.Join(targetDir, ".g10k-deploy.json")
	needToSync := true
	if er.returnCode != 0 {
		if gitModule.ignoreUnreachable {
			Debugf("Failed to populate module " + targetDir + " but ignore-unreachable is set. Continuing...")
			purgeDir(targetDir, "syncToModuleDir, because ignore-unreachable is set for this module")
		}
		return false
	}

	if len(er.output) > 0 {
		commitHash := strings.TrimSuffix(er.output, "\n")
		if strings.HasPrefix(srcDir, config.EnvCacheDir) {
			if fileExists(deployFile) {
				dr := readDeployResultFile(deployFile)
				if dr.Signature == strings.TrimSuffix(er.output, "\n") && dr.DeploySuccess {
					needToSync = false
					mutex.Lock()
					Debugf("Setting PuppetfileMatch for env: " + correspondingPuppetEnvironment + " because it is already up-to-date")
					needSyncEnvs[correspondingPuppetEnvironment+":PuppetfileMatch"] = empty
					mutex.Unlock()
				}
			}
		} else {
			targetHashByte, _ := os.ReadFile(hashFile)
			targetHash := string(targetHashByte)
			Debugf("string content of " + hashFile + " is: " + targetHash)
			if targetHash == commitHash {
				needToSync = false
				Debugf("Skipping, because no diff found between " + srcDir + "(" + commitHash + ") and " + targetDir + "(" + targetHash + ")")
			} else {
				Debugf("Need to sync, because existing Git module: " + targetDir + " has commit " + targetHash + " and the to be synced commit is: " + commitHash)
			}
		}

	}
	if needToSync && er.returnCode == 0 {
		mutex.Lock()
		Infof("Need to sync " + targetDir)
		needSyncDirs = append(needSyncDirs, targetDir)
		if _, ok := needSyncEnvs[correspondingPuppetEnvironment]; !ok {
			needSyncEnvs[correspondingPuppetEnvironment] = empty
		}
		needSyncGitCount++
		mutex.Unlock()
		moduleDir := "modules"
		purgeWholeEnvDir := true
		// check if it is a control repo and already exists
		if isControlRepo && isDir(targetDir) {
			// then check if it contains a Puppetfile
			gitShowCmd := "git --git-dir " + srcDir + " show " + gitModule.tree + ":Puppetfile"
			executeResult := executeCommand(gitShowCmd, "", config.Timeout, true, false)
			Debugf("Executing " + gitShowCmd)
			if executeResult.returnCode != 0 {
				purgeWholeEnvDir = true
			} else {
				purgeWholeEnvDir = false

				// calculate the checksum of the deployed Puppetfile
				deployedPuppetfile := filepath.Join(targetDir, "Puppetfile")
				deployedPuppetfileChecksum := ""
				if fileExists(deployedPuppetfile) {
					deployedPuppetfileChecksum = getSha256sumFile(deployedPuppetfile)
				}

				// calculate the checksum of the upstream Puppetfile
				upstreamPuppetfileChecksum := getSha256sumString(executeResult.output)

				if deployedPuppetfileChecksum == upstreamPuppetfileChecksum {
					Debugf("Puppetfile checksum match for " + targetDir)
					// we can signal to skip module resolution if only the Puppetfile hasn't changed
					// but we can't easily return this info from here to resolvePuppetEnvironment
					// So we might need to store this state somewhere
					mutex.Lock()
					Debugf("Setting PuppetfileMatch for env: " + correspondingPuppetEnvironment)
					needSyncEnvs[correspondingPuppetEnvironment+":PuppetfileMatch"] = empty
					mutex.Unlock()
				}

				lines := strings.Split(executeResult.output, "\n")

				for _, line := range lines {
					if m := reModuledir.FindStringSubmatch(line); len(m) > 1 {
						// moduledir CLI parameter override
						if len(moduleDirParam) != 0 {
							moduleDir = moduleDirParam
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
			purgeDir(targetDir, "need to sync")
		} else {
			Infof("Detected control repo change, but trying to preserve module dir " + filepath.Join(targetDir, moduleDir))
			purgeControlRepoExceptModuledir(targetDir, moduleDir)
		}

		if !dryRun && !config.CloneGitModules || isControlRepo {
			if pfMode {
				purgeDir(targetDir, "git dir with changes in -puppetfile mode")
			}
			checkDirAndCreate(targetDir, "git dir")
			gitArchiveArgs := []string{"--git-dir", srcDir, "archive", gitModule.tree}
			cmd := exec.Command("git", gitArchiveArgs...)
			Debugf("Executing git --git-dir " + srcDir + " archive " + gitModule.tree)
			cmdOut, err := cmd.StdoutPipe()
			if err != nil {
				if !gitModule.ignoreUnreachable {
					Infof("Failed to populate module " + targetDir + " but ignore-unreachable is set. Continuing...")
				} else {
					return false
				}
				Fatalf("syncToModuleDir(): Failed to execute command: git --git-dir " + srcDir + " archive " + gitModule.tree + " Error: " + err.Error())
			}
			cmd.Start()

			before := time.Now()
			unTar(cmdOut, targetDir)
			duration := time.Since(before).Seconds()
			mutex.Lock()
			ioGitTime += duration
			mutex.Unlock()

			err = cmd.Wait()
			if err != nil {
				Fatalf("syncToModuleDir(): Failed to execute command: git --git-dir " + srcDir + " archive " + gitModule.tree + " Error: " + err.Error())
				//"\nIf you are using GitLab please ensure that you've added your deploy key to your repository." +
				//"\nThe Puppet environment which is using this unresolveable repository is " + correspondingPuppetEnvironment)
			}

			Verbosef("syncToModuleDir(): Executing git --git-dir " + srcDir + " archive " + gitModule.tree + " took " + strconv.FormatFloat(duration, 'f', 5, 64) + "s")

			commitHash := strings.TrimSuffix(er.output, "\n")
			if isControlRepo {
				Debugf("Writing to deploy file " + deployFile)
				dr := DeployResult{
					Name:      gitModule.tree,
					Signature: commitHash,
					StartedAt: startedAt,
				}
				writeStructJSONFile(deployFile, dr)
			} else {
				Debugf("Writing hash " + commitHash + " from command " + revParseCmd + " to " + hashFile)
				f, _ := os.Create(hashFile)
				defer f.Close()
				f.WriteString(commitHash)
				f.Sync()
			}

		} else if config.CloneGitModules {
			return doMirrorOrUpdate(gitModule, targetDir, 0)
		}
	}
	return true
}

func detectDefaultBranch(gitDir string) string {
	remoteShowOriginCmd := "git ls-remote --symref " + gitDir
	er := executeCommand(remoteShowOriginCmd, "", config.Timeout, false, false)
	foundRefs := strings.Split(er.output, "\n")
	if len(foundRefs) < 1 {
		Fatalf("Unable to detect default branch for git repository with command git ls-remote --symref " + gitDir)
	}
	// should look like this:
	// ref: refs/heads/main\tHEAD
	headBranchParts := strings.Split(foundRefs[0], "\t")
	defaultBranch := strings.TrimPrefix(string(headBranchParts[0]), "ref: refs/heads/")
	//fmt.Println(defaultBranch)
	return defaultBranch
}

func detectGitRemoteURLChange(d string, url string) bool {
	gitRemoteCmd := "git --git-dir " + d + " remote -v"

	er := executeCommand(gitRemoteCmd, "", config.Timeout, false, false)
	if er.returnCode != 0 {
		Warnf("WARN: Could not detect remote URL for git repository " + d + " trying to purge it and mirror it again")
		return true
	}

	f := strings.Fields(er.output)
	if len(f) < 3 {
		Warnf("WARN: Could not detect remote URL for git repository " + d + " trying to purge it and mirror it again")
		return true
	}
	configuredRemote := f[1]
	return configuredRemote != url
}
