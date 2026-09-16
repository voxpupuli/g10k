package main

import (
	"path/filepath"
	"strings"
)

func (rt *Runtime) purgeUnmanagedContent(allBasedirs map[string]bool, allEnvironments map[string]bool) {
	if !stringSliceContains(rt.Config.PurgeLevels, "deployment") {
		if !stringSliceContains(rt.Config.PurgeLevels, "environment") {
			// nothing allowed to purge
			return
		}
	}
	for source, sa := range rt.Config.Sources {
		// fmt.Printf("source: %+v\n", sa)
		prefix := rt.resolveSourcePrefix(source, sa)

		if len(rt.Environment) > 0 {
			if !strings.HasPrefix(rt.Environment, prefix) {
				rt.Debugf("Skipping purging unmanaged content for source '" + source + "', because -environment parameter is set to " + rt.Environment)
				continue
			}
		}

		// Clean up unknown environment directories
		if len(rt.Branch) == 0 {
			for basedir := range allBasedirs {
				globPath := filepath.Join(basedir, prefix+"*")
				rt.Debugf("Glob'ing with path " + globPath)
				environments, _ := filepath.Glob(globPath)

				allowlistEnvironments := []string{}
				if len(rt.Config.DeploymentPurgeAllowList) > 0 {
					for _, wlpattern := range rt.Config.DeploymentPurgeAllowList {
						allowlistGlobPath := filepath.Join(basedir, wlpattern)
						rt.Debugf("deployment_purge_allowlist Glob'ing with path " + allowlistGlobPath)
						we, _ := filepath.Glob(allowlistGlobPath)
						allowlistEnvironments = append(allowlistEnvironments, we...)
					}
				}

				for _, env := range environments {
					envPath := strings.Split(env, "/")
					envName := envPath[len(envPath)-1]
					if len(rt.Environment) > 0 {
						if envName != rt.Environment {
							rt.Debugf("Skipping purging unmanaged content for Puppet environment '" + envName + "', because -environment parameter is set to " + rt.Environment)
							continue
						}
					}
					if stringSliceContains(rt.Config.PurgeLevels, "deployment") {
						rt.Debugf("Checking if environment should exist: " + env)
						if allEnvironments[env] {
							rt.Debugf("Not purging environment " + env + " because it is managed")
						} else if stringSliceContains(allowlistEnvironments, env) {
							rt.Debugf("Not purging environment " + env + " due to deployment_purge_allowlist match")
						} else {
							if rt.checkRemoteSourceOfEnvironment(env, rt.Config.Sources) {
								// TODO: add test for this using https://github.com/xorpaul/g10k_purge_env_test/branches
								rt.Debugf("Purging environment " + env + " because its remote source matches configured source remote")
								rt.Infof("Removing unmanaged environment " + env)
								if !rt.DryRun {
									rt.purgeDir(env, "purgeStaleContent()")
								}
							} else {
								rt.Debugf("Purging environment " + env + " because its remote source belongs to a different source remote")
								rt.Infof("Removing unmanaged environment " + env)
								if !rt.DryRun {
									rt.purgeDir(env, "purgeStaleContent()")
								}
							}
						}
					}
				}
			}
		}
	}
}

func (rt *Runtime) checkRemoteSourceOfEnvironment(environmentDir string, configSources map[string]Source) bool {
	// check for .g10k-deploy.json inside the environment directory and read source remote from there
	// if it matches then return true

	dr := DeployResult{}
	deployFile := filepath.Join(environmentDir, ".g10k-deploy.json")
	if fileExists(deployFile) {
		dr = rt.readDeployResultFile(deployFile)
	} else {
		rt.Debugf("found no " + deployFile + " file, this folder is likely unmanaged and will be purged")
	}

	for _, source := range configSources {
		rt.Debugf("Comparing source remote " + source.Remote + " with deploy result git url " + dr.GitURL)
		if dr.GitURL == source.Remote {
			return true
		}
	}
	return false
}

func (rt *Runtime) purgeControlRepoExceptModuledir(dir string, moduleDir string) {
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
	rt.Debugf("Glob'ing with path " + globPath)
	folders, _ := filepath.Glob(globPath)
	for _, folder := range folders {
		if folder == moduleDir || strings.HasPrefix(folder, moduleDir) {
			continue
		} else if stringSliceContains(allowlistFolders, folder) {
			Debugf("Not deleting " + folder + " due to purge_allowlist match")
			continue
		} else {
			rt.Debugf("deleting " + folder)
			rt.purgeDir(folder, "purgeControlRepoExceptModuledir")
		}

	}

}
