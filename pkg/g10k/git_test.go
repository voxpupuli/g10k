package g10k

import "testing"

// The -check4update git detection path shells out to a real git binary via
// Runtime.executeCommand (exec.CommandContext), which is not injectable, so it
// cannot be driven from an isolated, self-contained unit test. The coverage
// strategy is therefore to unit test the only pure piece, the tag comparator
// highestSemverTag(), and cover the end-to-end detection behavior manually
// against local git repositories (see README "-check4update git staleness
// model").

func TestHighestSemverTag(t *testing.T) {
	tests := []struct {
		name string
		in   []string
		want string
	}{
		{"empty", nil, ""},
		{"non-semver only", []string{"release", "foo", "bar"}, ""},
		{"single semver", []string{"v1.2.3"}, "v1.2.3"},
		{"highest of mix", []string{"v1.0.0", "v1.10.0", "v1.2.0"}, "v1.10.0"},
		{"numeric order beats lexical", []string{"v2.0.0", "v10.0.0", "v1.0.0"}, "v10.0.0"},
		{"case insensitive compare", []string{"V1.0.0", "v2.0.0"}, "v2.0.0"},
		{"case insensitive equal returns original", []string{"v1.0.0", "V1.0.0"}, "v1.0.0"},
		{"missing prefix equals prefixed", []string{"1.5.0", "v1.5.0"}, "1.5.0"},
		{"prerelease lower than release", []string{"v1.2.0", "v1.2.0-alpha"}, "v1.2.0"},
		{"ignores non-semver among semver", []string{"latest", "v3.4.5", "dev"}, "v3.4.5"},
		{"only junk between", []string{"", "  "}, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := highestSemverTag(tc.in); got != tc.want {
				t.Errorf("highestSemverTag(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestStripVersionPrefix(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"v1.2.3", "1.2.3"},
		{"V1.2.3", "1.2.3"},
		{"1.2.3", "1.2.3"},
		{"release", "release"},
		{"", ""},
	}
	for _, tc := range tests {
		t.Run(tc.in, func(t *testing.T) {
			if got := stripVersionPrefix(tc.in); got != tc.want {
				t.Errorf("stripVersionPrefix(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
