package g10k

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func readTestReport(t *testing.T, path string) Report {
	t.Helper()
	content, err := os.ReadFile(path)
	require.NoError(t, err)
	var report Report
	require.NoError(t, json.Unmarshal(content, &report))
	return report
}

func findReportModule(t *testing.T, env ReportEnvironment, name string) ReportModule {
	t.Helper()
	for _, m := range env.Modules {
		if m.Name == name {
			return m
		}
	}
	require.FailNow(t, "module not found in report", name)
	return ReportModule{}
}

func writeTestForgeModule(t *testing.T, dir, author, name, version string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(dir, 0o755))
	metadata := fmt.Sprintf(`{"name": "%s-%s", "version": "%s", "author": "%s"}`, author, name, version, author)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "metadata.json"), []byte(metadata), 0o644))
}

// TestReportPuppetfileMode runs g10k for real in -puppetfile mode with a local git repository
// and a tarball served over HTTP and checks the resulting report.
func TestReportPuppetfileMode(t *testing.T) {
	archive := testTarball(t, archiveEntry{name: "file", content: "ok"})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(archive)
	}))
	defer server.Close()

	env := t.TempDir()
	t.Chdir(env)
	t.Setenv("g10k_cachedir", filepath.Join(env, "cache"))

	repo := filepath.Join(env, "repo")
	require.NoError(t, os.MkdirAll(repo, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(repo, "init.pp"), []byte("class mygit {}\n"), 0o644))
	gitCmd := func(args ...string) string {
		cmd := exec.Command("git", append([]string{"-C", repo, "-c", "user.name=t", "-c", "user.email=t@example.com", "-c", "init.defaultBranch=main"}, args...)...)
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, string(out))
		return string(out)
	}
	gitCmd("init")
	gitCmd("add", ".")
	gitCmd("commit", "-m", "init")
	commit := gitCmd("rev-parse", "HEAD")
	commit = commit[:len(commit)-1]

	pfPath := filepath.Join(env, "Puppetfile")
	pf := fmt.Sprintf("mod 'mygit', :git => '%s', :branch => 'main'\nmod 'apache', type: 'tarball', source: '%s/apache'\n", repo, server.URL)
	require.NoError(t, os.WriteFile(pfPath, []byte(pf), 0o600))

	reportPath := filepath.Join(env, "report.json")
	code, err := Run(Options{Quiet: true, PFMode: true, PFLocation: pfPath, ModuleDir: filepath.Join(env, "modules"), MaxWorker: 2, MaxExtractWorker: 2, Report: reportPath, BuildVersion: "test"})
	require.NoError(t, err)
	require.Equal(t, 0, code)

	report := readTestReport(t, reportPath)
	require.Equal(t, "test", report.G10kVersion)
	require.Equal(t, pfPath, report.Target)
	require.False(t, report.DryRun)
	require.Len(t, report.Environments, 1)
	re := report.Environments[0]
	require.Len(t, re.Modules, 2)

	git := findReportModule(t, re, "mygit")
	require.Equal(t, ReportTypeGit, git.Type)
	require.Equal(t, ReportStateDeployed, git.State)
	require.Equal(t, commit, git.Commit)
	require.Equal(t, "main", git.Ref)
	require.Equal(t, repo, git.URL)
	require.Equal(t, filepath.Join(env, "modules", "mygit"), git.Path)

	tb := findReportModule(t, re, "apache")
	require.Equal(t, ReportTypeTarball, tb.Type)
	require.Equal(t, ReportStateDeployed, tb.State)
	require.Equal(t, server.URL+"/apache", tb.Source)

	// a module removed from disk after the run must show up as missing
	require.NoError(t, os.RemoveAll(filepath.Join(env, "modules", "mygit")))
	rt := NewRuntime(Options{Quiet: true, PFMode: true, Report: reportPath})
	rt.collectReport(map[string]Puppetfile{"cmdlineparam": {
		gitModules: map[string]GitModule{"mygit": {git: repo, branch: "main", moduleDir: filepath.Join(env, "modules")}},
	}})
	require.Equal(t, ReportStateMissing, rt.reportEnvironments[0].Modules[0].State)
}

