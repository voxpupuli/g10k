package g10k

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Report states of a module, see ReportModule.State.
const (
	ReportStateDeployed = "deployed"
	ReportStateOutdated = "outdated"
	ReportStateMissing  = "missing"
	ReportStateLocal    = "local"
)

// Report module types, see ReportModule.Type.
const (
	ReportTypeForge   = "forge"
	ReportTypeGit     = "git"
	ReportTypeTarball = "tarball"
)

// Report describes what is deployed on disk after a g10k run.
// It is written as JSON by the -report option.
type Report struct {
	GeneratedAt  time.Time           `json:"generated_at"`
	G10kVersion  string              `json:"g10k_version,omitempty"`
	Target       string              `json:"target"`
	DryRun       bool                `json:"dry_run"`
	Environments []ReportEnvironment `json:"environments"`
}

// ReportEnvironment is a deployed Puppet environment (or the Puppetfile of -puppetfile mode)
// with the modules declared in its Puppetfile.
type ReportEnvironment struct {
	Name               string         `json:"name"`
	Path               string         `json:"path"`
	Source             string         `json:"source,omitempty"`
	Branch             string         `json:"branch,omitempty"`
	GitURL             string         `json:"git_url,omitempty"`
	Commit             string         `json:"commit,omitempty"`
	DeploySuccess      *bool          `json:"deploy_success,omitempty"`
	DeployedAt         *time.Time     `json:"deployed_at,omitempty"`
	PuppetfileChecksum string         `json:"puppetfile_checksum,omitempty"`
	Modules            []ReportModule `json:"modules"`
}

// ReportModule is one module declared in a Puppetfile and what was found on disk.
// Fields not relevant for the module Type are omitted.
type ReportModule struct {
	Name  string `json:"name"`
	Type  string `json:"type"`
	Path  string `json:"path"`
	State string `json:"state"`
	// forge
	Author          string `json:"author,omitempty"`
	DeclaredVersion string `json:"declared_version,omitempty"`
	DeployedVersion string `json:"deployed_version,omitempty"`
	LatestVersion   string `json:"latest_version,omitempty"`
	// git
	URL string `json:"url,omitempty"`
	// Ref is the branch, tag or commit g10k really deployed. With :control_branch and :default_branch
	// this is the default branch when the control branch does not exist in the module repository.
	Ref    string `json:"ref,omitempty"`
	Commit string `json:"commit,omitempty"`
	// tarball
	Source     string `json:"source,omitempty"`
	GitHubRepo string `json:"github_repo,omitempty"`
	Version    string `json:"version,omitempty"`
	Checksum   string `json:"checksum,omitempty"`
}

// collectReport inspects the deployed state of all given environments and stores it in rt.
// Environments without a Puppetfile are reported with an empty module list.
// It does nothing unless the -report option is set.
func (rt *Runtime) collectReport(environments ...map[string]Puppetfile) {
	if rt.Report == "" {
		return
	}
	var envs []ReportEnvironment
	for _, m := range environments {
		for env, pf := range m {
			envs = append(envs, rt.buildReportEnvironment(env, pf))
		}
	}
	sort.Slice(envs, func(i, j int) bool { return envs[i].Name < envs[j].Name })
	rt.Mutex.Lock()
	rt.reportEnvironments = envs
	rt.Mutex.Unlock()
}

