package g10k

import (
	"reflect"
	"testing"

	"github.com/voxpupuli/g10k/pkg/g10k/puppetfileparser"
)

func TestParserModuleDirHandler(t *testing.T) {
	for _, tt := range []struct {
		name        string
		override    string
		input       string
		wantDirs    []string
		wantModule1 string
		wantModule2 string
	}{
		{"default", "", "mod 'a/one'\nmod 'a/two'\n", []string{"modules"}, "modules", "modules"},
		{"multiple", "", "moduledir 'one//'\nmod 'a/one'\nmoduledir 'two/'\nmod 'a/two'\n", []string{"one", "two"}, "one", "two"},
		{"override", "custom", "moduledir 'one'\nmod 'a/one'\nmoduledir 'two'\nmod 'a/two'\n", []string{"custom"}, "custom", "custom"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			rt := NewRuntime(Options{ModuleDir: tt.override})
			state := rt.newPFParserState("Puppetfile", "source", "branch", false)
			if err := puppetfileparser.Parse(tt.input, state); err != nil {
				t.Fatal(err)
			}
			got := state.finalize("")
			if !reflect.DeepEqual(got.moduleDirs, tt.wantDirs) {
				t.Errorf("moduleDirs = %#v, want %#v", got.moduleDirs, tt.wantDirs)
			}
			if got.forgeModules["one"].moduleDir != tt.wantModule1 || got.forgeModules["two"].moduleDir != tt.wantModule2 {
				t.Errorf("module directories = %q, %q", got.forgeModules["one"].moduleDir, got.forgeModules["two"].moduleDir)
			}
		})
	}
}
