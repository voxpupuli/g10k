package main

import (
	"crypto/md5"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/fatih/color"
	"github.com/klauspost/pgzip"
	"github.com/tidwall/gjson"
)

// createSymlinkIdempotent creates a symlink from linkPath pointing to targetPath.
// If the symlink already exists and points to targetPath it is treated as success,
// allowing concurrent goroutines to race on the same symlink without fataling.
func (rt *Runtime) createSymlinkIdempotent(targetPath, linkPath string) error {
	if err := os.Symlink(targetPath, linkPath); err != nil {
		if os.IsExist(err) {
			existing, readErr := os.Readlink(linkPath)
			if readErr != nil {
				return err
			}
			if existing == targetPath {
				rt.Debugf("symlink " + linkPath + " already points to " + targetPath + ", skipping")
				return nil
			}
			return fmt.Errorf("symlink %s already exists pointing to %s, expected %s", linkPath, existing, targetPath)
		}
		return err
	}
	return nil
}

func (rt *Runtime) checkDeprecation(fm ForgeModule, lastCheckedFile string) bool {
	// check content of lastCheckedFile (which should be the Forge API response body) if the module is deprecated
	// return false if the api needs to be queried again
	if fileInfo, err := os.Stat(lastCheckedFile); err == nil {
		if fileInfo.Size() < 1 {
			rt.Debugf("found empty file " + lastCheckedFile)
			return false
		} else if fm.cacheTTL > 0 && fileInfo.ModTime().Add(fm.cacheTTL).Before(time.Now()) {
			return false
		} else {
			json, err := os.ReadFile(lastCheckedFile)
			if err != nil {
				rt.Fatalf("doModuleInstallOrNothing(): Error while reading Forge API result from file " + lastCheckedFile + err.Error())
			}
			_ = rt.parseForgeAPIResult(string(json), fm)
			return true
		}
	}
	return false
}

func (rt *Runtime) doModuleInstallOrNothing(fm ForgeModule) {
	moduleName := fm.author + "-" + fm.name
	moduleVersion := fm.version
	workDir := filepath.Join(rt.Config.ForgeCacheDir, moduleName+"-"+fm.version)
	lastCheckedFile := filepath.Join(rt.Config.ForgeCacheDir, moduleName+"-latest-last-checked")
	fr := ForgeResult{false, fm.version, "", 0}
	if rt.Check4Update {
		moduleVersion = "latest"
	}
	switch moduleVersion {
	case "latest":
		if !isDir(workDir) {
			rt.Debugf(workDir + " does not exist, fetching Forge module")
			// check forge API what the latest version is
			fr = rt.queryForgeAPI(fm)
			if fr.needToGet {
				if _, ok := rt.UniqueForgeModules[moduleName+"-"+fr.versionNumber]; ok {
					rt.Debugf("no need to fetch Forge module " + moduleName + " in latest, because latest is " + fr.versionNumber + " and that will already be fetched")
					fr.needToGet = false
					versionDir := filepath.Join(rt.Config.ForgeCacheDir, moduleName+"-"+fr.versionNumber)
					absolutePath, err := filepath.Abs(versionDir)
					rt.Debugf("trying to create symlink " + workDir + " pointing to " + absolutePath)
					if err != nil {
						rt.Fatalf("doModuleInstallOrNothing(): Error while resolving absolute file path for " + versionDir + " Error: " + err.Error())
					}
					if err := rt.createSymlinkIdempotent(absolutePath, workDir); err != nil {
						rt.Fatalf("doModuleInstallOrNothing(): 1 Error while creating symlink " + workDir + " pointing to " + absolutePath + " Error: " + err.Error())
					}
					//} else {
					//Debugf("need to fetch Forge module " + moduleName + " in latest, because version " + fr.versionNumber + " will not be fetched already")

					//fmt.Println(needToGet)
				}
			}
		} else {
			if fm.cacheTTL > 0 {
				//Debugf("checking for " + lastCheckedFile)
				if fileInfo, err := os.Lstat(lastCheckedFile); err == nil {
					//Debugf("found " + lastCheckedFile + " with mTime " + fileInfo.ModTime().String())
					if fileInfo.ModTime().Add(fm.cacheTTL).After(time.Now()) {
						rt.Debugf("No need to check forge API if latest version of module " + moduleName + " has been updated, because last-checked file " + lastCheckedFile + " is not older than " + fm.cacheTTL.String())
						// need to add the current (cached!) -latest version number to the latestForgeModules, because otherwise we would always sync this module, because 1.4.1 != -latest
						me := rt.readModuleMetadata(workDir + "/metadata.json")
						rt.LatestForgeModules.Lock()
						rt.LatestForgeModules.m[moduleName] = me.version
						rt.LatestForgeModules.Unlock()

						if rt.checkDeprecation(fm, lastCheckedFile) {
							return
						}

					}
				}
			}
			// check forge API if latest version of this module has been updated
			rt.Debugf("check forge API if latest version of module " + moduleName + " has been updated")
			// XXX: disable adding If-Modified-Since header for now
			// because then the latestForgeModules does not get set with the actual module version for latest
			// maybe if received 304 get the actual version from the -latest symlink
			fr = rt.queryForgeAPI(fm)
			//fmt.Println(needToGet)
		}

	case "present":
		// ensure that a latest version this module exists
		latestDir := filepath.Join(rt.Config.ForgeCacheDir, moduleName+"-latest")
		if !isDir(latestDir) {
			if _, ok := rt.UniqueForgeModules[moduleName+"-latest"]; ok {
				rt.Debugf("we got " + fm.author + "-" + fm.name + "-" + fm.version + ", but no " + latestDir + " to use, but -latest is already being fetched.")
				return
			}
			rt.Debugf("we got " + fm.author + "-" + fm.name + "-" + fm.version + ", but no " + latestDir + " to use. Getting -latest")
			fm.version = "latest"
			rt.doModuleInstallOrNothing(fm)
			return
		}
		rt.Debugf("Nothing to do for module " + fm.author + "-" + fm.name + "-" + fm.version + ", because " + latestDir + " exists")
	default:
		if !isDir(workDir) {
			_ = rt.queryForgeAPI(fm)
			fr.needToGet = true
		} else {
			if !rt.checkDeprecation(fm, lastCheckedFile) {
				_ = rt.queryForgeAPI(fm)
			}
			rt.Debugf("Using cache for " + moduleName + " in version " + moduleVersion + " because " + workDir + " exists")
			return
		}
	}

	//log.Println("fr.needToGet for ", m, fr.needToGet)

	if fr.needToGet {
		if fm.version != "latest" {
			rt.Debugf("Trying to remove: " + workDir)
			_ = os.Remove(workDir)
		} else {
			// os.Readlink error is okay
			versionDir, _ := os.Readlink(workDir)
			if versionDir == filepath.Join(rt.Config.ForgeCacheDir, moduleName+"-"+fr.versionNumber) {
				rt.Debugf("No reason to re-symlink again")
			} else {
				if isDir(workDir) {
					rt.Debugf("Trying to remove symlink: " + workDir)
					_ = os.Remove(workDir)
				}
				versionDir = filepath.Join(rt.Config.ForgeCacheDir, moduleName+"-"+fr.versionNumber)
				absolutePath, err := filepath.Abs(versionDir)
				if err != nil {
					rt.Fatalf("doModuleInstallOrNothing(): Error while resolving absolute file path for " + versionDir + " Error: " + err.Error())
				}
				rt.Debugf("trying to create symlink " + workDir + " pointing to " + absolutePath)
				if err := rt.createSymlinkIdempotent(absolutePath, workDir); err != nil {
					rt.Fatalf("doModuleInstallOrNothing(): 2 Error while creating symlink " + workDir + " pointing to " + absolutePath + err.Error())
				}
			}
		}
		rt.downloadForgeModule(moduleName, fr.versionNumber, fm, 1)
	}

}

