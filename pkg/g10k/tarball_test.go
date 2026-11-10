package g10k

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type archiveEntry struct {
	name, content, link string
	kind                byte
}

func testTarball(t *testing.T, entries ...archiveEntry) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, entry := range entries {
		kind := entry.kind
		if kind == 0 {
			kind = tar.TypeReg
		}
		header := &tar.Header{Name: entry.name, Typeflag: kind, Mode: 0o644, Linkname: entry.link}
		if kind == tar.TypeDir {
			header.Mode = 0o755
		}
		if kind == tar.TypeReg {
			header.Size = int64(len(entry.content))
		}
		require.NoError(t, tw.WriteHeader(header))
		if kind == tar.TypeReg {
			_, err := io.WriteString(tw, entry.content)
			require.NoError(t, err)
		}
	}
	require.NoError(t, tw.Close())
	require.NoError(t, gz.Close())
	return buf.Bytes()
}

func tarballTestRuntime(t *testing.T, opts Options) *Runtime {
	t.Helper()
	rt := NewRuntime(opts)
	rt.Config.CacheDir = filepath.Join(t.TempDir(), "cache")
	rt.Config.Maxworker = 2
	rt.Config.MaxExtractworker = 2
	rt.Config.Timeout = 5
	rt.Config.PurgeLevels = []string{"puppetfile"}
	return rt
}

func parseTestTarballPuppetfile(t *testing.T, rt *Runtime, dir, content string) Puppetfile {
	t.Helper()
	path := filepath.Join(dir, "Puppetfile")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	pf, err := rt.ParsePuppetfile(path, "", "test", "main", false)
	require.NoError(t, err)
	pf.workDir = dir
	return pf
}

func TestTarballAcceptanceDirectCacheAndPurge(t *testing.T) {
	archive := testTarball(t, archiveEntry{name: "manifests/init.pp", content: "class apache {}"})
	digest := fmt.Sprintf("%x", sha256.Sum256(archive))
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		_, _ = w.Write(archive)
	}))
	defer server.Close()
	rt := tarballTestRuntime(t, Options{Quiet: true})
	env1, env2 := t.TempDir(), t.TempDir()
	content := fmt.Sprintf("moduledir 'vendor'\nmod 'puppetlabs-apache', type: 'tarball', source: '%s/archive', version: '%s'\n", server.URL, digest)
	pf1 := parseTestTarballPuppetfile(t, rt, env1, content)
	pf2 := parseTestTarballPuppetfile(t, rt, env2, content)
	stale := filepath.Join(env1, "vendor", "stale")
	require.NoError(t, os.MkdirAll(stale, 0o755))
	require.NoError(t, rt.resolvePuppetfile(map[string]Puppetfile{"one": pf1, "two": pf2}))
	for _, env := range []string{env1, env2} {
		data, err := os.ReadFile(filepath.Join(env, "vendor", "apache", "manifests", "init.pp"))
		require.NoError(t, err)
		require.Equal(t, "class apache {}", string(data))
	}
	_, err := os.Stat(stale)
	require.ErrorIs(t, err, os.ErrNotExist)
	require.EqualValues(t, 1, requests.Load())
	require.Equal(t, 2, rt.NeedSyncTarballCount)
	require.NoError(t, rt.resolvePuppetfile(map[string]Puppetfile{"one": pf1, "two": pf2}))
	require.Equal(t, 2, rt.NeedSyncTarballCount)
	require.EqualValues(t, 1, requests.Load())
	require.NoError(t, os.WriteFile(filepath.Join(env1, "vendor", "apache", "manifests", "init.pp"), []byte("modified"), 0o644))
	require.NoError(t, rt.resolvePuppetfile(map[string]Puppetfile{"one": pf1, "two": pf2}))
	require.Equal(t, 3, rt.NeedSyncTarballCount)
	require.EqualValues(t, 1, requests.Load())
}

type githubTestTransport struct {
	server *url.URL
	base   http.RoundTripper
	paths  []string
}

func (t *githubTestTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	t.paths = append(t.paths, req.URL.EscapedPath())
	copy := req.Clone(req.Context())
	u := *req.URL
	u.Scheme, u.Host = t.server.Scheme, t.server.Host
	copy.URL = &u
	return t.base.RoundTrip(copy)
}

