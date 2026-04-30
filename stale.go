package main

import (
	"path/filepath"
	"strings"
)

func purgeUnmanagedContent(allBasedirs map[string]bool, allEnvironments map[string]bool) {
	if !stringSliceContains(config.PurgeLevels, "deployment") {
		if !stringSliceContains(config.PurgeLevels, "environment") {
			// nothing allowed to purge
			return
		}
	}
	for source, sa := range config.Sources {
		// fmt.Printf("source: %+v\n", sa)
		prefix := resolveSourcePrefix(source, sa)

		if len(environmentParam) > 0 {
			if !strings.HasPrefix(environmentParam, prefix) {
				Debugf("Skipping purging unmanaged content for source '" + source + "', because -environment parameter is set to " + environmentParam)
				continue
			}
		}

		// Clean up unknown environment directories
		if len(branchParam) == 0 {
			for basedir := range allBasedirs {
				globPath := filepath.Join(basedir, prefix+"*")
				Debugf("Glob'ing with path " + globPath)
				environments, _ := filepath.Glob(globPath)

				allowlistEnvironments := []string{}
				if len(config.DeploymentPurgeAllowList) > 0 {
					for _, wlpattern := range config.DeploymentPurgeAllowList {
						allowlistGlobPath := filepath.Join(basedir, wlpattern)
						Debugf("deployment_purge_allowlist Glob'ing with path " + allowlistGlobPath)
						we, _ := filepath.Glob(allowlistGlobPath)
						allowlistEnvironments = append(allowlistEnvironments, we...)
					}
				}

				for _, env := range environments {
					envPath := strings.Split(env, "/")
					envName := envPath[len(envPath)-1]
					if len(environmentParam) > 0 {
						if envName != environmentParam {
							Debugf("Skipping purging unmanaged content for Puppet environment '" + envName + "', because -environment parameter is set to " + environmentParam)
							continue
						}
					}
					if stringSliceContains(config.PurgeLevels, "deployment") {
						Debugf("Checking if environment should exist: " + env)
						if allEnvironments[env] {
							Debugf("Not purging environment " + env + " because it is managed")
						} else if stringSliceContains(allowlistEnvironments, env) {
							Debugf("Not purging environment " + env + " due to deployment_purge_allowlist match")
						} else {
							if checkRemoteSourceOfEnvironment(env, config.Sources) {
								// TODO: add test for this using https://github.com/xorpaul/g10k_purge_env_test/branches
								Debugf("Purging environment " + env + " because its remote source matches configured source remote")
								Infof("Removing unmanaged environment " + env)
								if !dryRun {
									purgeDir(env, "purgeStaleContent()")
								}
							} else {
								Debugf("Purging environment " + env + " because its remote source belongs to a different source remote")
								Infof("Removing unmanaged environment " + env)
								if !dryRun {
									purgeDir(env, "purgeStaleContent()")
								}
							}
						}
					}
				}
			}
		}
	}
}

func checkRemoteSourceOfEnvironment(environmentDir string, configSources map[string]Source) bool {
	// check for .g10k-deploy.json inside the environment directory and read source remote from there
	// if it matches then return true

	dr := DeployResult{}
	deployFile := filepath.Join(environmentDir, ".g10k-deploy.json")
	if fileExists(deployFile) {
		dr = readDeployResultFile(deployFile)
	} else {
		Debugf("found no " + deployFile + " file, this folder is likely unmanaged and will be purged")
	}

	for _, source := range configSources {
		Debugf("Comparing source remote " + source.Remote + " with deploy result git url " + dr.GitURL)
		if dr.GitURL == source.Remote {
			return true
		}
	}
	return false
}

func purgeControlRepoExceptModuledir(dir string, moduleDir string) {
	moduleDir = filepath.Join(dir, moduleDir)

	allowlistFolders := []string{}
	if len(config.PurgeAllowList) > 0 {
		for _, wlpattern := range config.PurgeAllowList {
			allowlistGlobPath := filepath.Join(dir, wlpattern)
			Debugf("purge_allowlist Glob'ing with path " + allowlistGlobPath)
			we, _ := filepath.Glob(allowlistGlobPath)
			allowlistFolders = append(allowlistFolders, we...)
		}
	}

	globPath := filepath.Join(dir, "*")
	Debugf("Glob'ing with path " + globPath)
	folders, _ := filepath.Glob(globPath)
	for _, folder := range folders {
		if folder == moduleDir || strings.HasPrefix(folder, moduleDir) {
			continue
		} else if stringSliceContains(allowlistFolders, folder) {
			Debugf("Not deleting " + folder + " due to purge_allowlist match")
			continue
		} else {
			Debugf("deleting " + folder)
			purgeDir(folder, "purgeControlRepoExceptModuledir")
		}

	}

}
