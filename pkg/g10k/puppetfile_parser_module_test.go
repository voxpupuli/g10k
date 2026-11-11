package g10k

import (
	"reflect"
	"strings"
	"testing"

	"github.com/voxpupuli/g10k/pkg/g10k/puppetfileparser"
)

func TestParserModuleHandler(t *testing.T) {
	rt := NewRuntime(Options{Quiet: true})
	rt.Config.ForgeBaseURL = "default"
	rt.Config.IgnoreUnreachableModules = true
	state := rt.newPFParserState("Puppetfile", "source", "branch", false)
	input := "mod 'author/forge', '1.2.3', :sha256sum => 'digest'\n" +
		"mod 'git', :git => 'repo', :branch => :control_branch\n" +
		"mod 'author/notation', :git => 'repo2', :tag => 'v1'\n" +
		"mod 'local', :local => true\n"
	if err := puppetfileparser.Parse(input, state); err != nil {
		t.Fatal(err)
	}
	got := state.finalize("key")
	want := Puppetfile{
		forgeBaseURL: "default",
		forgeModules: map[string]ForgeModule{
			"forge": {version: "1.2.3", name: "forge", author: "author", sha256sum: "digest", moduleDir: "modules", sourceBranch: "source_branch"},
		},
		gitModules: map[string]GitModule{
			"git":      {git: "repo", link: true, ignoreUnreachable: true, moduleDir: "modules"},
			"notation": {git: "repo2", tag: "v1", ignoreUnreachable: true, moduleDir: "modules"},
			"local":    {local: true, ignoreUnreachable: true, moduleDir: "modules"},
		},
		privateKey:   "key",
		source:       "source",
		sourceBranch: "branch",
		moduleDirs:   []string{"modules"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Puppetfile = %#v, want %#v", got, want)
	}
}

func TestParserModuleHandlerErrors(t *testing.T) {
	for _, tt := range []struct {
		name  string
		input string
		force bool
		want  string
	}{
		{"duplicate forge", "mod 'a/same'\nmod 'b/same'\n", false, "Duplicate forge module"},
		{"duplicate git", "mod 'same', :git => 'one'\nmod 'same', :git => 'two'\n", false, "Duplicate module"},
		{"forge then git", "mod 'a/same'\nmod 'same', :git => 'repo'\n", false, "git puppet module with same name"},
		{"git then forge", "mod 'same', :git => 'repo'\nmod 'a/same'\n", false, "Forge Puppet module with same name"},
		{"duplicate attribute", "mod 'git', :git => 'one', :git => 'two'\n", false, "Duplicate module"},
		{"conflicting refs", "mod 'git', :git => 'repo', :branch => 'main', :tag => 'v1'\n", false, "conflicting git attributes"},
		{"invalid boolean", "mod 'git', :git => 'repo', :link => 'nope'\n", false, "to boolean"},
		{"force versions", "mod 'a/same', :latest\n", true, "force_forge_versions"},
		{"invalid forge name", "mod 'a/b/c'\n", false, "Forge module name is invalid"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			state := NewRuntime(Options{Quiet: true}).newPFParserState("Puppetfile", "source", "branch", tt.force)
			err := puppetfileparser.Parse(tt.input, state)
			if err == nil || !strings.Contains(err.Error(), tt.want) || !strings.Contains(err.Error(), "Puppetfile") {
				t.Fatalf("error = %v, want %q and path", err, tt.want)
			}
		})
	}
}