func TestTarballAcceptanceGitHub(t *testing.T) {
	archive := testTarball(t,
		archiveEntry{name: "owner-repo-123/", kind: tar.TypeDir},
		archiveEntry{name: "owner-repo-123/metadata.json", content: `{"name":"apache"}`},
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(archive)
	}))
	defer server.Close()
	serverURL, err := url.Parse(server.URL)
	require.NoError(t, err)
	transport := &githubTestTransport{server: serverURL, base: server.Client().Transport}
	rt := tarballTestRuntime(t, Options{Quiet: true, ModuleDir: "custom"})
	rt.tarballClient = &http.Client{Transport: transport}
	env := t.TempDir()
	pf := parseTestTarballPuppetfile(t, rt, env, "moduledir 'ignored'\nmod 'puppetlabs-apache', '0.6.0', :github_tarball => 'puppetlabs/puppetlabs-apache'\n")
	require.NoError(t, rt.resolvePuppetfile(map[string]Puppetfile{"main": pf}))
	require.Equal(t, []string{"/repos/puppetlabs/puppetlabs-apache/tarball/0.6.0"}, transport.paths)
	data, err := os.ReadFile(filepath.Join(env, "custom", "apache", "metadata.json"))
	require.NoError(t, err)
	require.Equal(t, `{"name":"apache"}`, string(data))
	require.NoError(t, rt.resolvePuppetfile(map[string]Puppetfile{"main": pf}))
	require.Len(t, transport.paths, 1)
}

func TestTarballGitHubURLs(t *testing.T) {
	require.Equal(t, "https://api.github.com/repos/owner/repo/tarball",
		(TarballModule{githubRepo: "owner/repo"}).downloadURL())
	require.Equal(t, "https://api.github.com/repos/owner/repo/tarball/feature%2Fbranch",
		(TarballModule{githubRepo: "owner/repo", version: "feature/branch"}).downloadURL())
}

func TestTarballFailureDoesNotReplaceOrPurge(t *testing.T) {
	archive := testTarball(t, archiveEntry{name: "file", content: "new"})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(archive)
	}))
	defer server.Close()
	rt := tarballTestRuntime(t, Options{Quiet: true})
	env := t.TempDir()
	module := filepath.Join(env, "modules", "apache")
	stale := filepath.Join(env, "modules", "stale")
	require.NoError(t, os.MkdirAll(module, 0o755))
	require.NoError(t, os.MkdirAll(stale, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(module, "file"), []byte("old"), 0o644))
	pf := parseTestTarballPuppetfile(t, rt, env, fmt.Sprintf("mod 'apache', type: 'tarball', source: '%s', version: '%s'\n", server.URL, strings.Repeat("a", 64)))
	require.ErrorContains(t, rt.resolvePuppetfile(map[string]Puppetfile{"main": pf}), "SHA-256")
	data, err := os.ReadFile(filepath.Join(module, "file"))
	require.NoError(t, err)
	require.Equal(t, "old", string(data))
	_, err = os.Stat(stale)
	require.NoError(t, err)
	entries, err := os.ReadDir(filepath.Join(rt.Config.CacheDir, "tarball"))
	require.NoError(t, err)
	require.Empty(t, entries)
}

func TestTarballDryRunAndModuleFilter(t *testing.T) {
	archive := testTarball(t, archiveEntry{name: "file", content: "content"})
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		_, _ = w.Write(archive)
	}))
	defer server.Close()
	rt := tarballTestRuntime(t, Options{Quiet: true, DryRun: true, Module: "apache"})
	env := t.TempDir()
	pf := parseTestTarballPuppetfile(t, rt, env, fmt.Sprintf("mod 'author/apache', type: 'tarball', source: '%s/one'\nmod 'author/other', type: 'tarball', source: '%s/two'\n", server.URL, server.URL))
	require.NoError(t, rt.resolvePuppetfile(map[string]Puppetfile{"main": pf}))
	require.Equal(t, 1, rt.NeedSyncTarballCount)
	require.EqualValues(t, 1, requests.Load())
	_, err := os.Stat(filepath.Join(env, "modules"))
	require.ErrorIs(t, err, os.ErrNotExist)
	_, err = os.Stat(rt.Config.CacheDir)
	require.ErrorIs(t, err, os.ErrNotExist)
	code, err := rt.finish("test")
	require.NoError(t, err)
	require.Equal(t, 1, code)
}

