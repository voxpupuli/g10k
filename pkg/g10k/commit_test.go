package g10k

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunCommitRequiresBranchOrEnvironment(t *testing.T) {
	code, err := Run(Options{
		ConfigFile: "dummy.yaml",
		Commit:     "abc123def456",
	})
	if code != 1 {
		t.Fatalf("expected exit code 1, got %d", code)
	}
	if err == nil {
		t.Fatal("expected error when -commit is set without -branch or -environment")
	}
	if !strings.Contains(err.Error(), "-commit specified without -branch or -environment") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunCommitAllowedWithBranch(t *testing.T) {
	// Validation for -commit alone must not fire when -branch is also set.
	// readConfigfile will fail on the missing file; that is expected here.
	code, err := Run(Options{
		ConfigFile: "dummy-does-not-exist.yaml",
		Branch:     "production",
		Commit:     "abc123def456",
	})
	if code != 1 {
		t.Fatalf("expected exit code 1 from missing config, got %d", code)
	}
	if err == nil {
		t.Fatal("expected error from missing config file")
	}
	if strings.Contains(err.Error(), "-commit specified without") {
		t.Fatalf("commit+branch should pass commit validation, got: %v", err)
	}
}

func TestCommitMatchesExpected(t *testing.T) {
	full := "da1560886d4f094c3e6c9ef40349f7d38b5d27d7"
	cases := []struct {
		resolved string
		expected string
		want     bool
	}{
		{full, full, true},
		{full, strings.ToUpper(full), true},
		{full, full[:7], true},
		{full, "deadbeef", false},
		{full, "", false},
		{"", full, false},
	}
	for _, tc := range cases {
		if got := commitMatchesExpected(tc.resolved, tc.expected); got != tc.want {
			t.Fatalf("commitMatchesExpected(%q, %q)=%v, want %v", tc.resolved, tc.expected, got, tc.want)
		}
	}
}

func TestResolveAndValidateCommit(t *testing.T) {
	dir := t.TempDir()
	repo := filepath.Join(dir, "repo.git")
	if err := exec.Command("git", "init", "--bare", repo).Run(); err != nil {
		t.Fatalf("git init --bare: %v", err)
	}

	work := filepath.Join(dir, "work")
	if err := os.Mkdir(work, 0o755); err != nil {
		t.Fatal(err)
	}
	cmds := [][]string{
		{"git", "clone", repo, work},
	}
	for _, c := range cmds {
		cmd := exec.Command(c[0], c[1:]...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v: %v (%s)", c, err, out)
		}
	}

	write := func(msg string) string {
		if err := os.WriteFile(filepath.Join(work, "Puppetfile"), []byte(msg+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		run := func(args ...string) {
			cmd := exec.Command("git", args...)
			cmd.Dir = work
			cmd.Env = append(os.Environ(),
				"GIT_AUTHOR_NAME=test",
				"GIT_AUTHOR_EMAIL=test@example.com",
				"GIT_COMMITTER_NAME=test",
				"GIT_COMMITTER_EMAIL=test@example.com",
			)
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("git %v: %v (%s)", args, err, out)
			}
		}
		run("add", "Puppetfile")
		run("commit", "-m", msg)
		run("push", "origin", "HEAD:main")
		out, err := exec.Command("git", "-C", work, "rev-parse", "HEAD").Output()
		if err != nil {
			t.Fatal(err)
		}
		return strings.TrimSpace(string(out))
	}

	sha1 := write("one")
	_ = write("two")

	rt := NewRuntime(Options{})
	rt.Config.Timeout = 30

	t.Run("full sha", func(t *testing.T) {
		got := rt.resolveAndValidateCommit(repo, sha1)
		if got != sha1 {
			t.Fatalf("got %s, want %s", got, sha1)
		}
	})

	t.Run("abbreviated sha", func(t *testing.T) {
		got := rt.resolveAndValidateCommit(repo, sha1[:8])
		if got != sha1 {
			t.Fatalf("got %s, want %s", got, sha1)
		}
	})

	t.Run("missing sha", func(t *testing.T) {
		funcName := "TestResolveAndValidateCommit/missing_sha"
		if os.Getenv("TEST_FOR_CRASH_"+funcName) == "1" {
			rt.resolveAndValidateCommit(repo, "0000000000000000000000000000000000000001")
			return
		}
		cmd := exec.Command(os.Args[0], "-test.run=TestResolveAndValidateCommit/missing_sha$")
		cmd.Env = append(os.Environ(), "TEST_FOR_CRASH_"+funcName+"=1")
		out, err := cmd.CombinedOutput()
		if err == nil {
			t.Fatalf("expected process to exit with error, out=%s", out)
		}
		if !strings.Contains(string(out), "could not be found") {
			t.Fatalf("expected missing-commit message, got: %s", out)
		}
	})
}
