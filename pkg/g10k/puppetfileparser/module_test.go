package puppetfileparser_test

import (
	"testing"

	"github.com/voxpupuli/g10k/pkg/g10k/puppetfileparser"
)

func TestModuleAttributes(t *testing.T) {
	checkEvents(t, "mod 'author/example', '1.0.0', :latest, :git => 'https://example.test/repo', :branch => :control_branch, true\n", []event{
		{
			kind:  "mod",
			value: "author/example",
			attrs: []puppetfileparser.Attribute{
				{Value: "1.0.0", IsBareValue: true},
				{Name: "latest"},
				{Name: "git", Value: "https://example.test/repo", HasArrow: true},
				{Name: "branch", Value: ":control_branch", HasArrow: true},
				{Value: "true", IsBareValue: true},
			},
			line: "mod 'author/example', '1.0.0', :latest, :git => 'https://example.test/repo', :branch => :control_branch, true",
		},
	})
}

func TestModuleMultilineAndComments(t *testing.T) {
	checkEvents(t, "mod 'example', # comment\n  :git => 'repo',\n  # comment\n  :tag => 'v1'\n", []event{
		{
			kind:  "mod",
			value: "example",
			attrs: []puppetfileparser.Attribute{
				{Name: "git", Value: "repo", HasArrow: true},
				{Name: "tag", Value: "v1", HasArrow: true},
			},
			line: "mod 'example',:git => 'repo',:tag => 'v1'",
		},
	})
}