func TestTarballCacheCorruptionAndChanges(t *testing.T) {
	first := testTarball(t, archiveEntry{name: "file", content: "first"})
	second := testTarball(t, archiveEntry{name: "file", content: "second"})
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.URL.Path == "/second" {
			_, _ = w.Write(second)
		} else {
			_, _ = w.Write(first)
		}
	}))
	defer server.Close()
	rt := tarballTestRuntime(t, Options{Quiet: true})
	env := t.TempDir()
	parse := func(source, checksum string) Puppetfile {
		version := ""
		if checksum != "" {
			version = ", version: '" + checksum + "'"
		}
		return parseTestTarballPuppetfile(t, rt, env, fmt.Sprintf("mod 'apache', type: 'tarball', source: '%s'%s\n", source, version))
	}
	pf := parse(server.URL+"/first", "")
	require.NoError(t, rt.resolvePuppetfile(map[string]Puppetfile{"main": pf}))
	cacheEntries, err := os.ReadDir(filepath.Join(rt.Config.CacheDir, "tarball"))
	require.NoError(t, err)
	require.Len(t, cacheEntries, 1)
	require.NoError(t, os.WriteFile(filepath.Join(rt.Config.CacheDir, "tarball", cacheEntries[0].Name()), []byte("corrupt"), 0o600))
	require.NoError(t, rt.resolvePuppetfile(map[string]Puppetfile{"main": pf}))
	require.EqualValues(t, 2, requests.Load())
	pf = parse(server.URL+"/second", "")
	require.NoError(t, rt.resolvePuppetfile(map[string]Puppetfile{"main": pf}))
	content, err := os.ReadFile(filepath.Join(env, "modules", "apache", "file"))
	require.NoError(t, err)
	require.Equal(t, "second", string(content))
	require.EqualValues(t, 3, requests.Load())
	pf = parse(server.URL+"/first", fmt.Sprintf("%x", sha256.Sum256(first)))
	require.NoError(t, rt.resolvePuppetfile(map[string]Puppetfile{"main": pf}))
	require.EqualValues(t, 4, requests.Load())
	key := sha256.Sum256([]byte(pf.tarballModules["apache"].cacheKey()))
	cacheFile := filepath.Join(rt.Config.CacheDir, "tarball", fmt.Sprintf("%x.tar.gz", key))
	require.NoError(t, os.WriteFile(cacheFile, second, 0o600))
	require.NoError(t, rt.resolvePuppetfile(map[string]Puppetfile{"main": pf}))
	require.EqualValues(t, 5, requests.Load())
}

func TestTarballCacheWithoutChecksumIsStable(t *testing.T) {
	first := testTarball(t, archiveEntry{name: "file", content: "first"})
	second := testTarball(t, archiveEntry{name: "file", content: "second"})
	var updated atomic.Bool
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		if updated.Load() {
			_, _ = w.Write(second)
		} else {
			_, _ = w.Write(first)
		}
	}))
	defer server.Close()
	rt := tarballTestRuntime(t, Options{Quiet: true})
	env := t.TempDir()
	pf := parseTestTarballPuppetfile(t, rt, env, fmt.Sprintf("mod 'apache', type: 'tarball', source: '%s'\n", server.URL))
	require.NoError(t, rt.resolvePuppetfile(map[string]Puppetfile{"main": pf}))
	updated.Store(true)
	require.NoError(t, rt.resolvePuppetfile(map[string]Puppetfile{"main": pf}))
	data, err := os.ReadFile(filepath.Join(env, "modules", "apache", "file"))
	require.NoError(t, err)
	require.Equal(t, "first", string(data))
	require.EqualValues(t, 1, requests.Load())
}

func TestTarballDownloadFailures(t *testing.T) {
	for _, tt := range []struct {
		name   string
		status int
		body   []byte
		want   string
	}{
		{"404", http.StatusNotFound, nil, "404"},
		{"malformed gzip", http.StatusOK, []byte("not gzip"), "validate downloaded tarball"},
		{"malicious archive", http.StatusOK, testTarball(t, archiveEntry{name: "../escape", content: "bad"}), "unsafe tar entry"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.status)
				_, _ = w.Write(tt.body)
			}))
			defer server.Close()
			rt := tarballTestRuntime(t, Options{Quiet: true})
			env := t.TempDir()
			pf := parseTestTarballPuppetfile(t, rt, env, fmt.Sprintf("mod 'apache', type: 'tarball', source: '%s'\n", server.URL))
			require.ErrorContains(t, rt.resolvePuppetfile(map[string]Puppetfile{"main": pf}), tt.want)
			entries, err := os.ReadDir(filepath.Join(rt.Config.CacheDir, "tarball"))
			require.NoError(t, err)
			require.Empty(t, entries)
		})
	}
}

