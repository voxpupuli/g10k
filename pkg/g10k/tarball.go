package g10k

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

func (m TarballModule) downloadURL() string {
	if m.githubRepo == "" {
		return m.source
	}
	source := "https://api.github.com/repos/" + m.githubRepo + "/tarball"
	if m.version != "" {
		source += "/" + url.PathEscape(m.version)
	}
	return source
}

func (m TarballModule) cacheKey() string {
	kind := "direct"
	if m.githubRepo != "" {
		kind = "github"
	}
	return kind + "\x00" + m.downloadURL() + "\x00" + strings.ToLower(m.checksum)
}

func (rt *Runtime) resolveTarballModules(puppetfiles map[string]Puppetfile) error {
	client := &http.Client{Timeout: time.Duration(rt.Config.Timeout) * time.Second}
	if rt.tarballClient != nil {
		client = rt.tarballClient
	}
	type archiveJob struct {
		module  TarballModule
		archive string
		cleanup func()
	}
	type deploymentJob struct {
		module  TarballModule
		workDir string
		env     string
		cache   int
	}
	var archives []archiveJob
	var deployments []deploymentJob
	keys := make(map[string]int)
	for env, pf := range puppetfiles {
		for name, module := range pf.tarballModules {
			if rt.Module != "" && rt.Module != name {
				continue
			}
			key := module.cacheKey()
			index, ok := keys[key]
			if !ok {
				index = len(archives)
				keys[key] = index
				archives = append(archives, archiveJob{module: module})
			}
			deployments = append(deployments, deploymentJob{module: module, workDir: pf.workDir, env: env, cache: index})
		}
	}
	if len(deployments) == 0 {
		return nil
	}
	if rt.Config.Maxworker < 1 || rt.Config.MaxExtractworker < 1 {
		return fmt.Errorf("tarball worker limits must be positive")
	}
	validationSem := make(chan struct{}, rt.Config.MaxExtractworker)
	errs := parallelTarballJobs(len(archives), rt.Config.Maxworker, func(i int) error {
		job := &archives[i]
		archive, cleanup, err := rt.tarballCache(client, job.module, validationSem)
		if err != nil {
			return fmt.Errorf("tarball module %s: %w", job.module.name, err)
		}
		job.archive, job.cleanup = archive, cleanup
		return nil
	})
	for _, job := range archives {
		if job.cleanup != nil {
			defer job.cleanup()
		}
	}
	if err := errors.Join(errs...); err != nil {
		return err
	}
	errs = parallelTarballJobs(len(deployments), rt.Config.MaxExtractworker, func(i int) error {
		job := deployments[i]
		if err := rt.syncTarballModule(archives[job.cache].archive, job.module, job.workDir, job.env); err != nil {
			return fmt.Errorf("tarball module %s in environment %s: %w", job.module.name, job.env, err)
		}
		rt.Mutex.Lock()
		rt.SyncTarballCount++
		rt.Mutex.Unlock()
		return nil
	})
	return errors.Join(errs...)
}

func parallelTarballJobs(count, limit int, run func(int) error) []error {
	var wg sync.WaitGroup
	sem := make(chan struct{}, limit)
	errs := make([]error, count)
	for i := 0; i < count; i++ {
		sem <- struct{}{}
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			defer func() { <-sem }()
			errs[i] = run(i)
		}(i)
	}
	wg.Wait()
	return errs
}

