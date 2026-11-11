package g10k

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParserAdapterSyntaxDiagnostics(t *testing.T) {
	path := filepath.Join(t.TempDir(), "Puppetfile")
	if err := os.WriteFile(path, []byte("mod 'broken', :git =>\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		name string
		opts Options
		want string
	}{
		{"normal", Options{}, "Error: Trailing comma or invalid setting for module found in " + path + " line: mod 'broken', :git =>"},
		{"dry run", Options{DryRun: true}, "Error: Could not interpret line: mod 'broken', :git => In " + path},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewRuntime(tt.opts).ParsePuppetfile(path, "", "", "", false)
			if err == nil || err.Error() != tt.want {
				t.Fatalf("error = %v, want %q", err, tt.want)
			}
			if got.forgeModules != nil || got.gitModules != nil {
				t.Fatalf("expected no partial result, got %#v", got)
			}
		})
	}
}