func (rt *Runtime) buildReportEnvironment(env string, pf Puppetfile) ReportEnvironment {
	re := ReportEnvironment{
		Name:    env,
		Path:    absPath(pf.workDir),
		Source:  pf.source,
		Branch:  pf.controlRepoBranch,
		GitURL:  redactURL(pf.gitURL),
		Modules: []ReportModule{},
	}
	deployFile := filepath.Join(pf.workDir, ".g10k-deploy.json")
	if fileExists(deployFile) {
		dr := rt.readDeployResultFile(deployFile)
		re.Commit = dr.Signature
		re.DeploySuccess = &dr.DeploySuccess
		if !dr.FinishedAt.IsZero() {
			finishedAt := dr.FinishedAt.UTC()
			re.DeployedAt = &finishedAt
		}
		re.PuppetfileChecksum = dr.PuppetfileChecksum
	}

	// in -puppetfile mode g10k works relative to the current directory
	basedir := ""
	if !rt.PFMode {
		basedir = normalizeDir(pf.workDir)
	}
	for name, gm := range pf.gitModules {
		if rt.Module != "" && rt.Module != name {
			continue
		}
		re.Modules = append(re.Modules, rt.buildReportGitModule(name, gm, pf, basedir))
	}
	for name, fm := range pf.forgeModules {
		if rt.Module != "" && rt.Module != name {
			continue
		}
		re.Modules = append(re.Modules, rt.buildReportForgeModule(fm, pf.workDir))
	}
	for name, tm := range pf.tarballModules {
		if rt.Module != "" && rt.Module != name {
			continue
		}
		re.Modules = append(re.Modules, buildReportTarballModule(name, tm, pf.workDir))
	}
	sort.Slice(re.Modules, func(i, j int) bool {
		if re.Modules[i].Type != re.Modules[j].Type {
			return re.Modules[i].Type < re.Modules[j].Type
		}
		return re.Modules[i].Name < re.Modules[j].Name
	})
	return re
}

func (rt *Runtime) buildReportGitModule(name string, gm GitModule, pf Puppetfile, basedir string) ReportModule {
	// same target directory as in resolvePuppetfile()
	targetDir := normalizeDir(filepath.Join(normalizeDir(filepath.Join(pf.workDir, gm.moduleDir)), name))
	if len(gm.installPath) > 0 {
		targetDir = normalizeDir(filepath.Join(basedir, normalizeDir(gm.installPath), name))
	}
	rm := ReportModule{Name: name, Type: ReportTypeGit, Path: absPath(targetDir), URL: redactURL(gm.git)}
	switch {
	case gm.local:
		rm.State = ReportStateLocal
		if !isDir(targetDir) {
			rm.State = ReportStateMissing
		}
	case !isDir(targetDir):
		rm.State = ReportStateMissing
	default:
		rm.State = ReportStateDeployed
		rm.Ref = rt.resolvedGitRef(targetDir)
		// .latest_commit is not written when the module is a git clone (clone_git_modules)
		if content, err := os.ReadFile(filepath.Join(targetDir, ".latest_commit")); err == nil {
			rm.Commit = strings.TrimSpace(string(content))
		}
	}
	return rm
}

// recordResolvedGitRef remembers which branch, tag or commit was really deployed into targetDir.
// It does nothing unless the -report option is set.
func (rt *Runtime) recordResolvedGitRef(targetDir string, ref string) {
	if rt.Report == "" {
		return
	}
	rt.Mutex.Lock()
	defer rt.Mutex.Unlock()
	if rt.resolvedGitRefs == nil {
		rt.resolvedGitRefs = make(map[string]string)
	}
	rt.resolvedGitRefs[absPath(targetDir)] = ref
}

func (rt *Runtime) resolvedGitRef(targetDir string) string {
	rt.Mutex.Lock()
	defer rt.Mutex.Unlock()
	return rt.resolvedGitRefs[absPath(targetDir)]
}

