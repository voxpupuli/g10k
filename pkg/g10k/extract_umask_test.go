package g10k

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// tarSpec describes a single entry written into a tar stream.
type tarSpec struct {
	name   string
	isDir  bool
	mode   int64
	content string
}

// buildRawTar builds an uncompressed tar stream from the given specs.
func buildRawTar(t *testing.T, specs []tarSpec) []byte {
	t.Helper()
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	for _, spec := range specs {
		kind := tar.TypeReg
		mode := int64(0o644)
		if spec.isDir {
			kind = tar.TypeDir
			mode = 0o755
		}
		if spec.mode != 0 {
			mode = spec.mode
		}
		header := &tar.Header{Name: spec.name, Typeflag: byte(kind), Mode: mode}
		if kind == tar.TypeReg {
			header.Size = int64(len(spec.content))
		}
		require.NoError(t, tw.WriteHeader(header))
		if kind == tar.TypeReg {
			_, err := io.WriteString(tw, spec.content)
			require.NoError(t, err)
		}
	}
	require.NoError(t, tw.Close())
	return buf.Bytes()
}

// buildGzTar builds a gzip-compressed tar stream from the given specs.
func buildGzTar(t *testing.T, specs []tarSpec) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, spec := range specs {
		kind := tar.TypeReg
		mode := int64(0o644)
		if spec.isDir {
			kind = tar.TypeDir
			mode = 0o755
		}
		if spec.mode != 0 {
			mode = spec.mode
		}
		header := &tar.Header{Name: spec.name, Typeflag: byte(kind), Mode: mode}
		if kind == tar.TypeReg {
			header.Size = int64(len(spec.content))
		}
		require.NoError(t, tw.WriteHeader(header))
		if kind == tar.TypeReg {
			_, err := io.WriteString(tw, spec.content)
			require.NoError(t, err)
		}
	}
	require.NoError(t, tw.Close())
	require.NoError(t, gz.Close())
	return buf.Bytes()
}

// withZeroUmask runs fn with the process umask cleared so directory modes are
// deterministic regardless of the test runner's own umask.
func withZeroUmask(t *testing.T, fn func()) {
	t.Helper()
	prev := syscall.Umask(0)
	defer func() { _ = syscall.Umask(prev) }()
	fn()
}

func mustPerm(t *testing.T, path string) os.FileMode {
	t.Helper()
	fi, err := os.Stat(path)
	require.NoError(t, err, "stat %s", path)
	return fi.Mode().Perm()
}

// TestParseExtractUmask checks the octal umask string parser.
func TestParseExtractUmask(t *testing.T) {
	tests := []struct {
		value   string
		want    int
		wantErr bool
	}{
		{value: "", want: 0},
		{value: "   ", want: 0},
		{value: "022", want: 0o022},
		{value: "027", want: 0o027},
		{value: "0o027", want: 0o027},
		{value: "0O027", want: 0o027},
		{value: "077", want: 0o077},
		{value: "abc", wantErr: true},
		{value: "99", wantErr: true},
	}
	for _, tt := range tests {
		got, err := parseExtractUmask(tt.value)
		if tt.wantErr {
			require.Error(t, err, "parseExtractUmask(%q) should fail", tt.value)
			continue
		}
		require.NoError(t, err, "parseExtractUmask(%q) should not fail", tt.value)
		assert.Equal(t, tt.want, got, "parseExtractUmask(%q)", tt.value)
	}
}

// TestExtractUmaskUnTar checks that the umask is applied to git/forge extraction
// and that execute bits are preserved.
func TestExtractUmaskUnTar(t *testing.T) {
	withZeroUmask(t, func() {
		rt := NewRuntime(Options{Quiet: true})
		rt.Config.ExtractUmask = 0o027
		target := t.TempDir()

		specs := []tarSpec{
			{name: "mod", isDir: true},
			{name: "mod/bin", isDir: true},
			{name: "mod/conf.pp", mode: 0o644, content: "class a {}\n"},
			{name: "mod/bin/fact", mode: 0o755, content: "#!/bin/sh\necho hi\n"},
			{name: "mod/secret", mode: 0o600, content: "s"},
		}
		data := buildRawTar(t, specs)

		rt.unTar(bytes.NewReader(data), target)

		assert.Equal(t, os.FileMode(0o750), mustPerm(t, filepath.Join(target, "mod")), "dir mode")
		assert.Equal(t, os.FileMode(0o640), mustPerm(t, filepath.Join(target, "mod/conf.pp")), "644 mode")
		assert.Equal(t, os.FileMode(0o750), mustPerm(t, filepath.Join(target, "mod/bin/fact")), "755 mode")
		assert.Equal(t, os.FileMode(0o600), mustPerm(t, filepath.Join(target, "mod/secret")), "600 mode")

		// executed scripts must keep the execute bit so facts/scripts stay runnable
		factInfo, err := os.Stat(filepath.Join(target, "mod/bin/fact"))
		require.NoError(t, err)
		assert.NotZero(t, factInfo.Mode().Perm()&0o111, "exec script must keep execute bit")
	})
}

// TestExtractUmaskNoUmaskPassthrough checks that without a umask the original
// modes are preserved unchanged (previous behaviour).
func TestExtractUmaskNoUmaskPassthrough(t *testing.T) {
	withZeroUmask(t, func() {
		rt := NewRuntime(Options{Quiet: true})
		rt.Config.ExtractUmask = 0
		target := t.TempDir()

		specs := []tarSpec{
			{name: "mod", isDir: true},
			{name: "mod/bin", isDir: true},
			{name: "mod/conf.pp", mode: 0o644, content: "class a {}\n"},
			{name: "mod/bin/fact", mode: 0o755, content: "#!/bin/sh\n"},
		}
		data := buildRawTar(t, specs)

		rt.unTar(bytes.NewReader(data), target)

		assert.Equal(t, os.FileMode(0o755), mustPerm(t, filepath.Join(target, "mod")), "dir mode")
		assert.Equal(t, os.FileMode(0o644), mustPerm(t, filepath.Join(target, "mod/conf.pp")), "644 mode")
		assert.Equal(t, os.FileMode(0o755), mustPerm(t, filepath.Join(target, "mod/bin/fact")), "755 mode")
	})
}

// TestExtractUmaskTarball checks that tarball extraction honours the umask too.
func TestExtractUmaskTarball(t *testing.T) {
	withZeroUmask(t, func() {
		rt := tarballTestRuntime(t, Options{Quiet: true})
		rt.Config.ExtractUmask = 0o027
		target := t.TempDir()

		specs := []tarSpec{
			{name: "mod", isDir: true},
			{name: "mod/bin", isDir: true},
			{name: "mod/conf.pp", mode: 0o644, content: "class a {}\n"},
			{name: "mod/bin/fact", mode: 0o755, content: "#!/bin/sh\n"},
		}
		archive := buildGzTar(t, specs)
		archivePath := filepath.Join(t.TempDir(), "mod.tar.gz")
		require.NoError(t, os.WriteFile(archivePath, archive, 0o600))

		require.NoError(t, rt.extractTarball(archivePath, target, 0))

		assert.Equal(t, os.FileMode(0o750), mustPerm(t, filepath.Join(target, "mod")), "dir mode")
		assert.Equal(t, os.FileMode(0o640), mustPerm(t, filepath.Join(target, "mod/conf.pp")), "644 mode")
		assert.Equal(t, os.FileMode(0o750), mustPerm(t, filepath.Join(target, "mod/bin/fact")), "755 mode")
	})
}