func (rt *Runtime) queryForgeAPI(fm ForgeModule) ForgeResult {
	baseURL := rt.Config.ForgeBaseURL
	if len(fm.baseURL) > 0 {
		baseURL = fm.baseURL
	}
	url := baseURL + "/v3/modules/" + fm.author + "-" + fm.name + "?exclude_fields=changelog+readme+license+releases"
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		rt.Fatalf("queryForgeAPI(): Error creating GET request for Puppetlabs forge API" + err.Error())
	}
	req.Header.Set("User-Agent", "https://github.com/xorpaul/g10k/")
	req.Header.Set("Connection", "keep-alive")

	proxyURL, err := http.ProxyFromEnvironment(req)
	if err != nil {
		rt.Fatalf("queryForgeAPI(): Error while getting http proxy with golang http.ProxyFromEnvironment()" + err.Error())
	}
	client := &http.Client{Transport: &http.Transport{Proxy: http.ProxyURL(proxyURL)}}
	before := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		if rt.Config.UseCacheFallback {
			rt.Warnf("Forge API error, trying to use cache for module " + fm.author + "/" + fm.author + "-" + fm.name)
			_ = rt.getLatestCachedModule(fm)
			return ForgeResult{false, "", "", 0}
		}
		rt.Fatalf("queryForgeAPI(): Error while issuing the HTTP request to " + url + " Error: " + err.Error())
	}
	duration := time.Since(before).Seconds()
	rt.Verbosef("Querying Forge API " + url + " took " + strconv.FormatFloat(duration, 'f', 5, 64) + "s")

	rt.Mutex.Lock()
	rt.SyncForgeTime += duration
	rt.Mutex.Unlock()
	defer func() { _ = resp.Body.Close() }()

	switch resp.StatusCode {
	case http.StatusOK:
		// need to get latest version
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			rt.Fatalf("queryForgeAPI(): Error while reading response body for Forge module " + fm.name + " from " + url + ": " + err.Error())
		}

		json := string(body)
		fr := rt.parseForgeAPIResult(json, fm)

		lastCheckedFile := filepath.Join(rt.Config.ForgeCacheDir, fm.author+"-"+fm.name+"-latest-last-checked")
		rt.Debugf("writing last-checked file " + lastCheckedFile)
		f, _ := os.Create(lastCheckedFile)
		defer func() { _ = f.Close() }()
		_, _ = f.WriteString(json) // FIXME: error should be handled

		return ForgeResult{true, fr.versionNumber, fr.md5sum, fr.fileSize}

	case http.StatusNotModified:
		rt.Debugf("Got 304 nothing to do for module " + fm.author + "-" + fm.name)
		return ForgeResult{false, "", "", 0}
	case http.StatusNotFound:
		rt.Fatalf("Received 404 from Forge for module " + fm.author + "-" + fm.name + " using URL " + url + " Does the module really exist and is it correctly named?")
		return ForgeResult{false, "", "", 0}
	}
	rt.Fatalf("Unexpected response code " + resp.Status)
	return ForgeResult{false, "", "", 0}
}