func (rt *Runtime) syncTarballModule(archive string, module TarballModule, workDir, env string) error {
	tempBase := ""
	targetDir := filepath.Join(workDir, module.moduleDir, module.name)
	if !rt.DryRun {
		tempBase = filepath.Dir(targetDir)
		if err := os.MkdirAll(tempBase, 0o755); err != nil {
			return fmt.Errorf("create module directory: %w", err)
		}
	}
	stage, err := os.MkdirTemp(tempBase, ".g10k-tarball-")
	if err != nil {
		return fmt.Errorf("create extraction directory: %w", err)
	}
	defer rt.cleanupTarballTemp(stage)
	if err := os.Chmod(stage, 0o755); err != nil {
		return fmt.Errorf("set extraction directory permissions: %w", err)
	}
	if err := rt.extractTarball(archive, stage, module.githubRepo != ""); err != nil {
		return fmt.Errorf("extract archive: %w", err)
	}
	expected, err := SnapshotDirectory(stage)
	if err != nil {
		return fmt.Errorf("inspect extracted archive: %w", err)
	}
	actual, err := SnapshotDirectory(targetDir)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect installed module: %w", err)
	}
	if err == nil && CompareSnapshots(expected, actual).IsIdentical() {
		return nil
	}
	rt.Infof("Need to sync " + targetDir)
	rt.Mutex.Lock()
	rt.NeedSyncTarballCount++
	rt.NeedSyncDirs = append(rt.NeedSyncDirs, targetDir)
	rt.NeedSyncEnvs[env] = struct{}{}
	rt.Mutex.Unlock()
	if rt.DryRun {
		return nil
	}
	if err := replaceTarballDirectory(stage, targetDir); err != nil {
		return fmt.Errorf("install module: %w", err)
	}
	return nil
}

func (rt *Runtime) cleanupTarballTemp(name string) {
	if err := os.RemoveAll(name); err != nil {
		rt.Warnf("Could not remove temporary tarball path " + name + ": " + err.Error())
	}
}

func (rt *Runtime) tarballCache(client *http.Client, module TarballModule, validationSem chan struct{}) (string, func(), error) {
	validate := func(archive string) error {
		validationSem <- struct{}{}
		defer func() { <-validationSem }()
		return rt.validateTarballArchive(archive, module.githubRepo != "")
	}
	source := module.downloadURL()
	key := sha256.Sum256([]byte(module.cacheKey()))
	cacheDir := filepath.Join(rt.Config.CacheDir, "tarball")
	cacheFile := filepath.Join(cacheDir, hex.EncodeToString(key[:])+".tar.gz")
	if f, err := os.Open(cacheFile); err == nil {
		ok, checkErr := verifyTarballChecksum(f, module.checksum)
		closeErr := f.Close()
		if checkErr != nil {
			return "", nil, fmt.Errorf("read cached tarball: %w", checkErr)
		}
		if closeErr != nil {
			return "", nil, fmt.Errorf("close cached tarball: %w", closeErr)
		}
		if ok {
			if err := validate(cacheFile); err == nil {
				return cacheFile, func() {}, nil
			} else {
				rt.Warnf("Invalid cached tarball for module " + module.name + ": " + err.Error() + "; downloading again")
			}
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", nil, fmt.Errorf("open cached tarball: %w", err)
	}

	tempDir := cacheDir
	if rt.DryRun {
		tempDir = ""
	} else if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return "", nil, fmt.Errorf("create tarball cache: %w", err)
	}
	temp, err := os.CreateTemp(tempDir, ".g10k-download-")
	if err != nil {
		return "", nil, fmt.Errorf("create temporary download: %w", err)
	}
	tempName := temp.Name()
	cleanup := func() { rt.cleanupTarballTemp(tempName) }
	if err := downloadTarball(client, source, temp); err != nil {
		_ = temp.Close()
		cleanup()
		return "", nil, err
	}
	if _, err := temp.Seek(0, io.SeekStart); err != nil {
		_ = temp.Close()
		cleanup()
		return "", nil, fmt.Errorf("seek downloaded tarball: %w", err)
	}
	ok, err := verifyTarballChecksum(temp, module.checksum)
	if err != nil || !ok {
		_ = temp.Close()
		cleanup()
		if err != nil {
			return "", nil, fmt.Errorf("verify downloaded tarball: %w", err)
		}
		return "", nil, fmt.Errorf("downloaded tarball SHA-256 does not match expected digest")
	}
	if err := temp.Close(); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("close downloaded tarball: %w", err)
	}
	if err := validate(tempName); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("validate downloaded tarball: %w", err)
	}
	if rt.DryRun {
		return tempName, cleanup, nil
	}
	if err := os.Rename(tempName, cacheFile); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("publish tarball cache: %w", err)
	}
	return cacheFile, func() {}, nil
}

