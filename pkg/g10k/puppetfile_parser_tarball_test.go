package g10k

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParserRejectsInvalidTarballs(t *testing.T) {
	for _, tt := range []struct {
		name  string
		input string
		want  string
	}{
		{"mixed git", "mod 'author/name', :git => 'repo', :github_tarball => 'owner/repo'\n", "invalid tarball attribute"},
		{"invalid digest", "mod 'author/name', type: 'tarball', source: 'url', version: 'wrong'\n", "SHA-256"},
		{"missing source", "mod 'apache', type: 'tarball'\n", "source is required"},
		{"other keyword", "mod 'author/name', source: 'url'\n", "invalid setting"},
		{"unsafe name", "mod '../evil', type: 'tarball', source: 'url'\n", "invalid tarball module name"},
		{"invalid repository", "mod 'apache', :github_tarball => '../evil'\n", "invalid github_tarball repository"},
		{"duplicate tarballs", "mod 'a/name', type: 'tarball', source: 'url'\nmod 'b/name', type: 'tarball', source: 'url'\n", "duplicate module"},
		{"forge then tarball", "mod 'a/name'\nmod 'b/name', type: 'tarball', source: 'url'\n", "duplicate module"},
		{"tarball then forge", "mod 'a/name', type: 'tarball', source: 'url'\nmod 'b/name'\n", "duplicate module"},
		{"git then tarball", "mod 'name', :git => 'url'\nmod 'a/name', type: 'tarball', source: 'url'\n", "duplicate module"},
		{"tarball then git", "mod 'a/name', type: 'tarball', source: 'url'\nmod 'name', :git => 'url'\n", "duplicate module"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "Puppetfile")
			if err := os.WriteFile(path, []byte(tt.input), 0o600); err != nil {
				t.Fatal(err)
			}
			got, err := NewRuntime(Options{Quiet: true}).ParsePuppetfile(path, "", "", "", false)
			if err == nil || !strings.Contains(err.Error(), tt.want) || !strings.Contains(err.Error(), path) {
				t.Fatalf("error = %v, want %q and path", err, tt.want)
			}
			if got.forgeModules != nil || got.gitModules != nil {
				t.Fatalf("expected no partial result, got %#v", got)
			}
		})
	}
}

func TestParserPreservesLegacyNonTarballAttributes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "Puppetfile")
	if err := os.WriteFile(path, []byte("mod 'author/a', :github_tarball\nmod 'author/b', :type => 'git'\nmod 'author/c', type:'git'\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := NewRuntime(Options{Quiet: true}).ParsePuppetfile(path, "", "", "", false)
	if err != nil {
		t.Fatal(err)
	}
	if got.forgeModules["a"].version != "github_tarball" || got.forgeModules["b"].version != "git" || got.forgeModules["c"].version != "type:'git'" {
		t.Fatalf("forge modules = %#v", got.forgeModules)
	}
}