// parseForgeAPIResult parses the JSON response of the Forge API
func (rt *Runtime) parseForgeAPIResult(json string, fm ForgeModule) ForgeResult {

	before := time.Now()
	currentRelease := gjson.Get(json, "current_release").Map()
	deprecatedTimestamp := gjson.Get(json, "deprecated_at")
	successorModule := gjson.Get(json, "superseded_by").Map()
	duration := time.Since(before).Seconds()

	version := currentRelease["version"].String()
	modulemd5sum := currentRelease["file_md5"].String()
	moduleFilesize := currentRelease["file_size"].Int()

	rt.Mutex.Lock()
	rt.ForgeJSONParseTime += duration
	rt.Mutex.Unlock()

	if deprecatedTimestamp.Exists() && deprecatedTimestamp.Value() != nil {
		supersededText := ""
		if successorModule["slug"].Exists() {
			supersededText = " The author has suggested " + successorModule["slug"].String() + " as its replacement"
		}
		// check the verbosity level
		// otherwise these warnings mess up the progress bars
		if rt.Info || rt.Debug {
			rt.Warnf("WARN: Forge module " + fm.author + "-" + fm.name + " has been deprecated by its author since " + deprecatedTimestamp.String() + supersededText)
		} else {
			rt.Mutex.Lock()
			rt.forgeModuleDeprecationNotice += "WARN: Forge module " + fm.author + "-" + fm.name + " has been deprecated by its author since " + deprecatedTimestamp.String() + supersededText + "\n"
			rt.Mutex.Unlock()
		}
	}

	if len(version) < 1 {
		rt.Fatalf("ERROR: could not determine version of module " + fm.author + "/" + fm.name)
	}

	rt.Debugf("found version " + version + " for " + fm.name + "-latest")
	rt.LatestForgeModules.Lock()
	rt.LatestForgeModules.m[fm.author+"-"+fm.name] = version
	rt.LatestForgeModules.Unlock()

	return ForgeResult{true, version, modulemd5sum, moduleFilesize}
}

// getMetadataForgeModule queries the configured Puppet Forge and return
func (rt *Runtime) getMetadataForgeModule(fm ForgeModule) ForgeModule {
	baseURL := rt.Config.ForgeBaseURL
	if len(fm.baseURL) > 0 {
		baseURL = fm.baseURL
	}
	url := baseURL + "/v3/releases/" + fm.author + "-" + fm.name + "-" + fm.version
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		rt.Fatalf("getMetadataForgeModule(): Error while creating GET http request with url " + url + " Error: " + err.Error())
	}
	req.Header.Set("User-Agent", "https://github.com/xorpaul/g10k/")
	req.Header.Set("Connection", "keep-alive")
	proxyURL, err := http.ProxyFromEnvironment(req)
	if err != nil {
		rt.Fatalf("getMetadataForgeModule(): Error while getting http proxy with golang http.ProxyFromEnvironment()" + err.Error())
	}
	client := &http.Client{Transport: &http.Transport{Proxy: http.ProxyURL(proxyURL)}}
	before := time.Now()
	rt.Debugf("GETing " + url)
	resp, err := client.Do(req)
	duration := time.Since(before).Seconds()
	rt.Verbosef("GETing Forge metadata from " + url + " took " + strconv.FormatFloat(duration, 'f', 5, 64) + "s")
	rt.Mutex.Lock()
	rt.SyncForgeTime += duration
	rt.Mutex.Unlock()
	if err != nil {
		rt.Fatalf("getMetadataForgeModule(): Error while querying metadata for Forge module " + fm.name + " from " + url + ": " + err.Error())
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusOK {
		body, err := io.ReadAll(resp.Body)

		if err != nil {
			rt.Fatalf("getMetadataForgeModule(): Error while reading response body for Forge module " + fm.name + " from " + url + ": " + err.Error())
		}

		before := time.Now()
		currentRelease := gjson.Parse(string(body)).Map()
		duration := time.Since(before).Seconds()
		modulemd5sum := currentRelease["file_md5"].String()
		moduleFilesize := currentRelease["file_size"].Int()
		rt.Debugf("module: " + fm.author + "/" + fm.name + " modulemd5sum: " + modulemd5sum + " moduleFilesize: " + strconv.FormatInt(moduleFilesize, 10))

		rt.Mutex.Lock()
		rt.ForgeJSONParseTime += duration
		rt.Mutex.Unlock()

		return ForgeModule{md5sum: modulemd5sum, fileSize: moduleFilesize}
	}
	rt.Fatalf("getMetadataForgeModule(): Unexpected response code while GETing " + url + " " + resp.Status)
	return ForgeModule{}
}