func TestReportForgeModuleStates(t *testing.T) {
	dir := t.TempDir()
	rt := NewRuntime(Options{Report: "x"})
	rt.LatestForgeModules.m["puppetlabs-stdlib"] = "9.0.0"
	rt.LatestForgeModules.m["puppetlabs-apt"] = "8.0.0"

	writeTestForgeModule(t, filepath.Join(dir, "modules", "stdlib"), "puppetlabs", "stdlib", "8.5.0")
	writeTestForgeModule(t, filepath.Join(dir, "modules", "apt"), "puppetlabs", "apt", "8.0.0")
	writeTestForgeModule(t, filepath.Join(dir, "modules", "ntp"), "puppetlabs", "ntp", "6.0.0")
	writeTestForgeModule(t, filepath.Join(dir, "modules", "concat"), "puppetlabs", "concat", "7.0.0")

	tests := []struct {
		name     string
		fm       ForgeModule
		state    string
		deployed string
		latest   string
	}{
		{"pinned version differs", ForgeModule{name: "stdlib", author: "puppetlabs", version: "9.0.0"}, ReportStateOutdated, "8.5.0", "9.0.0"},
		{"latest, older deployed", ForgeModule{name: "stdlib", author: "puppetlabs", version: "latest"}, ReportStateOutdated, "8.5.0", "9.0.0"},
		{"latest, up to date", ForgeModule{name: "apt", author: "puppetlabs", version: "latest"}, ReportStateDeployed, "8.0.0", "8.0.0"},
		{"pinned, matching", ForgeModule{name: "ntp", author: "puppetlabs", version: "6.0.0"}, ReportStateDeployed, "6.0.0", ""},
		{"present accepts any version", ForgeModule{name: "concat", author: "puppetlabs", version: "present"}, ReportStateDeployed, "7.0.0", ""},
		{"present ignores a newer latest", ForgeModule{name: "stdlib", author: "puppetlabs", version: "present"}, ReportStateDeployed, "8.5.0", "9.0.0"},
		{"not installed", ForgeModule{name: "firewall", author: "puppetlabs", version: "1.0.0"}, ReportStateMissing, "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.fm.moduleDir = "modules"
			rm := rt.buildReportForgeModule(tt.fm, dir)
			require.Equal(t, tt.state, rm.State)
			require.Equal(t, tt.deployed, rm.DeployedVersion)
			require.Equal(t, tt.latest, rm.LatestVersion)
			require.Equal(t, ReportTypeForge, rm.Type)
			require.Equal(t, "puppetlabs", rm.Author)
			require.Equal(t, tt.fm.version, rm.DeclaredVersion)
		})
	}
}

func TestReportGitModuleStates(t *testing.T) {
	dir := t.TempDir()
	rt := NewRuntime(Options{Report: "x"})
	pf := Puppetfile{workDir: dir, controlRepoBranch: "production"}

	deployed := filepath.Join(dir, "modules", "deployed")
	require.NoError(t, os.MkdirAll(deployed, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(deployed, ".latest_commit"), []byte("abc123"), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "modules", "localmod"), 0o755))

	rt.recordResolvedGitRef(deployed, "v1")
	rm := rt.buildReportGitModule("deployed", GitModule{git: "https://example.com/d.git", tag: "v1", moduleDir: "modules"}, pf, dir)
	require.Equal(t, ReportStateDeployed, rm.State)
	require.Equal(t, "abc123", rm.Commit)
	require.Equal(t, "v1", rm.Ref)

	rm = rt.buildReportGitModule("gone", GitModule{git: "https://example.com/g.git", moduleDir: "modules"}, pf, dir)
	require.Equal(t, ReportStateMissing, rm.State)
	require.Empty(t, rm.Commit)

	rm = rt.buildReportGitModule("localmod", GitModule{local: true, moduleDir: "modules"}, pf, dir)
	require.Equal(t, ReportStateLocal, rm.State)

}

func TestReportEnvironmentReadsDeployFile(t *testing.T) {
	dir := t.TempDir()
	rt := NewRuntime(Options{Report: "x"})
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".g10k-deploy.json"),
		[]byte(`{"name":"production","signature":"deadbeef","finished_at":"2024-01-02T03:04:05Z","deploy_success":true,"puppetfile_checksum":"sum"}`), 0o644))

	re := rt.buildReportEnvironment("prod", Puppetfile{workDir: dir, source: "main", controlRepoBranch: "production", gitURL: "git@example.com:c.git"})
	require.Equal(t, "deadbeef", re.Commit)
	require.NotNil(t, re.DeploySuccess)
	require.True(t, *re.DeploySuccess)
	require.NotNil(t, re.DeployedAt)
	require.Equal(t, "sum", re.PuppetfileChecksum)
	require.Equal(t, "production", re.Branch)
	require.Equal(t, "git@example.com:c.git", re.GitURL)
	require.Empty(t, re.Modules)
	require.NotNil(t, re.Modules, "modules must serialize as [] and not null")
}

func TestReportRedactsCredentials(t *testing.T) {
	tests := map[string]string{
		"https://user:secret@example.com/repo.git":  "https://user:REDACTED@example.com/repo.git",
		"https://example.com/a.tar.gz?token=secret": "https://example.com/a.tar.gz?REDACTED",
		"https://token@example.com/repo.git":        "https://token@example.com/repo.git",
		"git@example.com:org/repo.git":              "git@example.com:org/repo.git",
		"/srv/git/repo":                             "/srv/git/repo",
		"":                                          "",
	}
	for in, want := range tests {
		require.Equal(t, want, redactURL(in), in)
	}
	rm := buildReportTarballModule("m", TarballModule{name: "m", source: "https://u:p@example.com/m.tgz?sig=abc"}, t.TempDir())
	require.NotContains(t, rm.Source, "abc")
	require.NotContains(t, rm.Source, ":p@")
}

