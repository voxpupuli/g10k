package puppetfileparser_test

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/voxpupuli/g10k/pkg/g10k/puppetfileparser"
)

const digest = "aedd6dc1a5136c6a1a1ec2f285df2a70b0fe4c9effb254b5a1f58116e4c1659e"

func TestTarballStripComponents(t *testing.T) {
	for _, tt := range []struct {
		attribute string
		count     int
		wantError string
	}{
		{"strip_components: 0", 0, ""},
		{"strip_components: 2", 2, ""},
		{"strip_components:1", 1, ""},
		{":strip_components => 3", 3, ""},
		{"strip_components: -1", 0, "non-negative integer"},
		{"strip_components: 1.5", 0, "non-negative integer"},
		{"strip_components: 'abc'", 0, "non-negative integer"},
		{"strip_components: ''", 0, "non-negative integer"},
		{"strip_components: 999999999999999999999999", 0, "non-negative integer"},
		{"strip_components: 1, strip_components: 2", 0, "duplicate"},
	} {
		t.Run(tt.attribute, func(t *testing.T) {
			h := &recordingHandler{}
			require.NoError(t, puppetfileparser.Parse("mod 'apache', type: 'tarball', source: 'url', "+tt.attribute+"\n", h))
			got, ok, err := puppetfileparser.ParseTarball(h.events[0].value, h.events[0].attrs)
			require.True(t, ok)
			if tt.wantError != "" {
				require.ErrorContains(t, err, tt.wantError)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.count, got.StripComponents)
		})
	}
	h := &recordingHandler{}
	require.NoError(t, puppetfileparser.Parse("mod 'apache', :github_tarball => 'owner/repo', strip_components: 1\n", h))
	_, ok, err := puppetfileparser.ParseTarball(h.events[0].value, h.events[0].attrs)
	require.True(t, ok)
	require.ErrorContains(t, err, "invalid tarball attribute")
}

func TestTarballDeclarations(t *testing.T) {
	for _, tt := range []struct {
		name  string
		input string
		want  puppetfileparser.Tarball
	}{
		{
			"r10k",
			"mod 'puppetlabs-apache',\n  type: 'tarball',\n  source: 'https://repo.example.com/apache.tar.gz',\n  version: '" + digest + "' # sha256\n",
			puppetfileparser.Tarball{Name: "puppetlabs-apache", Source: "https://repo.example.com/apache.tar.gz", Version: digest},
		},
		{
			"r10k without digest",
			"mod 'apache', type: 'tarball', source: 'https://example.test/a.tar.gz'\n",
			puppetfileparser.Tarball{Name: "apache", Source: "https://example.test/a.tar.gz"},
		},
		{
			"r10k arrow notation with checksum alias",
			"mod 'author/apache', :type => :tarball, :source => 'https://example.test/a.tar.gz', :checksum => '" + digest + "'\n",
			puppetfileparser.Tarball{Name: "author/apache", Source: "https://example.test/a.tar.gz", Version: digest},
		},
		{
			"keywords without whitespace",
			"mod 'apache', type:'tarball', source:\"https://example.test/a.tar.gz\", version:'" + digest + "'\n",
			puppetfileparser.Tarball{Name: "apache", Source: "https://example.test/a.tar.gz", Version: digest},
		},
		{
			"attached unquoted values",
			"mod 'apache', type::tarball, source:https://example.test/a.tar.gz\n",
			puppetfileparser.Tarball{Name: "apache", Source: "https://example.test/a.tar.gz"},
		},
		{
			"librarian",
			"mod 'puppetlabs-apache', '0.6.0',\n  :github_tarball => 'puppetlabs/puppetlabs-apache'\n",
			puppetfileparser.Tarball{Name: "puppetlabs-apache", Version: "0.6.0", GitHubRepository: "puppetlabs/puppetlabs-apache"},
		},
		{
			"librarian without version",
			"mod 'apache', :github_tarball => 'puppetlabs/puppetlabs-apache'\n",
			puppetfileparser.Tarball{Name: "apache", GitHubRepository: "puppetlabs/puppetlabs-apache"},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			h := &recordingHandler{}
			if err := puppetfileparser.Parse(tt.input, h); err != nil {
				t.Fatal(err)
			}
			if len(h.events) != 1 {
				t.Fatalf("events = %#v", h.events)
			}
			got, ok, err := puppetfileparser.ParseTarball(h.events[0].value, h.events[0].attrs)
			if err != nil || !ok || !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("ParseTarball() = %#v, %v, %v; want %#v, true, nil", got, ok, err, tt.want)
			}
		})
	}
}