func (rt *Runtime) extractForgeModule(wgForgeModule *sync.WaitGroup, file *io.PipeReader, fileName string) {
	defer wgForgeModule.Done()
	funcName := funcName()

	before := time.Now()
	fileReader, err := pgzip.NewReader(file)

	rt.unTar(fileReader, rt.Config.ForgeCacheDir)

	if err != nil {
		rt.Fatalf(funcName + "(): pgzip reader error for module " + fileName + " error:" + err.Error())
	}
	defer func() { _ = fileReader.Close() }()

	duration := time.Since(before).Seconds()
	rt.Verbosef("Extracting " + filepath.Join(rt.Config.ForgeCacheDir, fileName) + " took " + strconv.FormatFloat(duration, 'f', 5, 64) + "s")
	rt.Mutex.Lock()
	rt.IoForgeTime += duration
	rt.Mutex.Unlock()
}

func (rt *Runtime) downloadForgeModule(name string, version string, fm ForgeModule, retryCount int) {
	funcName := funcName()
	var wgForgeModule sync.WaitGroup

	extractR, extractW := io.Pipe()
	saveFileR, saveFileW := io.Pipe()

	//url := "https://forgeapi.puppet.com/v3/files/puppetlabs-apt-2.1.1.tar.gz"
	fileName := name + "-" + version + ".tar.gz"

	if !isDir(filepath.Join(rt.Config.ForgeCacheDir, name+"-"+version)) {
		baseURL := rt.Config.ForgeBaseURL
		if len(fm.baseURL) > 0 {
			baseURL = fm.baseURL
		}
		url := baseURL + "/v3/files/" + fileName
		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			rt.Fatalf("getMetadataForgeModule(): Error while creating GET http request with url " + url + " Error: " + err.Error())
		}
		req.Header.Set("User-Agent", "https://github.com/xorpaul/g10k/")
		req.Header.Set("Connection", "close")
		proxyURL, err := http.ProxyFromEnvironment(req)
		if err != nil {
			rt.Fatalf(funcName + "(): Error while getting http proxy with golang http.ProxyFromEnvironment()" + err.Error())
		}
		client := &http.Client{Transport: &http.Transport{Proxy: http.ProxyURL(proxyURL)}}
		before := time.Now()
		rt.Debugf("GETing " + url)
		resp, err := client.Do(req)
		duration := time.Since(before).Seconds()
		rt.Verbosef("GETing " + url + " took " + strconv.FormatFloat(duration, 'f', 5, 64) + "s")
		rt.Mutex.Lock()
		rt.SyncForgeTime += duration
		rt.Mutex.Unlock()
		if err != nil {
			rt.Fatalf(funcName + "(): Error while GETing Forge module " + name + " from " + url + ": " + err.Error())
		}
		defer func() { _ = resp.Body.Close() }()

		switch resp.StatusCode {
		case http.StatusOK:
			wgForgeModule.Add(1)
			go func() {
				defer wgForgeModule.Done()
				targetFileName := filepath.Join(rt.Config.ForgeCacheDir, fileName)
				rt.Debugf(funcName + "(): Trying to create " + targetFileName)
				out, err := os.Create(targetFileName)
				if err != nil {
					rt.Fatalf(funcName + "(): Error while creating file for Forge module " + targetFileName + " Error: " + err.Error())
				}
				defer func() { _ = out.Close() }()
				_, _ = io.Copy(out, saveFileR) // FIXME: error should be handled
				rt.Debugf(funcName + "(): Finished creating " + targetFileName)
			}()
			wgForgeModule.Add(1)
			go rt.extractForgeModule(&wgForgeModule, extractR, fileName)
			wgForgeModule.Add(1)
			go func() {
				defer wgForgeModule.Done()

				// after completing the copy, we need to close
				// the PipeWriters to propagate the EOF to all
				// PipeReaders to avoid deadlock
				defer func() { _ = extractW.Close() }()
				defer func() { _ = saveFileW.Close() }()

				// build the multiwriter for all the pipes
				mw := io.MultiWriter(extractW, saveFileW)

				// copy the data into the multiwriter
				if _, err := io.Copy(mw, resp.Body); err != nil {
					rt.Fatalf("Error while writing to MultiWriter " + err.Error())
				}
			}()
		case http.StatusNotFound:
			rt.Fatalf("Received 404 from Forge using URL " + url +
				"\nCheck if the module name '" + fm.author + "-" + fm.name + "' and version '" + version + "' really exist" +
				"\nUsed in Puppet environment '" + fm.sourceBranch + "'")
		default:
			rt.Fatalf("Unexpected response code while GETing " + url + " " + resp.Status)
		}
	} else {
		rt.Debugf("Using cache for Forge module " + name + " version: " + version)
	}
	wgForgeModule.Wait()

	if rt.CheckSum || fm.sha256sum != "" {
		fm.version = version
		if rt.doForgeModuleIntegrityCheck(fm) {
			if retryCount == 0 {
				rt.Fatalf("downloadForgeModule(): giving up for Puppet module " + name + " version: " + version)
			}
			rt.Warnf("Retrying...")
			rt.purgeDir(filepath.Join(rt.Config.ForgeCacheDir, fileName), "downloadForgeModule()")
			rt.purgeDir(strings.ReplaceAll(filepath.Join(rt.Config.ForgeCacheDir, fileName), ".tar.gz", "/"), "downloadForgeModule()")
			// retry if hash sum mismatch found
			rt.downloadForgeModule(name, version, fm, retryCount-1)
		}
	}

}

