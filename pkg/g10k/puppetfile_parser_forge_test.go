package g10k

import (
	"strings"
	"testing"
	"time"

	"github.com/voxpupuli/g10k/pkg/g10k/puppetfileparser"
)

func TestParserForgeDirectiveHandlers(t *testing.T) {
	for _, tt := range []struct {
		name    string
		input   string
		wantURL string
		wantTTL time.Duration
		wantErr string
	}{
		{"default", "mod 'author/module'\n", "default", 0, ""},
		{"custom", "forge.baseUrl 'custom'\nforge.cacheTTL '50m'\nmod 'author/module'\n", "custom", 50 * time.Minute, ""},
		{"override after module", "mod 'author/module'\nforge.baseURL 'later'\n", "later", 0, ""},
		{"invalid TTL", "forge.cacheTtl 'bad'\n", "", 0, "golang Duration"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			rt := NewRuntime(Options{})
			rt.Config.ForgeBaseURL = "default"
			state := rt.newPFParserState("Puppetfile", "source", "branch", false)
			err := puppetfileparser.Parse(tt.input, state)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) || !strings.Contains(err.Error(), "Puppetfile") {
					t.Fatalf("error = %v, want %q and path", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			got := state.finalize("")
			if got.forgeBaseURL != tt.wantURL || got.forgeCacheTTL != tt.wantTTL {
				t.Errorf("URL = %q, TTL = %v", got.forgeBaseURL, got.forgeCacheTTL)
			}
		})
	}
}