func (rt *Runtime) buildReportForgeModule(fm ForgeModule, workDir string) ReportModule {
	targetDir := normalizeDir(filepath.Join(normalizeDir(filepath.Join(workDir, fm.moduleDir)), fm.name))
	rm := ReportModule{
		Name:            fm.name,
		Type:            ReportTypeForge,
		Path:            absPath(targetDir),
		Author:          fm.author,
		DeclaredVersion: fm.version,
	}
	rt.LatestForgeModules.RLock()
	rm.LatestVersion = rt.LatestForgeModules.m[fm.author+"-"+fm.name]
	rt.LatestForgeModules.RUnlock()

	metadataFile := filepath.Join(targetDir, "metadata.json")
	if !fileExists(metadataFile) {
		rm.State = ReportStateMissing
		return rm
	}
	rm.DeployedVersion = rt.readModuleMetadata(metadataFile).version
	rm.State = ReportStateDeployed
	// version the module is expected to have: the pinned one, or the latest known one for "latest".
	// "present" accepts any installed version.
	expected := fm.version
	switch expected {
	case "present":
		expected = ""
	case "latest":
		expected = rm.LatestVersion
	}
	if expected != "" && expected != rm.DeployedVersion {
		rm.State = ReportStateOutdated
	}
	return rm
}

func buildReportTarballModule(name string, tm TarballModule, workDir string) ReportModule {
	targetDir := filepath.Join(workDir, tm.moduleDir, name)
	rm := ReportModule{
		Name:       name,
		Type:       ReportTypeTarball,
		Path:       absPath(targetDir),
		Source:     redactURL(tm.downloadURL()),
		GitHubRepo: tm.githubRepo,
		Version:    tm.version,
		Checksum:   tm.checksum,
		State:      ReportStateDeployed,
	}
	if !isDir(targetDir) {
		rm.State = ReportStateMissing
	}
	return rm
}

// redactURL removes credentials (password of the user info and query string) from a URL,
// so the report can be shared without leaking secrets. Anything that is not a URL with a
// scheme, like local paths or scp-like git remotes (git@host:path), is returned unchanged.
func redactURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return raw
	}
	if _, hasPassword := u.User.Password(); hasPassword {
		u.User = url.UserPassword(u.User.Username(), "REDACTED")
	}
	if u.RawQuery != "" {
		u.RawQuery = "REDACTED"
	}
	return u.String()
}

// absPath returns the absolute form of path, or path itself if that fails.
func absPath(path string) string {
	if abs, err := filepath.Abs(path); err == nil {
		return abs
	}
	return path
}

// checkReportPath fails early, before any work is done, if the report can not be written.
func checkReportPath(report string) error {
	if report == "" {
		return nil
	}
	if fi, err := os.Stat(filepath.Dir(report)); err != nil || !fi.IsDir() {
		return fmt.Errorf("error: directory of -report file %s does not exist", report)
	}
	if fi, err := os.Stat(report); err == nil && fi.IsDir() {
		return fmt.Errorf("error: -report file %s is a directory", report)
	}
	return nil
}

// writeReport writes the report collected by collectReport() as JSON to rt.Report.
func (rt *Runtime) writeReport(target string) error {
	rt.Mutex.Lock()
	envs := rt.reportEnvironments
	rt.Mutex.Unlock()
	if envs == nil {
		envs = []ReportEnvironment{}
	}
	report := Report{
		GeneratedAt:  time.Now().UTC(),
		G10kVersion:  rt.BuildVersion,
		Target:       target,
		DryRun:       rt.DryRun,
		Environments: envs,
	}
	content, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("could not encode report: %w", err)
	}
	content = append(content, '\n')
	// write to a temporary file first, so a failed run never leaves a truncated report
	tmp, err := os.CreateTemp(filepath.Dir(rt.Report), ".g10k-report-")
	if err != nil {
		return fmt.Errorf("could not write report %s: %w", rt.Report, err)
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	if _, err = tmp.Write(content); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("could not write report %s: %w", rt.Report, err)
	}
	if err = tmp.Close(); err != nil {
		return fmt.Errorf("could not write report %s: %w", rt.Report, err)
	}
	if err = os.Chmod(tmp.Name(), 0o644); err != nil {
		return fmt.Errorf("could not write report %s: %w", rt.Report, err)
	}
	if err = os.Rename(tmp.Name(), rt.Report); err != nil {
		return fmt.Errorf("could not write report %s: %w", rt.Report, err)
	}
	return nil
}