// readModuleMetadata returns the Forgemodule struct of the given module file path
func (rt *Runtime) readModuleMetadata(file string) ForgeModule {
	content, _ := os.ReadFile(file)

	before := time.Now()
	name := gjson.Get(string(content), "name").String()
	version := gjson.Get(string(content), "version").String()
	author := gjson.Get(string(content), "author").String()
	duration := time.Since(before).Seconds()
	rt.Mutex.Lock()
	rt.metadataJSONParseTime += duration
	rt.Mutex.Unlock()

	rt.Debugf("Found in file " + file + " name: " + name + " version: " + version + " author: " + author)

	moduleName := "N/A"
	if strings.Contains(name, "-") {
		moduleName = strings.Split(name, "-")[1]
	} else {
		rt.Debugf("Error: Something went wrong while decoding file " + file + " searching for the module name (found for name: " + name + "), version and author")
	}

	return ForgeModule{name: moduleName, version: version, author: strings.ToLower(author)}
}

func (rt *Runtime) resolveForgeModules(modules map[string]ForgeModule) {
	defer rt.timeTrack(time.Now(), funcName())
	if len(modules) <= 0 {
		rt.Debugf("empty ForgeModule[] found, skipping...")
		return
	}
	// Dummy channel to coordinate the number of concurrent goroutines.
	// This channel should be buffered otherwise we will be immediately blocked
	// when trying to fill it.

	rt.Debugf("Resolving " + strconv.Itoa(len(modules)) + " Forge modules with " + strconv.Itoa(rt.Config.Maxworker) + " workers")
	concurrentGoroutines := make(chan struct{}, rt.Config.Maxworker)
	// Fill the dummy channel with config.Maxworker empty struct.
	for i := 0; i < rt.Config.Maxworker; i++ {
		concurrentGoroutines <- struct{}{}
	}

	// The done channel indicates when a single goroutine has
	// finished its job.
	done := make(chan bool)
	// The waitForAllJobs channel allows the main program
	// to wait until we have indeed done all the jobs.
	waitForAllJobs := make(chan bool)
	// Collect all the jobs, and since the job is finished, we can
	// release another spot for a goroutine.
	go func() {
		for i := 1; i <= len(modules); i++ {
			<-done
			// Say that another goroutine can now start.
			concurrentGoroutines <- struct{}{}
		}
		// We have collected all the jobs, the program can now terminate
		waitForAllJobs <- true
	}()
	wg := sync.WaitGroup{}
	wg.Add(len(modules))

	for m, fm := range modules {
		go func(m string, fm ForgeModule) {
			// Try to receive from the concurrentGoroutines channel. When we have something,
			// it means we can start a new goroutine because another one finished.
			// Otherwise, it will block the execution until an execution
			// spot is available.
			<-concurrentGoroutines
			defer wg.Done()
			rt.Debugf("resolveForgeModules(): Trying to get forge module " + m + " with Forge base url " + fm.baseURL + " and CacheTtl set to " + fm.cacheTTL.String())
			rt.doModuleInstallOrNothing(fm)
			done <- true
		}(m, fm)
	}
	// Wait for all jobs to finish
	<-waitForAllJobs
	wg.Wait()
}

func (rt *Runtime) check4ForgeUpdate(moduleName string, currentVersion string, latestVersion string) {
	rt.Verbosef("found currently deployed Forge module " + moduleName + " in version: " + currentVersion)
	rt.Verbosef("found latest Forge module of " + moduleName + " in version: " + latestVersion)
	if currentVersion != latestVersion {
		color.Yellow("ATTENTION: Forge module: " + moduleName + " latest: " + latestVersion + " currently deployed: " + currentVersion)
		rt.NeedSyncForgeCount++
	}
}

