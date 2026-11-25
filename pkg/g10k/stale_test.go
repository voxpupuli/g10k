package g10k

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestPurgeControlRepoPreservingResourceTypes verifies that, by default,
// .resource_types is wiped like any other unmanaged content when a control
// repo environment is resynced (matching g10k's historical behaviour), and
// that adding "resource_types" to purge_levels protects it, mirroring how
// moduledir is already preserved.
func TestPurgeControlRepoPreservingResourceTypes(t *testing.T) {
	makeEnv := func(t *testing.T) string {
		t.Helper()
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "Puppetfile"), []byte(""), 0o644))
		require.NoError(t, os.MkdirAll(filepath.Join(dir, "modules", "stdlib"), 0o755))
		require.NoError(t, os.MkdirAll(filepath.Join(dir, resourceTypesDirName), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, resourceTypesDirName, "foo.pp"), []byte(""), 0o644))
		return dir
	}

	t.Run("resource_types is wiped without the purge_level", func(t *testing.T) {
		rt := NewRuntime(Options{Quiet: true})
		dir := makeEnv(t)

		preserve := []string{filepath.Join(dir, "modules")}
		if stringSliceContains(rt.Config.PurgeLevels, "resource_types") {
			preserve = append(preserve, filepath.Join(dir, resourceTypesDirName))
		}
		rt.purgeControlRepoPreserving(dir, preserve)

		require.False(t, fileExists(filepath.Join(dir, resourceTypesDirName)))
		require.True(t, fileExists(filepath.Join(dir, "modules", "stdlib")))
		require.False(t, fileExists(filepath.Join(dir, "Puppetfile")))
	})

	t.Run("resource_types is preserved with the purge_level opt-in", func(t *testing.T) {
		rt := NewRuntime(Options{Quiet: true})
		rt.Config.PurgeLevels = []string{"resource_types"}
		dir := makeEnv(t)

		preserve := []string{filepath.Join(dir, "modules")}
		if stringSliceContains(rt.Config.PurgeLevels, "resource_types") {
			preserve = append(preserve, filepath.Join(dir, resourceTypesDirName))
		}
		rt.purgeControlRepoPreserving(dir, preserve)

		require.True(t, fileExists(filepath.Join(dir, resourceTypesDirName, "foo.pp")))
		require.True(t, fileExists(filepath.Join(dir, "modules", "stdlib")))
		require.False(t, fileExists(filepath.Join(dir, "Puppetfile")))
	})
}
