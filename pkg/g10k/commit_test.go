package g10k

import (
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