func TestTarballLocalFileAndSkiplist(t *testing.T) {
	tmp := t.TempDir()
	archive := filepath.Join(tmp, "module.tar.gz")
	require.NoError(t, os.WriteFile(archive, testTarball(t,
		archiveEntry{name: "manifests/init.pp", content: "ok"},
		archiveEntry{name: "spec/test.rb", content: "skip"},
	), 0o600))
	rt := tarballTestRuntime(t, Options{Quiet: true})
	rt.Config.PurgeSkiplist = []string{"spec"}
	env := t.TempDir()
	pf := parseTestTarballPuppetfile(t, rt, env, fmt.Sprintf("mod 'apache', type: 'tarball', source: 'file://%s'\n", archive))
	require.NoError(t, rt.resolvePuppetfile(map[string]Puppetfile{"main": pf}))
	data, err := os.ReadFile(filepath.Join(env, "modules", "apache", "manifests", "init.pp"))
	require.NoError(t, err)
	require.Equal(t, "ok", string(data))
	_, err = os.Stat(filepath.Join(env, "modules", "apache", "spec"))
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestTarballSafeLinks(t *testing.T) {
	archive := testTarball(t,
		archiveEntry{name: "dir/file", content: "content"},
		archiveEntry{name: "dir/link", kind: tar.TypeSymlink, link: "file"},
		archiveEntry{name: "hard", kind: tar.TypeLink, link: "dir/file"},
	)
	tmp := t.TempDir()
	file := filepath.Join(tmp, "archive")
	target := filepath.Join(tmp, "target")
	require.NoError(t, os.WriteFile(file, archive, 0o600))
	require.NoError(t, os.Mkdir(target, 0o755))
	require.NoError(t, tarballTestRuntime(t, Options{}).extractTarball(file, target, false))
	for _, name := range []string{"dir/link", "hard"} {
		data, err := os.ReadFile(filepath.Join(target, name))
		require.NoError(t, err)
		require.Equal(t, "content", string(data))
	}
}

func TestTarballExtractsLegacyRegularFile(t *testing.T) {
	archive := testTarball(t, archiveEntry{name: "file", content: "legacy"})
	gz, err := gzip.NewReader(bytes.NewReader(archive))
	require.NoError(t, err)
	raw, err := io.ReadAll(gz)
	require.NoError(t, err)
	require.NoError(t, gz.Close())

	// Rewrite the regular-file header to use the original NUL type flag.
	raw[156] = 0
	copy(raw[148:156], "        ")
	var checksum int
	for _, b := range raw[:512] {
		checksum += int(b)
	}
	copy(raw[148:156], fmt.Sprintf("%06o\x00 ", checksum))
	var compressed bytes.Buffer
	writer := gzip.NewWriter(&compressed)
	_, err = writer.Write(raw)
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	tmp := t.TempDir()
	file := filepath.Join(tmp, "archive")
	target := filepath.Join(tmp, "target")
	require.NoError(t, os.WriteFile(file, compressed.Bytes(), 0o600))
	require.NoError(t, os.Mkdir(target, 0o755))
	require.NoError(t, tarballTestRuntime(t, Options{}).extractTarball(file, target, false))
	data, err := os.ReadFile(filepath.Join(target, "file"))
	require.NoError(t, err)
	require.Equal(t, "legacy", string(data))
}

type tarballTestTransportFunc func(*http.Request) (*http.Response, error)

func (f tarballTestTransportFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

type tarballTestCloseErrorBody struct {
	io.Reader
	err error
}

func (b tarballTestCloseErrorBody) Close() error { return b.err }

func TestTarballDownloadReportsCloseErrors(t *testing.T) {
	closeErr := errors.New("close failed")
	for _, status := range []int{http.StatusOK, http.StatusNotFound} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			client := &http.Client{Transport: tarballTestTransportFunc(func(*http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: status,
					Status:     http.StatusText(status),
					Body:       tarballTestCloseErrorBody{Reader: strings.NewReader("content"), err: closeErr},
				}, nil
			})}
			err := downloadTarball(client, "https://example.test/archive", io.Discard)
			require.ErrorIs(t, err, closeErr)
			if status != http.StatusOK {
				require.ErrorContains(t, err, http.StatusText(status))
			}
		})
	}
}

