package g10k

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-c", "user.name=g10k", "-c", "user.email=g10k@example.com"}, args...)...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed: %v: %s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

// newTestGitRemote creates a non-bare repository with the branches main and feature,
// returning its path and a function that adds a commit to the given branch.
func newTestGitRemote(t *testing.T) (string, func(branch string)) {
	t.Helper()
	dir := t.TempDir()
	runGit(t, dir, "init", "-q", "-b", "main")
	commit := func(branch string) {
		runGit(t, dir, "checkout", "-q", branch)
		if err := os.WriteFile(filepath.Join(dir, "file.txt"), []byte(branch+" "+runGit(t, dir, "rev-parse", "HEAD")), 0644); err != nil {
			t.Fatal(err)
		}
		runGit(t, dir, "add", "file.txt")
		runGit(t, dir, "commit", "-q", "-m", "update "+branch)
	}
	if err := os.WriteFile(filepath.Join(dir, "file.txt"), []byte("init"), 0644); err != nil {
		t.Fatal(err)
	}
	runGit(t, dir, "add", "file.txt")
	runGit(t, dir, "commit", "-q", "-m", "init")
	runGit(t, dir, "branch", "feature")
	return dir, commit
}

func newTestGitRuntime(t *testing.T) *Runtime {
	t.Helper()
	rt := NewRuntime(Options{Quiet: true})
	base := t.TempDir()
	rt.Config = ConfigSettings{
		EnvCacheDir:     filepath.Join(base, "envs-cache"),
		ModulesCacheDir: filepath.Join(base, "modules-cache"),
	}
	return rt
}

func refHash(t *testing.T, gitDir string, ref string) string {
	t.Helper()
	return runGit(t, gitDir, "rev-parse", ref)
}

func TestDoMirrorOrUpdateFetchBranchOnlyUpdatesRequestedBranch(t *testing.T) {
	remote, commit := newTestGitRemote(t)
	rt := newTestGitRuntime(t)
	workDir := filepath.Join(t.TempDir(), "mirror.git")
	gm := GitModule{git: remote}

	if !rt.doMirrorOrUpdate(gm, workDir, "", 0) {
		t.Fatal("initial mirror clone failed")
	}
	oldMain := refHash(t, workDir, "refs/heads/main")
	oldFeature := refHash(t, workDir, "refs/heads/feature")

	commit("main")
	commit("feature")
	newMain := refHash(t, remote, "refs/heads/main")
	newFeature := refHash(t, remote, "refs/heads/feature")

	if !rt.doMirrorOrUpdate(gm, workDir, "feature", 0) {
		t.Fatal("branch fetch of feature failed")
	}
	if got := refHash(t, workDir, "refs/heads/feature"); got != newFeature {
		t.Errorf("feature branch not updated: got %s, want %s", got, newFeature)
	}
	if got := refHash(t, workDir, "refs/heads/main"); got != oldMain || got == newMain {
		t.Errorf("main branch was updated by a single branch fetch: got %s, want untouched %s", got, oldMain)
	}
	if oldFeature == newFeature {
		t.Fatal("test setup error: feature branch did not change")
	}

	if !rt.doMirrorOrUpdate(gm, workDir, "", 0) {
		t.Fatal("full remote update failed")
	}
	if got := refHash(t, workDir, "refs/heads/main"); got != newMain {
		t.Errorf("full remote update did not update main: got %s, want %s", got, newMain)
	}
}

func TestDoMirrorOrUpdateFetchBranchFallsBackToRemoteUpdate(t *testing.T) {
	remote, commit := newTestGitRemote(t)
	rt := newTestGitRuntime(t)
	workDir := filepath.Join(t.TempDir(), "mirror.git")
	gm := GitModule{git: remote}

	if !rt.doMirrorOrUpdate(gm, workDir, "", 0) {
		t.Fatal("initial mirror clone failed")
	}

	// the branch only exists in the remote after the mirror was created, so the
	// targeted fetch of a missing branch fails and must fall back to a full update
	runGit(t, remote, "branch", "newbranch")
	commit("newbranch")
	newBranch := refHash(t, remote, "refs/heads/newbranch")

	if !rt.doMirrorOrUpdate(gm, workDir, "newbranch", 0) {
		t.Fatal("doMirrorOrUpdate() should succeed by falling back to git remote update --prune")
	}
	if got := refHash(t, workDir, "refs/heads/newbranch"); got != newBranch {
		t.Errorf("fallback did not fetch newbranch: got %s, want %s", got, newBranch)
	}
}