func (rt *Runtime) doForgeModuleIntegrityCheck(m ForgeModule) bool {
	funcName := funcName()
	var wgCheckSum sync.WaitGroup

	wgCheckSum.Add(1)
	fmm := ForgeModule{}
	go func(m ForgeModule) {
		defer wgCheckSum.Done()
		fmm = rt.getMetadataForgeModule(m)
		rt.Debugf(funcName + "(): target md5 hash sum: " + fmm.md5sum)
		if m.sha256sum != "" {
			rt.Debugf(funcName + "(): target sha256 hash sum from Puppetfile: " + m.sha256sum)
		}
	}(m)

	calculatedMd5Sum := ""
	calculatedSha256Sum := ""
	// http://rodaine.com/2015/04/async-split-io-reader-in-golang/
	// create the pipes
	md5R, md5W := io.Pipe()
	sha256R, sha256W := io.Pipe()
	var calculatedArchiveSize int64
	fileName := filepath.Join(rt.Config.ForgeCacheDir, m.author+"-"+m.name+"-"+m.version+".tar.gz")

	// md5 sum
	wgCheckSum.Add(1)
	go func(md5R *io.PipeReader) {
		defer wgCheckSum.Done()
		before := time.Now()
		hashmd5 := md5.New()
		if _, err := io.Copy(hashmd5, md5R); err != nil {
			rt.Fatalf(funcName + "(): Error while reading Forge module archive " + fileName + " ! Error: " + err.Error())
		}
		duration := time.Since(before).Seconds()
		rt.Verbosef("Calculating md5 sum for " + fileName + " took " + strconv.FormatFloat(duration, 'f', 5, 64) + "s")
		calculatedMd5Sum = hex.EncodeToString(hashmd5.Sum(nil))
		rt.Debugf(funcName + "(): calculated md5 hash sum: " + calculatedMd5Sum)
	}(md5R)

	if m.sha256sum != "" {
		// sha256 sum
		wgCheckSum.Add(1)
		go func(sha256R *io.PipeReader) {
			defer wgCheckSum.Done()
			before := time.Now()
			hashSha256 := sha256.New()
			if _, err := io.Copy(hashSha256, sha256R); err != nil {
				rt.Fatalf(funcName + "(): Error while reading Forge module archive " + fileName + " ! Error: " + err.Error())
			}
			duration := time.Since(before).Seconds()
			rt.Verbosef("Calculating sha256 sum for " + fileName + " took " + strconv.FormatFloat(duration, 'f', 5, 64) + "s")
			calculatedSha256Sum = hex.EncodeToString(hashSha256.Sum(nil))
			rt.Debugf(funcName + "(): calculated sha256 hash sum: " + calculatedSha256Sum)
		}(sha256R)
	}

	wgCheckSum.Add(1)
	go func() {
		defer wgCheckSum.Done()

		// after completing the copy, we need to close
		// the PipeWriters to propagate the EOF to all
		// PipeReaders to avoid deadlock
		defer func() { _ = md5W.Close() }()
		if m.sha256sum != "" {
			defer func() { _ = sha256W.Close() }()
		}

		var mw io.Writer
		if m.sha256sum != "" {
			// build the multiwriter for all the pipes
			mw = io.MultiWriter(md5W, sha256W)
		} else {
			mw = io.MultiWriter(md5W)
		}

		before := time.Now()
		if fi, err := os.Stat(fileName); err == nil {
			calculatedArchiveSize = fi.Size()
			file, err := os.Open(fileName)
			if err != nil {
				rt.Fatalf("Can't access Forge module archive " + fileName + " ! Error: " + err.Error())
			}
			defer func() { _ = file.Close() }()

			// copy the data into the multiwriter
			if _, err := io.Copy(mw, file); err != nil {
				rt.Fatalf("Error while writing to MultiWriter " + err.Error())
			}

		} else {
			rt.Fatalf("Can't access Forge module archive " + fileName + " ! Error: " + err.Error())
		}
		duration := time.Since(before).Seconds()
		rt.Verbosef("Calculating hash sum(s) for " + fileName + " took " + strconv.FormatFloat(duration, 'f', 5, 64) + "s")
		rt.Debugf(funcName + "(): calculated archive size: " + strconv.FormatInt(calculatedArchiveSize, 10))
	}()

	wgCheckSum.Wait()

	if fmm.md5sum != calculatedMd5Sum {
		rt.Warnf("WARNING: calculated md5sum " + calculatedMd5Sum + " for " + fileName + " does not match expected md5sum " + fmm.md5sum)
		return true
	}
	if m.sha256sum != calculatedSha256Sum {
		rt.Warnf("WARNING: calculated sha256sum " + calculatedSha256Sum + " for " + fileName + " does not match expected sha256sum " + m.sha256sum)
		return true
	}
	if fmm.fileSize != calculatedArchiveSize {
		rt.Warnf("WARNING: calculated file size " + strconv.FormatInt(calculatedArchiveSize, 10) + " for " + fileName + " does not match expected file size " + strconv.FormatInt(fmm.fileSize, 10))
		return true
	}
	rt.Debugf("calculated file size " + strconv.FormatInt(calculatedArchiveSize, 10) + " for " + fileName + " does match expected file size " + strconv.FormatInt(fmm.fileSize, 10))
	rt.Debugf("calculated md5sum " + calculatedMd5Sum + " for " + fileName + " does match expected md5sum " + fmm.md5sum)
	if m.sha256sum != "" {
		rt.Debugf("calculated sha256sum " + calculatedSha256Sum + " for " + fileName + " does match expected sha256sum " + m.sha256sum)
	}
	return false

}