// TestReportControlBranchFallback uses :control_branch with :default_branch. The control branch only exists
// in one of the repositories, the other one must be reported with the fallback ref that was really deployed.
func TestReportControlBranchFallback(t *testing.T) {
	env := t.TempDir()
	t.Chdir(env)
	t.Setenv("g10k_cachedir", filepath.Join(env, "cache"))
	t.Setenv("g10k_branch", "")

	newRepo := func(name string) (string, func(args ...string) string) {
		repo := filepath.Join(env, name)
		require.NoError(t, os.MkdirAll(repo, 0o755))
		run := func(args ...string) string {
			out, err := exec.Command("git", append([]string{"-C", repo, "-c", "user.name=t", "-c", "user.email=t@example.com", "-c", "init.defaultBranch=main"}, args...)...).CombinedOutput()
			require.NoError(t, err, string(out))
			return strings.TrimSpace(string(out))
		}
		run("init", "-q")
		require.NoError(t, os.WriteFile(filepath.Join(repo, "f"), []byte(name), 0o644))
		run("add", ".")
		run("commit", "-qm", "init")
		run("tag", "v1.0.0")
		return repo, run
	}
	withBranch, runWith := newRepo("with_branch")
	runWith("checkout", "-qb", "feature")
	require.NoError(t, os.WriteFile(filepath.Join(withBranch, "g"), []byte("x"), 0o644))
	runWith("add", ".")
	runWith("commit", "-qm", "feature")
	featureCommit := runWith("rev-parse", "HEAD")
	withoutBranch, runWithout := newRepo("without_branch")
	tagCommit := runWithout("rev-parse", "v1.0.0")

	pfPath := filepath.Join(env, "Puppetfile")
	pf := fmt.Sprintf("mod 'on_branch', :git => '%s', :branch => :control_branch, :default_branch => 'v1.0.0'\nmod 'on_fallback', :git => '%s', :branch => :control_branch, :default_branch => 'v1.0.0'\nmod 'unresolvable', :git => '%s', :branch => :control_branch, :default_branch => 'v9.9.9'\n", withBranch, withoutBranch, withoutBranch)
	require.NoError(t, os.WriteFile(pfPath, []byte(pf), 0o600))

	reportPath := filepath.Join(env, "report.json")
	code, err := Run(Options{Quiet: true, PFMode: true, PFLocation: pfPath, Branch: "feature", ModuleDir: filepath.Join(env, "modules"), MaxWorker: 2, MaxExtractWorker: 2, Report: reportPath})
	require.NoError(t, err)
	require.Equal(t, 0, code)

	modules := readTestReport(t, reportPath).Environments[0]
	onBranch := findReportModule(t, modules, "on_branch")
	require.Equal(t, "feature", onBranch.Ref)
	require.Equal(t, featureCommit, onBranch.Commit)

	fallback := findReportModule(t, modules, "on_fallback")
	require.Equal(t, "v1.0.0", fallback.Ref, "the fallback, not the control branch, was deployed")
	require.Equal(t, ReportStateDeployed, fallback.State)
	require.Equal(t, tagCommit, fallback.Commit)

	unresolvable := findReportModule(t, modules, "unresolvable")
	require.Equal(t, ReportStateMissing, unresolvable.State)
	require.Empty(t, unresolvable.Ref)
}

func TestReportDisabledAndValidation(t *testing.T) {
	// no -report: nothing collected
	rt := NewRuntime(Options{})
	rt.collectReport(map[string]Puppetfile{"x": {}})
	require.Nil(t, rt.reportEnvironments)

	require.NoError(t, checkReportPath(""))
	require.NoError(t, checkReportPath(filepath.Join(t.TempDir(), "r.json")))
	require.ErrorContains(t, checkReportPath(filepath.Join(t.TempDir(), "missing", "r.json")), "does not exist")
	require.ErrorContains(t, checkReportPath(t.TempDir()), "is a directory")

	// Run() fails before doing any work
	code, err := Run(Options{PFMode: true, Report: filepath.Join(t.TempDir(), "missing", "r.json")})
	require.Equal(t, 1, code)
	require.ErrorContains(t, err, "-report")
}

func TestReportWriteIsAtomicAndEmptyListSerialized(t *testing.T) {
	path := filepath.Join(t.TempDir(), "report.json")
	require.NoError(t, os.WriteFile(path, []byte("old"), 0o644))
	rt := NewRuntime(Options{Report: path})
	require.NoError(t, rt.writeReport("target"))
	report := readTestReport(t, path)
	require.Equal(t, "target", report.Target)
	require.NotNil(t, report.Environments)
	entries, err := os.ReadDir(filepath.Dir(path))
	require.NoError(t, err)
	require.Len(t, entries, 1, "no temporary file may be left behind")
	fi, err := os.Stat(path)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o644), fi.Mode().Perm())
}
