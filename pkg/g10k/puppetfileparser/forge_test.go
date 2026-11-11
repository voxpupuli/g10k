package puppetfileparser_test

import "testing"

func TestForgeDirectives(t *testing.T) {
	checkEvents(t, "forge.baseUrl 'https://forge.example'\nforge.baseURL 'other'\nforge.cacheTtl '50m'\nforge.cacheTTL 1h\n", []event{
		{kind: "forge.baseUrl", value: "https://forge.example"},
		{kind: "forge.baseUrl", value: "other"},
		{kind: "forge.cacheTtl", value: "50m", line: "forge.cacheTtl '50m'"},
		{kind: "forge.cacheTtl", value: "1h", line: "forge.cacheTTL 1h"},
	})
}
