package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupTestDirs creates isolated temp directories for a test and returns outDir
// (the directory where environments will be deployed). It sets g10k_cachedir via
// t.Setenv so that readConfigfile picks it up; the env var is restored after the test.
func setupTestDirs(t *testing.T) string {
	t.Helper()
	tmpDir := t.TempDir()
	t.Setenv("g10k_cachedir", filepath.Join(tmpDir, "cache"))
	return filepath.Join(tmpDir, "out")
}

// overrideSourceBasedirs sets every source in rt.Config to deploy into basedir.
func overrideSourceBasedirs(rt *Runtime, basedir string) {
	for name, sa := range rt.Config.Sources {
		sa.Basedir = basedir
		rt.Config.Sources[name] = sa
	}
}

// TestResolveStaticWithSnapshot tests static environment deployment and verifies
// the complete directory structure matches expectations without external dependencies.
func TestResolveStaticWithSnapshot(t *testing.T) {
	rt := NewRuntime(Options{Quiet: true})

	outDir := setupTestDirs(t)
	rt.Config = mustReadConfigfile(t, rt, "tests/TestConfigStatic.yaml")
	overrideSourceBasedirs(rt, outDir)
	rt.Config.Maxworker = 500
	rt.Branch = "static"

	mustResolvePuppetEnvironment(rt, false, "")

	deployDir := filepath.Join(outDir, "example_static")
	removeTimestampsFromDeployfile(rt, filepath.Join(deployDir, ".g10k-deploy.json"))

	actual, err := SnapshotDirectory(deployDir)
	require.NoError(t, err, "Failed to snapshot deployed environment")
	require.NotNil(t, actual)
	require.Greater(t, actual.Count, 0, "Should have deployed files")

	expected, err := LoadSnapshot("tests/snapshots/example_static.json")
	if err != nil && os.IsNotExist(err) {
		require.NoError(t, os.MkdirAll("tests/snapshots", 0755))
		require.NoError(t, SaveSnapshot(actual, "tests/snapshots/example_static.json"), "Failed to save reference snapshot")
		t.Skip("Reference snapshot created; run test again to verify")
	}
	require.NoError(t, err, "Failed to load expected snapshot")

	result := CompareSnapshots(expected, actual)
	assert.True(t, result.IsIdentical(), "Deployed files don't match expected state:\n%s", result.String())
	assert.Equal(t, len(expected.Files), result.MatchCount, "Expected all files to match")
}

// TestResolveStaticChangesDetected verifies that changes to deployed files are detected.
func TestResolveStaticChangesDetected(t *testing.T) {
	rt := NewRuntime(Options{Quiet: true})

	outDir := setupTestDirs(t)
	rt.Config = mustReadConfigfile(t, rt, "tests/TestConfigStatic.yaml")
	overrideSourceBasedirs(rt, outDir)
	rt.Config.Maxworker = 500
	rt.Branch = "static"

	mustResolvePuppetEnvironment(rt, false, "")

	deployDir := filepath.Join(outDir, "example_static")
	removeTimestampsFromDeployfile(rt, filepath.Join(deployDir, ".g10k-deploy.json"))

	correct, err := SnapshotDirectory(deployDir)
	require.NoError(t, err)

	// Deliberately delete a directory
	dirToDelete := filepath.Join(deployDir, "external_modules", "stdlib", "spec", "unit", "facter", "util")
	rt.purgeDir(dirToDelete, "TestResolveStaticChangesDetected()")

	modified, err := SnapshotDirectory(deployDir)
	require.NoError(t, err)

	result := CompareSnapshots(correct, modified)
	assert.False(t, result.IsIdentical(), "Should detect that files were deleted")
	assert.Greater(t, len(result.Missing), 0, "Should report missing files")
}

// TestResolveStaticSkiplistWithSnapshot tests skiplist functionality.
func TestResolveStaticSkiplistWithSnapshot(t *testing.T) {
	rt := NewRuntime(Options{Quiet: true})

	outDir := setupTestDirs(t)
	rt.Config = mustReadConfigfile(t, rt, "tests/TestConfigStaticSkiplist.yaml")
	overrideSourceBasedirs(rt, outDir)
	rt.Config.Maxworker = 500
	rt.Branch = "skiplist"

	mustResolvePuppetEnvironment(rt, false, "")

	deployDir := filepath.Join(outDir, "example_skiplist")
	removeTimestampsFromDeployfile(rt, filepath.Join(deployDir, ".g10k-deploy.json"))

	actual, err := SnapshotDirectory(deployDir)
	require.NoError(t, err, "Failed to snapshot skiplist environment")
	require.NotNil(t, actual)
	require.Greater(t, actual.Count, 0)

	expected, err := LoadSnapshot("tests/snapshots/example_skiplist.json")
	if err != nil && os.IsNotExist(err) {
		require.NoError(t, os.MkdirAll("tests/snapshots", 0755))
		require.NoError(t, SaveSnapshot(actual, "tests/snapshots/example_skiplist.json"))
		t.Skip("Reference snapshot created; run test again to verify")
	}
	require.NoError(t, err, "Failed to load expected snapshot")

	result := CompareSnapshots(expected, actual)
	assert.True(t, result.IsIdentical(), "Skiplist deployment doesn't match expected state:\n%s", result.String())

	// Verify skipped directories don't exist
	for _, subDir := range []string{"spec", "readmes", "examples"} {
		path := filepath.Join(deployDir, "external_modules", "stdlib", subDir)
		assert.False(t, fileExists(path), "Skiplisted directory should not exist: %s", path)
	}
}