func (rt *Runtime) syncForgeToModuleDir(name string, m ForgeModule, moduleDir string, correspondingPuppetEnvironment string) {
	funcName := funcName()
	rt.Mutex.Lock()
	rt.SyncForgeCount++
	rt.Mutex.Unlock()
	moduleName := m.author + "-" + m.name
	//Debugf("m.name " + m.name + " m.version " + m.version + " moduleName " + moduleName)
	targetDir := filepath.Join(moduleDir, m.name)
	metadataFile := filepath.Join(targetDir, "metadata.json")
	if m.version == "present" {
		if fileExists(metadataFile) {
			rt.Debugf("Nothing to do, found existing Forge module: " + targetDir)
			if rt.Check4Update {
				me := rt.readModuleMetadata(metadataFile)
				rt.LatestForgeModules.RLock()
				rt.check4ForgeUpdate(m.name, me.version, rt.LatestForgeModules.m[moduleName])
				rt.LatestForgeModules.RUnlock()
			}
			return
		}
		// safe to do, because we ensured in doModuleInstallOrNothing() that -latest exists
		m.version = "latest"

	}
	if isDir(targetDir) {
		if fileExists(metadataFile) {
			me := rt.readModuleMetadata(metadataFile)
			if m.version == "latest" {
				//fmt.Println(latestForgeModules)
				//fmt.Println("checking latestForgeModules for key", moduleName)
				rt.LatestForgeModules.RLock()
				if _, ok := rt.LatestForgeModules.m[moduleName]; ok {
					rt.Debugf("using version " + rt.LatestForgeModules.m[moduleName] + " for " + moduleName + "-" + m.version)
					m.version = rt.LatestForgeModules.m[moduleName]
				}
				rt.LatestForgeModules.RUnlock()
			}
			if rt.Check4Update {
				rt.LatestForgeModules.RLock()
				rt.check4ForgeUpdate(m.name, me.version, rt.LatestForgeModules.m[moduleName])
				rt.LatestForgeModules.RUnlock()
			}
			if me.version == m.version {
				rt.Debugf("Nothing to do, existing Forge module: " + targetDir + " has the same version " + me.version + " as the to be synced version: " + m.version)
				return
			}
			rt.Infof("Need to sync, because existing Forge module: " + targetDir + " has version " + me.version + " and the to be synced version is: " + m.version)
			rt.createOrPurgeDir(targetDir, "targetDir for module "+me.name)
		} else {
			rt.Debugf("Need to purge " + targetDir + ", because it exists without a metadata.json. This shouldn't happen!")
			rt.createOrPurgeDir(targetDir, "targetDir for module "+m.name+" with missing metadata.json")
		}
	}
	workDir := normalizeDir(filepath.Join(rt.Config.ForgeCacheDir, moduleName+"-"+m.version))
	resolvedWorkDir, err := filepath.EvalSymlinks(workDir)
	if err != nil {
		rt.Fatalf(funcName + "(): Failed to resolve possible symlink " + workDir + " Error: " + err.Error())
	}
	if !isDir(resolvedWorkDir) {
		if rt.Config.UseCacheFallback {
			rt.Warnf("Failed to use " + resolvedWorkDir + " Trying to use latest cached version of module " + moduleName)
			resolvedWorkDir = rt.getLatestCachedModule(m)
		} else {
			rt.Fatalf(funcName + "(): Forge module not found in dir: " + resolvedWorkDir)
		}
	}

	if !isDir(resolvedWorkDir) {
		rt.Fatalf(funcName + "(): Forge module not found in dir: " + resolvedWorkDir)
	}

	rt.Infof("Need to sync " + targetDir)
	if !rt.DryRun {
		targetDir = rt.checkDirAndCreate(targetDir, "as targetDir for module "+name)
		var targetDirDevice, workDirDevice uint64
		if fileInfo, err := os.Stat(targetDir); err == nil {
			if fileInfo.Sys() != nil {
				targetDirDevice = uint64(fileInfo.Sys().(*syscall.Stat_t).Dev)
			}
		} else {
			rt.Fatalf(funcName + "(): Error while os.Stat file " + targetDir)
		}
		if fileInfo, err := os.Stat(resolvedWorkDir); err == nil {
			if fileInfo.Sys() != nil {
				workDirDevice = uint64(fileInfo.Sys().(*syscall.Stat_t).Dev)
			}
		} else {
			rt.Fatalf(funcName + "(): Error while os.Stat file " + resolvedWorkDir)
		}

		if targetDirDevice != workDirDevice && !rt.UseMove {
			rt.Fatalf("Error: Can't hardlink Forge module files over different devices. Please consider changing the cachedir setting. ForgeCachedir: " + rt.Config.ForgeCacheDir + " target dir: " + targetDir)
		}

		rt.Mutex.Lock()
		rt.NeedSyncDirs = append(rt.NeedSyncDirs, targetDir)
		if _, ok := rt.NeedSyncEnvs[correspondingPuppetEnvironment]; !ok {
			rt.NeedSyncEnvs[correspondingPuppetEnvironment] = struct{}{}
		}
		rt.NeedSyncForgeCount++
		rt.Mutex.Unlock()
		destination := func(path string, info os.FileInfo, err error) error {
			if err != nil {
				rt.Fatalf(funcName + "(): Error while calling generic func() Error " + err.Error())
			}
			target, err := filepath.Rel(resolvedWorkDir, path)
			if err != nil {
				rt.Fatalf(funcName + "(): Can't make " + path + " relative to " + resolvedWorkDir + " Error: " + err.Error())
			}

			if info.IsDir() {
				if target != "." { // skip the root dir
					//Debugf(funcName + "() Trying to mkdir " + filepath.Join(targetDir, target))
					err = os.Mkdir(filepath.Join(targetDir, target), os.FileMode(0755))
					if err != nil {
						rt.Fatalf(funcName + "(): error while Mkdir() " + targetDir + "/" + target + " Error: " + err.Error())
					}
				}
			} else {
				if rt.UseMove {
					//Debugf(funcName + "() Trying to helper.moveFile " + path + " to " + targetDir + target)
					// deleteSourceFileToggle is set to false as we delete the source file later in the main() anyway after the sync completes
					err = rt.moveFile(path, filepath.Join(targetDir, target), false)
					if err != nil {
						rt.Fatalf(funcName + "(): Failed to helper.moveFile " + path + " to " + targetDir + "/" + target + " Error: " + err.Error())
					}
				} else {
					//Debugf(funcName + "() Trying to hardlink " + path + " to " + filepath.Join(targetDir, target))
					err = os.Link(path, filepath.Join(targetDir, target))
					if err != nil {
						rt.Fatalf(funcName + "(): Failed to hardlink " + path + " to " + targetDir + "/" + target + " Error: " + err.Error())
					}
				}
			}
			return nil
		}

		c := make(chan error)
		rt.Debugf(funcName + "() filepath.Walk'ing directory " + resolvedWorkDir)
		before := time.Now()
		go func() { c <- filepath.Walk(resolvedWorkDir, destination) }()
		<-c // Walk done
		duration := time.Since(before).Seconds()
		rt.Mutex.Lock()
		rt.IoForgeTime += duration
		rt.Mutex.Unlock()
		rt.Verbosef("Populating " + targetDir + " took " + strconv.FormatFloat(duration, 'f', 5, 64) + "s")
	}
}