func (rt *Runtime) validateTarballArchive(archive string, stripRoot bool) error {
	validationDir, err := os.MkdirTemp("", ".g10k-tarball-validation-")
	if err != nil {
		return fmt.Errorf("create archive validation directory: %w", err)
	}
	defer rt.cleanupTarballTemp(validationDir)
	return rt.extractTarball(archive, validationDir, stripRoot)
}

func verifyTarballChecksum(r io.Reader, expected string) (bool, error) {
	if expected == "" {
		return true, nil
	}
	h := sha256.New()
	if _, err := io.Copy(h, r); err != nil {
		return false, err
	}
	return strings.EqualFold(hex.EncodeToString(h.Sum(nil)), expected), nil
}

func downloadTarball(client *http.Client, source string, target io.Writer) (resultErr error) {
	u, err := url.Parse(source)
	if err != nil {
		return fmt.Errorf("invalid tarball source: %w", err)
	}
	switch u.Scheme {
	case "", "file":
		if u.Scheme == "file" && u.Host != "" && u.Host != "localhost" {
			return fmt.Errorf("unsupported file URL host %q", u.Host)
		}
		f, err := os.Open(u.Path)
		if err != nil {
			return fmt.Errorf("open tarball source: %w", err)
		}
		_, copyErr := io.Copy(target, f)
		closeErr := f.Close()
		if copyErr != nil {
			return fmt.Errorf("read tarball source: %w", copyErr)
		}
		if closeErr != nil {
			return fmt.Errorf("close tarball source: %w", closeErr)
		}
		return nil
	case "http", "https":
		req, err := http.NewRequest(http.MethodGet, source, nil)
		if err != nil {
			return fmt.Errorf("create tarball request: %w", err)
		}
		req.Header.Set("User-Agent", "https://github.com/voxpupuli/g10k")
		resp, err := client.Do(req)
		if err != nil {
			return fmt.Errorf("download tarball: %w", err)
		}
		defer joinTarballCloseError(&resultErr, resp.Body, "close tarball response")
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("download tarball: HTTP %s", resp.Status)
		}
		if _, err := io.Copy(target, resp.Body); err != nil {
			return fmt.Errorf("read tarball response: %w", err)
		}
		return nil
	default:
		return fmt.Errorf("unsupported tarball source scheme %q", u.Scheme)
	}
}

func joinTarballCloseError(resultErr *error, closer io.Closer, context string) {
	if err := closer.Close(); err != nil {
		*resultErr = errors.Join(*resultErr, fmt.Errorf("%s: %w", context, err))
	}
}

func replaceTarballDirectory(stage, target string) error {
	_, err := os.Lstat(target)
	if errors.Is(err, os.ErrNotExist) {
		return os.Rename(stage, target)
	}
	if err != nil {
		return err
	}
	backup, err := os.MkdirTemp(filepath.Dir(target), ".g10k-backup-")
	if err != nil {
		return err
	}
	if err := os.Remove(backup); err != nil {
		return err
	}
	if err := os.Rename(target, backup); err != nil {
		return err
	}
	if err := os.Rename(stage, target); err != nil {
		if restoreErr := os.Rename(backup, target); restoreErr != nil {
			return fmt.Errorf("replace: %w; restore backup %s: %v", err, backup, restoreErr)
		}
		return err
	}
	if err := os.RemoveAll(backup); err != nil {
		return fmt.Errorf("remove backup %s: %w", backup, err)
	}
	return nil
}