// TestSymlinkHandlingWithSnapshot verifies symlinks are correctly deployed and detected.
func TestSymlinkHandlingWithSnapshot(t *testing.T) {
	rt := NewRuntime(Options{Quiet: true})

	outDir := setupTestDirs(t)
	rt.Config = mustReadConfigfile(t, rt, "tests/both.yaml")
	overrideSourceBasedirs(rt, outDir)
	rt.Config.Maxworker = 500
	rt.Environment = "full_symlinks"

	mustResolvePuppetEnvironment(rt, false, "")

	deployDir := filepath.Join(outDir, "full_symlinks")
	removeTimestampsFromDeployfile(rt, filepath.Join(deployDir, ".g10k-deploy.json"))

	actual, err := SnapshotDirectory(deployDir)
	require.NoError(t, err, "Failed to snapshot symlink environment")
	require.NotNil(t, actual)
	require.Greater(t, actual.Count, 0)

	expected, err := LoadSnapshot("tests/snapshots/symlinks.json")
	if err != nil && os.IsNotExist(err) {
		require.NoError(t, os.MkdirAll("tests/snapshots", 0755))
		require.NoError(t, SaveSnapshot(actual, "tests/snapshots/symlinks.json"))
		t.Skip("Reference snapshot created; run test again to verify")
	}
	require.NoError(t, err, "Failed to load expected snapshot")

	result := CompareSnapshots(expected, actual)
	assert.True(t, result.IsIdentical(), "Symlink deployment doesn't match expected state:\n%s", result.String())

	// Verify that symlinks with non-existent targets are preserved (#150)
	for _, symlink := range []string{
		filepath.Join(deployDir, "modules", "testmodule", "not-working-symlink"),
		filepath.Join(deployDir, "1", "not-working-symlink"),
	} {
		assert.True(t, fileExists(symlink), "Symlink with non-existent target should exist: %s", symlink)
	}
}

// TestSymlinkStabilityAcrossRuns verifies symlink deployments are stable across multiple runs.
func TestSymlinkStabilityAcrossRuns(t *testing.T) {
	rt := NewRuntime(Options{Quiet: true})

	outDir := setupTestDirs(t)
	rt.Config = mustReadConfigfile(t, rt, "tests/both.yaml")
	overrideSourceBasedirs(rt, outDir)
	rt.Config.Maxworker = 500
	rt.Environment = "full_symlinks"

	deployDir := filepath.Join(outDir, "full_symlinks")
	var snapshots []*DirectorySnapshot

	for i := 0; i < 3; i++ {
		mustResolvePuppetEnvironment(rt, false, "")
		removeTimestampsFromDeployfile(rt, filepath.Join(deployDir, ".g10k-deploy.json"))

		snapshot, err := SnapshotDirectory(deployDir)
		require.NoError(t, err, "Failed to snapshot on iteration %d", i+1)
		snapshots = append(snapshots, snapshot)
	}

	for i := 1; i < len(snapshots); i++ {
		result := CompareSnapshots(snapshots[0], snapshots[i])
		assert.True(t, result.IsIdentical(),
			"Snapshot %d differs from snapshot 0:\n%s", i, result.String())
	}
}

// TestConfigFullworkingMultipleSourcesWithSnapshot verifies that deploying the same
// branch from multiple sources produces the expected result.
func TestConfigFullworkingMultipleSourcesWithSnapshot(t *testing.T) {
	rt := NewRuntime(Options{Quiet: true})

	outDir := setupTestDirs(t)
	rt.Config = mustReadConfigfile(t, rt, "tests/TestConfigFullworkingAndExample.yaml")
	overrideSourceBasedirs(rt, outDir)
	rt.Config.Maxworker = 500
	// Use a specific branch to avoid deploying broken environments from the example repo
	rt.Branch = "single"

	mustResolvePuppetEnvironment(rt, false, "")

	// Remove timestamps from all deployed environments
	for _, env := range []string{"example_single", "full_single"} {
		removeTimestampsFromDeployfile(rt, filepath.Join(outDir, env, ".g10k-deploy.json"))
	}

	actual, err := SnapshotDirectory(outDir)
	require.NoError(t, err, "Failed to snapshot multiple sources")
	require.NotNil(t, actual)
	require.Greater(t, actual.Count, 0)

	expected, err := LoadSnapshot("tests/snapshots/fullworking_multiple.json")
	if err != nil && os.IsNotExist(err) {
		require.NoError(t, os.MkdirAll("tests/snapshots", 0755))
		require.NoError(t, SaveSnapshot(actual, "tests/snapshots/fullworking_multiple.json"))
		t.Skip("Reference snapshot created; run test again to verify")
	}
	require.NoError(t, err, "Failed to load expected snapshot")

	result := CompareSnapshots(expected, actual)
	assert.True(t, result.IsIdentical(), "Multiple sources deployment doesn't match:\n%s", result.String())
}

// TestFilePermissionsPreserved verifies that file permissions are correctly preserved during deployment.
func TestFilePermissionsPreserved(t *testing.T) {
	rt := NewRuntime(Options{Quiet: true})

	outDir := setupTestDirs(t)
	rt.Config = mustReadConfigfile(t, rt, "tests/TestConfigStatic.yaml")
	overrideSourceBasedirs(rt, outDir)
	rt.Config.Maxworker = 500
	rt.Branch = "static"

	mustResolvePuppetEnvironment(rt, false, "")

	testFile := filepath.Join(outDir, "example_static", "external_modules", "aws",
		"examples", "audit-security-groups", "count_out_of_sync_resources.sh")

	fileInfo, err := os.Stat(testFile)
	require.NoError(t, err, "Test file should exist: %s", testFile)

	assert.Equal(t, "-rwxrwxr-x", fileInfo.Mode().String(),
		"File permissions not preserved correctly for %s", testFile)
}
