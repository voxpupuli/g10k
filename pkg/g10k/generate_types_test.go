package g10k

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGenerateTypesForEnvironment(t *testing.T) {
	tmpDir := t.TempDir()
	logFile := filepath.Join(tmpDir, "puppet-generate-types.log")
	stubPuppet := filepath.Join(tmpDir, "puppet")
	script := "#!/bin/sh\necho \"$@\" >> " + logFile + "\n"
	require.NoError(t, os.WriteFile(stubPuppet, []byte(script), 0o755))

	puppetConf := filepath.Join(tmpDir, "puppet.conf")
	require.NoError(t, os.WriteFile(puppetConf, []byte(""), 0o644))

	rt := NewRuntime(Options{Quiet: true})
	rt.Config.GenerateTypes = true
	rt.Config.PuppetPath = stubPuppet
	rt.Config.PuppetConf = puppetConf
	rt.Config.Timeout = 5

	rt.generateTypesForEnvironment("/tmp/example", "example_single")

	content, err := os.ReadFile(logFile)
	require.NoError(t, err)
	require.Contains(t, string(content), "generate types --environment example_single --environmentpath /tmp/example --config "+puppetConf)
}

func TestGenerateTypesForEnvironmentDisabled(t *testing.T) {
	tmpDir := t.TempDir()
	logFile := filepath.Join(tmpDir, "puppet-generate-types.log")
	stubPuppet := filepath.Join(tmpDir, "puppet")
	script := "#!/bin/sh\necho \"$@\" >> " + logFile + "\n"
	require.NoError(t, os.WriteFile(stubPuppet, []byte(script), 0o755))

	rt := NewRuntime(Options{Quiet: true})
	rt.Config.GenerateTypes = false
	rt.Config.PuppetPath = stubPuppet

	rt.generateTypesForEnvironment("/tmp/example", "example_single")

	require.False(t, fileExists(logFile), "puppet generate types should not run when generate_types is disabled")
}

func TestGenerateTypesForEnvironmentSkippedOnDryRun(t *testing.T) {
	tmpDir := t.TempDir()
	logFile := filepath.Join(tmpDir, "puppet-generate-types.log")
	stubPuppet := filepath.Join(tmpDir, "puppet")
	script := "#!/bin/sh\necho \"$@\" >> " + logFile + "\n"
	require.NoError(t, os.WriteFile(stubPuppet, []byte(script), 0o755))

	rt := NewRuntime(Options{Quiet: true, DryRun: true})
	rt.Config.GenerateTypes = true
	rt.Config.PuppetPath = stubPuppet

	rt.generateTypesForEnvironment("/tmp/example", "example_single")

	require.False(t, fileExists(logFile), "puppet generate types should not run during -dryrun")
}

func TestGenerateTypesForEnvironmentMissingPuppet(t *testing.T) {
	rt := NewRuntime(Options{Quiet: true})
	rt.Config.GenerateTypes = true
	rt.Config.PuppetPath = "/does/not/exist/puppet"

	// Should warn and return without crashing g10k
	rt.generateTypesForEnvironment("/tmp/example", "example_single")
}