func TestTarballKeywordAttributes(t *testing.T) {
	checkEvents(t, "mod 'apache', type: 'tarball', source: 'url'\n", []event{
		{
			kind:  "mod",
			value: "apache",
			attrs: []puppetfileparser.Attribute{
				{Name: "type", Value: "tarball", IsKeyword: true},
				{Name: "source", Value: "url", IsKeyword: true},
			},
			line: "mod 'apache', type: 'tarball', source: 'url'",
		},
	})
}

func TestTarballInvalidDeclarations(t *testing.T) {
	for _, tt := range []struct {
		name  string
		input string
		want  string
	}{
		{"missing source", "type: 'tarball'", "source is required"},
		{"empty source", "type: 'tarball', source: ''", "source is required"},
		{"invalid digest", "type: 'tarball', source: 'url', version: 'not-a-digest'", "SHA-256"},
		{"empty digest", "type: 'tarball', source: 'url', version: ''", "SHA-256"},
		{"short digest", "type: 'tarball', source: 'url', version: 'abcd'", "SHA-256"},
		{"duplicate source", "type: 'tarball', source: 'one', source: 'two'", "duplicate"},
		{"digest aliases", "type: 'tarball', source: 'url', version: '" + digest + "', checksum: '" + digest + "'", "duplicate"},
		{"duplicate version", "'1.0', '2.0', :github_tarball => 'owner/repo'", "duplicate"},
		{"conflicting sources", "type: 'tarball', source: 'url', :github_tarball => 'owner/repo'", "conflicting"},
		{"unknown attribute", "type: 'tarball', source: 'url', :git => 'repo'", "invalid tarball attribute"},
		{"bare direct version", "'1.0', type: 'tarball', source: 'url'", "invalid bare"},
		{"empty github", ":github_tarball => ''", "repository is required"},
		{"bare symbol", ":github_tarball => 'owner/repo', :latest", "invalid tarball attribute"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			h := &recordingHandler{}
			if err := puppetfileparser.Parse("mod 'apache', "+tt.input+"\n", h); err != nil {
				t.Fatal(err)
			}
			got, ok, err := puppetfileparser.ParseTarball(h.events[0].value, h.events[0].attrs)
			if !ok || err == nil || !strings.Contains(err.Error(), tt.want) || got != (puppetfileparser.Tarball{}) {
				t.Fatalf("ParseTarball() = %#v, %v, %v; want error containing %q", got, ok, err, tt.want)
			}
		})
	}
}

func TestNonTarballDeclarations(t *testing.T) {
	for _, input := range []string{
		"mod 'author/forge', '1.0'\n",
		"mod 'git', :git => 'ssh://[::1]/repo'\n",
		"mod 'author/forge', :github_tarball\n",
		"mod 'author/forge', :type => 'git'\n",
	} {
		t.Run(input, func(t *testing.T) {
			h := &recordingHandler{}
			if err := puppetfileparser.Parse(input, h); err != nil {
				t.Fatal(err)
			}
			got, ok, err := puppetfileparser.ParseTarball(h.events[0].value, h.events[0].attrs)
			if ok || err != nil || got != (puppetfileparser.Tarball{}) {
				t.Fatalf("ParseTarball() = %#v, %v, %v", got, ok, err)
			}
		})
	}
}

func TestMalformedKeywordSyntax(t *testing.T) {
	for _, input := range []string{
		"mod 'apache', type 'tarball'\n",
		"mod 'apache', type: => 'tarball'\n",
	} {
		t.Run(input, func(t *testing.T) {
			err := puppetfileparser.Parse(input, &recordingHandler{})
			var syntaxErr *puppetfileparser.SyntaxError
			if !errors.As(err, &syntaxErr) {
				t.Fatalf("error = %v, want SyntaxError", err)
			}
		})
	}
}