func TestTarballConcurrentDownloadsRespectLimit(t *testing.T) {
	archive := testTarball(t, archiveEntry{name: "file", content: "ok"})
	var active, maxActive, requests atomic.Int32
	started := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		current := active.Add(1)
		defer active.Add(-1)
		for {
			previous := maxActive.Load()
			if current <= previous || maxActive.CompareAndSwap(previous, current) {
				break
			}
		}
		if requests.Add(1) == 2 {
			close(started)
		}
		select {
		case <-started:
		case <-time.After(5 * time.Second):
			http.Error(w, "concurrent request did not start", http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write(archive)
	}))
	defer server.Close()
	rt := tarballTestRuntime(t, Options{Quiet: true})
	env := t.TempDir()
	pf := parseTestTarballPuppetfile(t, rt, env, fmt.Sprintf(
		"mod 'one', type: 'tarball', source: '%s/one'\nmod 'two', type: 'tarball', source: '%s/two'\nmod 'three', type: 'tarball', source: '%s/three'\n",
		server.URL, server.URL, server.URL))
	require.NoError(t, rt.resolvePuppetfile(map[string]Puppetfile{"main": pf}))
	require.EqualValues(t, 2, maxActive.Load())
	for _, module := range []string{"one", "two", "three"} {
		_, err := os.Stat(filepath.Join(env, "modules", module, "file"))
		require.NoError(t, err)
	}
}

func TestTarballPuppetfileModeRun(t *testing.T) {
	archive := testTarball(t, archiveEntry{name: "file", content: "ok"})
	var status atomic.Int32
	status.Store(http.StatusOK)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(int(status.Load()))
		_, _ = w.Write(archive)
	}))
	defer server.Close()
	env := t.TempDir()
	t.Chdir(env)
	path := filepath.Join(env, "Puppetfile")
	cache := filepath.Join(env, "cache")
	t.Setenv("g10k_cachedir", cache)
	opts := Options{Quiet: true, PFMode: true, PFLocation: path, ModuleDir: filepath.Join(env, "modules"), MaxWorker: 2, MaxExtractWorker: 2}
	require.NoError(t, os.WriteFile(path, []byte(fmt.Sprintf("mod 'apache', type: 'tarball', source: '%s/first'\n", server.URL)), 0o600))
	code, err := Run(opts)
	require.NoError(t, err)
	require.Equal(t, 0, code)
	data, err := os.ReadFile(filepath.Join(env, "modules", "apache", "file"))
	require.NoError(t, err)
	require.Equal(t, "ok", string(data))
	status.Store(http.StatusNotFound)
	require.NoError(t, os.WriteFile(path, []byte(fmt.Sprintf("mod 'apache', type: 'tarball', source: '%s/second'\n", server.URL)), 0o600))
	code, err = Run(opts)
	require.Equal(t, 1, code)
	require.ErrorContains(t, err, "404")
	data, err = os.ReadFile(filepath.Join(env, "modules", "apache", "file"))
	require.NoError(t, err)
	require.Equal(t, "ok", string(data))
}

func TestTarballExtractionSafety(t *testing.T) {
	for _, tt := range []struct {
		name    string
		entries []archiveEntry
		strip   bool
	}{
		{"traversal", []archiveEntry{{name: "../escape", content: "bad"}}, false},
		{"absolute", []archiveEntry{{name: "/escape", content: "bad"}}, false},
		{"symlink escape", []archiveEntry{{name: "link", kind: tar.TypeSymlink, link: "../escape"}}, false},
		{"hardlink escape", []archiveEntry{{name: "link", kind: tar.TypeLink, link: "../escape"}}, false},
		{"symlink parent", []archiveEntry{{name: "link", kind: tar.TypeSymlink, link: "../escape"}, {name: "link/file", content: "bad"}}, false},
		{"github multiple roots", []archiveEntry{{name: "one/a", content: "a"}, {name: "two/b", content: "b"}}, true},
		{"device", []archiveEntry{{name: "device", kind: tar.TypeChar}}, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			tmp := t.TempDir()
			archive := filepath.Join(tmp, "archive")
			require.NoError(t, os.WriteFile(archive, testTarball(t, tt.entries...), 0o600))
			target := filepath.Join(tmp, "target")
			require.NoError(t, os.Mkdir(target, 0o755))
			require.Error(t, tarballTestRuntime(t, Options{}).extractTarball(archive, target, tt.strip))
			_, err := os.Stat(filepath.Join(tmp, "escape"))
			require.ErrorIs(t, err, os.ErrNotExist)
		})
	}
}