// getLatestCachedModule returns the most recent version of the module that is requested
func (rt *Runtime) getLatestCachedModule(m ForgeModule) string {
	latest := "//"
	version := "latest"
	latestDir := filepath.Join(rt.Config.ForgeCacheDir, m.author+"-"+m.name+"-"+version)
	if !isDir(latestDir) {

		globPath := filepath.Join(rt.Config.ForgeCacheDir, m.author+"-"+m.name+"-*")
		rt.Debugf("Glob'ing with path " + globPath)
		matches, err := filepath.Glob(globPath)
		if len(matches) == 0 {
			rt.Fatalf("Could not find any cached version for Forge module " + m.author + "-" + m.name)
		}
		rt.Debugf("found potential module versions:" + strings.Join(matches, " "))
		if err != nil {
			rt.Fatalf("Failed to glob the latest cached module with glob path " + globPath + " Error: " + err.Error())
		}
		//fmt.Println(matches)
		for _, m := range matches {
			if isDir(m) {
				rt.Debugf("Comparing " + latest + " < " + m)
				if latest < m {
					rt.Debugf("Setting latest to " + m)
					latest = m
				}
			}
		}

		absolutePath, err := filepath.Abs(latest)
		if err != nil {
			rt.Fatalf("Error while resolving absolute file path for " + latest + " Error: " + err.Error())
		}
		rt.Debugf("trying to create symlink " + latestDir + " pointing to " + latest)
		if err := rt.createSymlinkIdempotent(absolutePath, latestDir); err != nil {
			rt.Fatalf("Error while creating symlink " + latestDir + " pointing to " + absolutePath + err.Error())
		}
		version = strings.Split(latest, m.author+"-"+m.name+"-")[1]
	} else {
		versionDir, err := os.Readlink(latestDir)
		if err != nil {
			rt.Fatalf("Error while reading symlink " + latestDir + " " + err.Error())
		}

		version = strings.Split(versionDir, m.author+"-"+m.name+"-")[1]
		latest = filepath.Join(rt.Config.ForgeCacheDir, m.author+"-"+m.name+"-"+version)
	}

	if latest == "//" {
		rt.Fatalf("Found no usable cache for module " + m.author + "-" + m.name)
	}

	//fmt.Println("version: ", version)
	rt.LatestForgeModules.Lock()
	rt.LatestForgeModules.m[m.author+"-"+m.name] = version
	rt.LatestForgeModules.Unlock()

	rt.Warnf("Using cached version " + version + " for " + m.author + "-" + m.name + "-latest")

	return latest
}
